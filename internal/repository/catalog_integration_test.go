package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestERPCatalogBusinessRestoreIntegration(t *testing.T) {
	name := os.Getenv("ERP_CATALOG_RESTORE_DB")
	if name == "" {
		t.Skip("explicit new catalog recovery copy required")
	}
	if !strings.HasPrefix(name, "ruoyi_go_mt_restore_catalog10_") || len(name) > 64 {
		t.Fatal("fresh catalog clone required")
	}
	cfg, e := config.Load(filepath.Join(os.Getenv("MT_TEST_ROOT"), "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != TenantDatabaseName || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		t.Fatal("local source only")
	}
	d.DBName = name
	cfg.MySQL.DSN = d.FormatDSN()
	if Init(cfg.MySQL) != nil {
		t.Fatal("clone unavailable")
	}
	t.Cleanup(func() {
		if e := Close(); e != nil {
			t.Error("clone close", e)
		}
	})
	base, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if VerifyERPCatalogSchema(base) != nil {
		t.Fatal("install010 in this new fault-test clone first")
	}
	newTenant := func() context.Context {
		v := model.SysTenant{TenantCode: fmt.Sprintf("catalog-db-%d", time.Now().UnixNano()%1000000000000), TenantName: "商品恢复副本事务测试", Status: "0", CreateBy: "catalog-restore-test"}
		if e := ProvisionTenant(base, &v, "owner", "$2a$10$YCpJk8aEXXYzYRG3M06U9ejMXUWdxtJYTHkSQCxJe3oGVpVf1YC.C"); e != nil {
			t.Fatal("new clone fixture", e)
		}
		t.Cleanup(func() {
			ctx, c := context.WithTimeout(tenantctx.System(context.Background()), 5*time.Second)
			defer c()
			if e := DB(ctx).Model(&model.SysTenant{}).Where("tenant_id=?", v.TenantID).Update("status", "1").Error; e != nil {
				t.Error("clone test tenant cleanup", e)
			}
		})
		return erpctx.WithUser(tenantctx.With(base, v.TenantID, false), *v.OwnerUserID)
	}
	a, b := newTenant(), newTenant()
	hash := func(v any) string { raw, _ := json.Marshal(v); return fmt.Sprintf("%x", sha256.Sum256(raw)) }
	input := func(code string) model.CatalogWrite {
		return model.CatalogWrite{RequestKey: uuid.NewString(), ProductCode: code, ProductName: "测试商品", Variants: []model.CatalogVariant{{SkuCode: code + "-S1", Unit: "件", Status: "0"}, {SkuCode: code + "-S2", Unit: "件", Status: "0"}}}
	}
	clone := func(v model.CatalogWrite) model.CatalogWrite {
		raw, _ := json.Marshal(v)
		var c model.CatalogWrite
		json.Unmarshal(raw, &c)
		return c
	}
	original := input("CAT-DB")
	hsh := hash(original)
	first, e := WriteCatalogProduct(a, 0, clone(original), hsh, "owner")
	if e != nil {
		t.Fatal(e)
	}
	pid := first.ProductIDs[0]
	t.Run("tenant_scope_and_current_action_replay", func(t *testing.T) {
		r, e := WriteCatalogProduct(a, 0, clone(original), hsh, "owner")
		if e != nil || !r.Existing || r.ProductIDs[0] != pid {
			t.Fatal("replay", e)
		}
		if _, e := WriteCatalogProduct(a, 0, clone(original), "different", "owner"); e == nil {
			t.Fatal("key mismatch accepted")
		}
		if _, e := SelectCatalogProduct(b, pid); e == nil {
			t.Fatal("foreign product visible")
		}
		other := input("CAT-DB")
		if _, e := WriteCatalogProduct(b, 0, other, hash(other), "owner"); e != nil {
			t.Fatal("tenant code isolation", e)
		}
	})
	current := func() model.CatalogWrite {
		p, e := SelectCatalogProduct(a, pid)
		if e != nil {
			t.Fatal(e)
		}
		v := model.CatalogWrite{RequestKey: uuid.NewString(), Version: p.Version, ProductCode: p.ProductCode, ProductName: p.ProductName, Images: p.Images}
		for _, x := range p.Variants {
			v.Variants = append(v.Variants, x.CatalogVariant)
		}
		return v
	}
	t.Run("parallel_CAS_exactly_one_and_no_removal", func(t *testing.T) {
		in := current()
		var wg sync.WaitGroup
		ok := 0
		var mu sync.Mutex
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v := clone(in)
				v.RequestKey = uuid.NewString()
				_, e := WriteCatalogProduct(a, pid, v, hash(v), "owner")
				if e == nil {
					mu.Lock()
					ok++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if ok != 1 {
			t.Fatal("CAS winners", ok)
		}
		in = current()
		in.Variants = in.Variants[:1]
		if _, e := WriteCatalogProduct(a, pid, in, hash(in), "owner"); e == nil {
			t.Fatal("removed persisted variant")
		}
	})
	t.Run("atomic_import_duplicates_roll_back_all_heads_skus_and_actions", func(t *testing.T) {
		p1, p2 := input("BATCH-1"), input("BATCH-2")
		p2.Variants[1].SkuCode = p1.Variants[0].SkuCode
		key := uuid.NewString()
		if _, e := ImportCatalogProducts(a, key, hash([]model.CatalogWrite{p1, p2}), []model.CatalogWrite{p1, p2}, []string{"", ""}, "owner"); e == nil {
			t.Fatal("duplicate accepted")
		}
		var n int64
		DB(a).Model(&model.CatalogProduct{}).Where("product_code IN ?", []string{"BATCH-1", "BATCH-2"}).Count(&n)
		if n != 0 {
			t.Fatal("partial heads")
		}
		DB(a).Model(&model.ErpSku{}).Where("sku_code LIKE 'BATCH-%'").Count(&n)
		if n != 0 {
			t.Fatal("partial SKUs")
		}
		DB(a).Model(&model.CatalogAction{}).Where("request_key=?", key).Count(&n)
		if n != 0 {
			t.Fatal("failed action retained")
		}
	})
	t.Run("write_failure_rolls_back_version_and_name", func(t *testing.T) {
		before := current()
		v := clone(before)
		v.ProductName = "不应保存"
		cb := db.Callback().Create()
		if e := cb.Before("gorm:create").Register("catalog:write-fault", func(q *gorm.DB) {
			if tenantctx.ID(q.Statement.Context) == tenantctx.ID(a) && q.Statement.Table == "sys_oper_log" {
				q.AddError(fmt.Errorf("injected final audit failure"))
			}
		}); e != nil {
			t.Fatal(e)
		}
		_, err := WriteCatalogProduct(a, pid, v, hash(v), "owner")
		cb.Remove("catalog:write-fault")
		if err == nil {
			t.Fatal("fault missing")
		}
		after := current()
		if after.Version != before.Version || after.ProductName != before.ProductName {
			t.Fatal("write not atomic")
		}
	})
	t.Run("referenced_critical_fields_and_legacy_bypass", func(t *testing.T) {
		v := current()
		skuID := v.Variants[0].SkuID
		wh := model.ErpWarehouse{WarehouseCode: "CAT-W", WarehouseName: "商品测试仓", Status: "0", CreateBy: "owner", CreateTime: types.Now()}
		if e := DB(a).Create(&wh).Error; e != nil {
			t.Fatal(e)
		}
		stock := model.ERPStock{WarehouseID: wh.WarehouseId, SkuID: skuID}
		if e := DB(a).Create(&stock).Error; e != nil {
			t.Fatal(e)
		}
		for _, field := range []string{"code", "unit", "spec"} {
			v = current()
			switch field {
			case "code":
				v.Variants[0].SkuCode = "CHANGED"
			case "unit":
				v.Variants[0].Unit = "箱"
			case "spec":
				v.Variants[0].Specification = "新规格"
			}
			if _, e := WriteCatalogProduct(a, pid, v, hash(v), "owner"); e == nil {
				t.Fatal("critical change", field)
			}
		}
		var sku model.ErpSku
		if DB(a).Where("sku_id=?", skuID).Take(&sku).Error != nil {
			t.Fatal("sku")
		}
		if e := UpdateErpSku(a, &sku); e == nil {
			t.Fatal("legacy bypass")
		}
		if e := DeleteErpSkuByIDs(a, []string{fmt.Sprint(skuID)}); e == nil {
			t.Fatal("legacy delete")
		}
		v = current()
		v.Variants[0].Status = "1"
		if _, e := WriteCatalogProduct(a, pid, v, hash(v), "owner"); e != nil {
			t.Fatal("disable valid", e)
		}
	})
}
