package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"net"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
	"sync"
	"testing"
	"time"
)

func logisticsTestClone(t *testing.T) context.Context {
	t.Helper()
	name := os.Getenv("ERP_LOGISTICS_RESTORE_DB")
	if name == "" {
		t.Skip("explicit new logistics restore copy required")
	}
	root := os.Getenv("MT_TEST_ROOT")
	var report struct {
		Target string `json:"restoredDatabase"`
	}
	raw, e := os.ReadFile(filepath.Join(root, "logs/erp-logistics-20261025/pre011-recovery/recovery.json"))
	if e != nil || json.Unmarshal(raw, &report) != nil || name != report.Target || name == TenantDatabaseName || !phase1DatabaseAllowed(name) {
		t.Fatal("only this round's new recovery copy")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
		t.Fatal("local dedicated source only")
	}
	d.DBName = name
	cfg.MySQL.DSN = d.FormatDSN()
	if Init(cfg.MySQL) != nil {
		t.Fatal("clone unavailable")
	}
	t.Cleanup(func() { _ = Close() })
	ctx, c := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(c)
	return ctx
}
func TestERPLogisticsRestoreSchema(t *testing.T) {
	ctx := logisticsTestClone(t)
	q := DB(tenantctx.System(ctx))
	for _, n := range logisticsBusinessTables {
		var count int64
		if q.Table(n).Count(&count).Error != nil || count != 0 {
			t.Fatal("fresh empty011 clone only")
		}
	}
	for _, n := range []string{"erp_package_shipping", "erp_shipping_method", "erp_carrier", "erp_logistics_action"} {
		if q.Exec("DROP TABLE "+n).Error != nil {
			t.Fatal("drop new clone-only table")
		}
	}
	if VerifyERPLogisticsSchema(ctx) == nil {
		t.Fatal("orphan version accepted")
	}
	if _, e := InstallERPLogistics(ctx); e == nil {
		t.Fatal("registered missing tables recreated")
	}
	if q.Exec("DELETE FROM erp_schema_migration WHERE version_key=?", ERPLogisticsVersion).Error != nil {
		t.Fatal("remove clone-only orphan")
	}
	if VerifyERPLogisticsSchema(ctx) != nil {
		t.Fatal("all absent should be optional")
	}
	cb := db.Callback().Raw()
	hook := "logistics11:create-fault"
	if cb.Before("gorm:raw").Register(hook, func(tx *gorm.DB) {
		if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE erp_shipping_method (") {
			tx.AddError(errors.New("synthetic logistics DDL interruption"))
		}
	}) != nil {
		t.Fatal("hook")
	}
	_, installErr := InstallERPLogistics(ctx)
	cb.Remove(hook)
	if installErr == nil {
		t.Fatal("DDL fault not reached")
	}
	state, e := InspectERPLogistics(ctx)
	if e != nil || state.InstalledTables != 1 || state.Recorded {
		t.Fatal("partial DDL not retained", e)
	}
	if VerifyERPLogisticsSchema(ctx) == nil {
		t.Fatal("partial accepted")
	}
	if _, e = InstallERPLogistics(ctx); e != nil {
		t.Fatal("resume", e)
	}
	if VerifyERPLogisticsSchema(ctx) != nil {
		t.Fatal("complete invalid")
	}
	if q.Exec("ALTER TABLE erp_carrier MODIFY version BIGINT NULL").Error != nil {
		t.Fatal("inject drift")
	}
	if VerifyERPLogisticsSchema(ctx) == nil {
		t.Fatal("drift accepted")
	}
	if _, e = InstallERPLogistics(ctx); e == nil {
		t.Fatal("drift silently repaired")
	}
	if q.Exec("ALTER TABLE erp_carrier MODIFY version BIGINT NOT NULL").Error != nil {
		t.Fatal("restore shape")
	}
	if VerifyERPLogisticsSchema(ctx) != nil {
		t.Fatal("restored invalid")
	}
}
func TestERPLogisticsRestoreBusiness(t *testing.T) {
	base := logisticsTestClone(t)
	if VerifyERPLogisticsSchema(base) != nil {
		t.Fatal("install011 in clone first")
	}
	newTenant := func() context.Context {
		t.Helper()
		v := model.SysTenant{TenantCode: fmt.Sprintf("l11-db-%d", time.Now().UnixNano()%1000000000000), TenantName: "物流副本事务测试", Status: "0", CreateBy: "logistics-test"}
		if e := ProvisionTenant(base, &v, "owner", "$2a$10$YCpJk8aEXXYzYRG3M06U9ejMXUWdxtJYTHkSQCxJe3oGVpVf1YC.C"); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			ctx, c := context.WithTimeout(tenantctx.System(context.Background()), 5*time.Second)
			defer c()
			if DB(ctx).Model(&model.SysTenant{}).Where("tenant_id=?", v.TenantID).Update("status", "1").Error != nil {
				t.Error("disable clone fixture")
			}
		})
		return erpctx.WithUser(tenantctx.With(base, v.TenantID, false), *v.OwnerUserID)
	}
	a, b := newTenant(), newTenant()
	hash := func(v any) string { raw, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(raw)) }
	write := func(id int64, in model.LogisticsWrite, method bool) *model.LogisticsResult {
		t.Helper()
		r, e := WriteLogisticsMaster(a, id, in, method, hash([]any{id, method, in}), "owner")
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	carrier := model.LogisticsWrite{RequestKey: uuid.NewString(), Code: "L11", Name: "原物流名称", Status: "0"}
	c := write(0, carrier, false)
	mIn := model.LogisticsWrite{RequestKey: uuid.NewString(), Code: "STANDARD", Name: "原配送方式", CarrierID: c.ID, Status: "0"}
	m := write(0, mIn, true)
	t.Run("scope_replay_and_join_projection", func(t *testing.T) {
		r := write(0, carrier, false)
		if !r.Existing || r.ID != c.ID {
			t.Fatal("replay")
		}
		if _, e := WriteLogisticsMaster(a, 0, carrier, false, "different", "owner"); e == nil {
			t.Fatal("same key mismatch")
		}
		if _, e := SelectLogisticsMaster(b, c.ID, false); e == nil {
			t.Fatal("foreign master")
		}
		rows, n, e := SelectLogisticsOptions(a, model.LogisticsQuery{}, page.Query{PageNum: 1, PageSize: 100})
		if e != nil || n != 1 || len(rows) != 1 || rows[0].CarrierName != "原物流名称" {
			t.Fatal("joined options", e)
		}
		v, e := SelectLogisticsMaster(a, m.ID, true)
		if e != nil || v.Code != "STANDARD" || v.CarrierID != c.ID {
			t.Fatal("method projection", e)
		}
	})
	t.Run("CAS_and_failed_audit_roll_back_master_and_original_result", func(t *testing.T) {
		in := carrier
		in.RequestKey = uuid.NewString()
		in.Version = 1
		in.Name = "不可落库"
		hook := "l11:audit"
		db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(errors.New("synthetic audit failure"))
			}
		})
		_, e := WriteLogisticsMaster(a, c.ID, in, false, hash(in), "owner")
		db.Callback().Create().Remove(hook)
		if e == nil {
			t.Fatal("audit fault not hit")
		}
		v, e := SelectLogisticsMaster(a, c.ID, false)
		if e != nil || v.Version != 1 || v.Name != carrier.Name {
			t.Fatal("partial master update")
		}
		var count int64
		DB(a).Model(&model.LogisticsAction{}).Where("request_key=?", in.RequestKey).Count(&count)
		if count != 0 {
			t.Fatal("orphan result")
		}
		var wg sync.WaitGroup
		results := make(chan error, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				v := carrier
				v.Version = 1
				v.RequestKey = uuid.NewString()
				v.Name = fmt.Sprintf("并发名称%d", i)
				_, e := WriteLogisticsMaster(a, c.ID, v, false, hash(v), "owner")
				results <- e
			}(i)
		}
		wg.Wait()
		close(results)
		wins := 0
		for e := range results {
			if e == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("CAS winners", wins)
		}
	})
	// Build a real reserved order through existing repository transactions.
	currentCarrier, err := SelectLogisticsMaster(a, c.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	resetCarrier := carrier
	resetCarrier.Version = currentCarrier.Version
	resetCarrier.RequestKey = uuid.NewString()
	write(c.ID, resetCarrier, false)
	sku := model.ErpSku{SkuCode: "L11-SKU", ProductName: "物流测试商品", Unit: "件", Status: "0"}
	if InsertErpSku(a, &sku) != nil {
		t.Fatal("SKU")
	}
	wh := model.ErpWarehouse{WarehouseCode: "L11-W", WarehouseName: "物流测试仓", Status: "0"}
	if InsertErpWarehouse(a, &wh) != nil {
		t.Fatal("warehouse")
	}
	shop := model.ErpShop{ShopCode: "L11-SHOP", ShopName: "物流测试店铺", ChannelCode: "MANUAL", SiteCode: "CN", Currency: "CNY", Status: "0", Version: 1}
	if InsertErpShop(a, &shop) != nil {
		t.Fatal("shop")
	}
	item := model.ErpShopItem{ShopID: shop.ShopId, ShopItemCode: "ITEM", Title: "商品", Status: "0", Version: 1}
	if InsertERPShopItem(a, &item) != nil {
		t.Fatal("item")
	}
	if _, e := ChangeERPShopItemMapping(a, item.ItemID, 1, sku.SkuId, "", "owner", false); e != nil {
		t.Fatal(e)
	}
	receipt := model.ERPDocument{WarehouseID: wh.WarehouseId, Kind: "IN", Status: "DRAFT", Version: 1, RequestKey: uuid.NewString(), RequestHash: strings.Repeat("a", 64)}
	if e := InsertERPDocument(a, &receipt, []model.ERPLineInput{{SkuID: sku.SkuId, Quantity: 10}}); e != nil {
		t.Fatal(e)
	}
	if e := TransitionERPDocument(a, "IN", receipt.DocumentID, "confirm", "owner"); e != nil {
		t.Fatal(e)
	}
	makePackage := func() int64 {
		t.Helper()
		in := model.ERPOrderWrite{ShopID: shop.ShopId, SourceOrderNo: uuid.NewString(), RequestKey: uuid.NewString(), ERPOrderFields: model.ERPOrderFields{Currency: "CNY", OrderedAt: types.Now(), ReceiverName: "合成收件人", ReceiverPhone: "TEST000", ReceiverCountry: "CN", ReceiverAddress: "合成测试地址"}, Lines: []model.ERPOrderLineInput{{SourceLineNo: "1", ShopItemCode: "ITEM", SourceTitle: "标题", Quantity: 1}}}
		o, e := CreateERPOrder(a, in, "MANUAL", hash(in), "owner")
		if e != nil {
			t.Fatal(e)
		}
		r, e := ReviewERPOrder(a, o.OrderID, model.ERPReviewInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, WarehouseID: wh.WarehouseId}, uuid.NewString(), "owner")
		if e != nil {
			t.Fatal(e)
		}
		if r.PackageID == nil {
			t.Fatal("review did not create a package")
		}
		return *r.PackageID
	}
	pid := makePackage()
	save := model.PackageShippingInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, ShippingMethodID: m.ID, TrackingNo: "000L11"}
	if _, e := SaveERPPackageShipping(a, pid, save, hash(save), "owner"); e != nil {
		t.Fatal(e)
	}
	t.Run("frozen_names_disabled_replay_and_shipping", func(t *testing.T) {
		v, e := SelectLogisticsMaster(a, c.ID, false)
		if e != nil {
			t.Fatal(e)
		}
		change := carrier
		change.Version = v.Version
		change.RequestKey = uuid.NewString()
		change.Name = "新名称"
		change.Status = "1"
		write(c.ID, change, false)
		mi := mIn
		mi.Version = 1
		mi.RequestKey = uuid.NewString()
		mi.Name = "新方式"
		mi.Status = "1"
		write(m.ID, mi, true)
		r, e := SaveERPPackageShipping(a, pid, save, hash(save), "owner")
		if e != nil || !r.Existing {
			t.Fatal("disabled original replay", e)
		}
		snap, e := SelectPackageShipping(a, pid)
		if e != nil || snap == nil || snap.CarrierName != "原物流名称" || snap.MethodName != "原配送方式" {
			t.Fatal("snapshot drift", e)
		}
		bad := save
		bad.Version = 2
		bad.RequestKey = uuid.NewString()
		bad.TrackingNo = "000L11-UPDATED"
		if _, e := SaveERPPackageShipping(a, pid, bad, hash(bad), "owner"); e != nil {
			t.Fatal("saved disabled method should allow tracking maintenance", e)
		}
		fresh := makePackage()
		newSelection := save
		newSelection.RequestKey = uuid.NewString()
		if _, e := SaveERPPackageShipping(a, fresh, newSelection, hash(newSelection), "owner"); e == nil {
			t.Fatal("disabled method newly selected")
		}
		if _, e := ActERPPackage(a, pid, model.ERPLogisticsInput{ERPActionToken: model.ERPActionToken{Version: 3, RequestKey: uuid.NewString()}}, true, uuid.NewString(), "owner"); e != nil {
			t.Fatal("saved disabled method should not strand hand shipment", e)
		}
		rows, n, e := SelectLogisticsShipments(a, model.ShipmentQuery{}, page.Query{PageNum: 1, PageSize: 100})
		if e != nil || n != 1 || len(rows) != 1 || rows[0].CarrierName != "原物流名称" || rows[0].TrackingNo != "000L11-UPDATED" {
			t.Fatal("shipment scoped left join", e)
		}
		if _, e := SelectPackageShipping(b, pid); e == nil {
			t.Fatal("foreign package snapshot")
		}
	})
	t.Run("legacy_switch_and_package_audit_failure_are_atomic", func(t *testing.T) {
		v, _ := SelectLogisticsMaster(a, c.ID, false)
		ci := carrier
		ci.Version = v.Version
		ci.RequestKey = uuid.NewString()
		write(c.ID, ci, false)
		v, _ = SelectLogisticsMaster(a, m.ID, true)
		mi := mIn
		mi.Version = v.Version
		mi.RequestKey = uuid.NewString()
		write(m.ID, mi, true)
		p := makePackage()
		in := save
		in.RequestKey = uuid.NewString()
		if _, e := SaveERPPackageShipping(a, p, in, hash(in), "owner"); e != nil {
			t.Fatal(e)
		}
		if _, e := ActERPPackage(a, p, model.ERPLogisticsInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, CarrierCode: "MANUAL", TrackingNo: "OLD"}, false, uuid.NewString(), "owner"); e != nil {
			t.Fatal(e)
		}
		if snap, e := SelectPackageShipping(a, p); e != nil || snap != nil {
			t.Fatal("manual switch kept stale association")
		}
		p = makePackage()
		in.RequestKey = uuid.NewString()
		hook := "l11:package-audit"
		db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(errors.New("synthetic package audit failure"))
			}
		})
		_, e := SaveERPPackageShipping(a, p, in, hash(in), "owner")
		db.Callback().Create().Remove(hook)
		if e == nil {
			t.Fatal("fault not hit")
		}
		view, e := SelectERPPackage(a, p)
		if e != nil || view.Version != 1 || view.CarrierCode != "" {
			t.Fatal("partial package write")
		}
		if snap, e := SelectPackageShipping(a, p); e != nil || snap != nil {
			t.Fatal("orphan shipping snapshot")
		}
	})
}
