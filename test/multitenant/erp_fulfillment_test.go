package multitenant_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

func TestERPFulfillment(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance only")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("invalid local config")
	}
	host, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.DBName != repository.TenantDatabaseName || d.Net != "tcp" || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("nonlocal/original DB refused")
	}
	if e = repository.Init(cfg.MySQL); e != nil {
		t.Fatal(e)
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { previous := s.t; s.t = t; defer func() { s.t = previous }(); fn(t) })
	}
	deny := func(r map[string]any) {
		s.t.Helper()
		if r["code"] == float64(200) {
			s.t.Fatal("expected rejection")
		}
	}
	data := func(r map[string]any) map[string]any { s.ok(r); return r["data"].(map[string]any) }
	master := func(f operationsFixture, kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "履约测试SKU"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "履约测试仓库"
		}
		s.ok(s.req("POST", "/erp/"+kind, f.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, f.owner, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)[kind+"Id"].(float64))
	}
	shop := func(f operationsFixture, code string) int64 {
		return int64(data(s.req("POST", "/erp/shop", f.owner, map[string]any{"shopCode": code, "shopName": "履约测试店铺", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"}))["shopId"].(float64))
	}
	wa, wb := master(a, "warehouse", "FLOW-W"), master(b, "warehouse", "FLOW-W")
	x, y, z := master(a, "sku", "FLOW-X"), master(a, "sku", "FLOW-Y"), master(a, "sku", "FLOW-Z")
	sa, sb := shop(a, "FLOW-S"), shop(b, "FLOW-S")
	item := func(code string, sku int64) int64 {
		id := int64(data(s.req("POST", "/erp/shopItem", a.owner, map[string]any{"shopId": sa, "shopItemCode": code, "title": "可售变种", "status": "0"}))["itemId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 1, "skuId": sku}))
		return id
	}
	firstItem := item("000A", x)
	item("000B", x)
	item("000Y", y)
	item("000Z", z)
	receipt := data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 100}, {SkuID: y, Quantity: 10}}}))
	s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%.0f/confirm", receipt["documentId"]), a.owner, nil))
	input := func(codes []string, qty []int64) model.ERPOrderWrite {
		var v model.ERPOrderWrite
		raw, _ := json.Marshal(map[string]any{"shopId": sa, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "PRIVATE_FULFILLMENT_RECIPIENT", "receiverPhone": "PRIVATE_FULFILLMENT_PHONE", "receiverCountry": "CN", "receiverAddress": "PRIVATE_FULFILLMENT_ADDRESS", "currency": "CNY", "originalAmount": nil, "orderedAt": "2026-10-21 12:00:00"})
		json.Unmarshal(raw, &v)
		for i, c := range codes {
			v.Lines = append(v.Lines, model.ERPOrderLineInput{SourceLineNo: fmt.Sprintf("%04d", i), ShopItemCode: c, SourceTitle: "履约原始标题", Quantity: qty[i]})
		}
		return v
	}
	create := func(codes []string, qty []int64) int64 {
		return int64(data(s.req("POST", "/erp/order", a.owner, input(codes, qty)))["orderId"].(float64))
	}
	order := func(id int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/order/%d", id), a.owner, nil))
	}
	pack := func(id int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/package/%d", id), a.owner, nil))
	}
	token := func(version any) map[string]any {
		return map[string]any{"version": version, "requestKey": uuid.NewString()}
	}
	review := func(id int64) map[string]any {
		v := token(order(id)["version"])
		v["warehouseId"] = wa
		return data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v))
	}
	cancel := func(id int64) {
		v := token(order(id)["version"])
		v["reason"] = "专用验收取消释放"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", id), a.owner, v))
	}
	logistics := func(pid int64) map[string]any {
		v := token(pack(pid)["version"])
		v["carrierCode"] = "MANUAL"
		v["trackingNo"] = "00000123456789"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", pid), a.owner, v))
		return v
	}
	balance := func(sku int64) (int64, int64) {
		r := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", wa, sku), a.owner, nil)
		s.ok(r)
		rows := r["rows"].([]any)
		if len(rows) == 0 {
			return 0, 0
		}
		v := rows[0].(map[string]any)
		return int64(v["onHand"].(float64)), int64(v["reserved"].(float64))
	}
	ctxFor := func() context.Context {
		return erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID)
	}
	var shippedOrder, shippedDoc, shippedPackage int64
	run("workbench_and_readiness_are_scoped_read_only_diagnostics", func(t *testing.T) {
		good := create([]string{"000A", "000B"}, []int64{2, 3})
		path := fmt.Sprintf("/erp/order/%d/readiness?warehouseId=%d", good, wa)
		r := data(s.req("GET", path, a.owner, nil))
		if r["ready"] != true || r["version"] != float64(1) || len(r["stocks"].([]any)) != 1 || r["stocks"].([]any)[0].(map[string]any)["required"] != float64(5) {
			t.Fatal("readiness aggregate mismatch")
		}
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "receiver") {
			t.Fatal("readiness exposes recipient")
		}
		if h, held := balance(x); h != 100 || held != 0 {
			t.Fatal("readiness mutated stock")
		}
		if order(good)["warehouseId"] != nil || order(good)["package"] != nil || order(good)["version"] != float64(1) {
			t.Fatal("readiness persisted review state")
		}
		deny(s.req("GET", path, b.owner, nil))
		deny(s.req("GET", path, platform, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d/readiness?warehouseId=%d", good, wb), a.owner, nil))
		for _, suffix := range []string{"", "?warehouseId=-1", "?warehouseId=1.2", "?warehouseId=9223372036854775808"} {
			deny(s.req("GET", fmt.Sprintf("/erp/order/%d/readiness%s", good, suffix), a.owner, nil))
		}
		for _, v := range []struct {
			codes []string
			qty   []int64
			code  string
		}{{[]string{"UNKNOWN"}, []int64{1}, "LINES"}, {[]string{"000Z"}, []int64{3}, "STOCK_SHORTAGE"}, {[]string{"000A", "000B"}, []int64{600000000, 600000000}, "LINES"}} {
			id := create(v.codes, v.qty)
			r := data(s.req("GET", fmt.Sprintf("/erp/order/%d/readiness?warehouseId=%d", id, wa), a.owner, nil))
			found := false
			for _, raw := range r["issues"].([]any) {
				if raw.(map[string]any)["code"] == v.code {
					found = true
				}
			}
			if r["ready"] != false || !found {
				t.Fatal("missing readiness blocker", v.code)
			}
		}
		for _, status := range []string{"PENDING_REVIEW", "READY_TO_SHIP", "SHIPPED", "CANCELLED"} {
			list := s.req("GET", "/erp/order/list?status="+status, a.owner, nil)
			s.ok(list)
			summary := data(s.req("GET", "/erp/order/workbench?status="+status, a.owner, nil))
			key := map[string]string{"PENDING_REVIEW": "pendingReview", "READY_TO_SHIP": "readyToShip", "SHIPPED": "shipped", "CANCELLED": "cancelled"}[status]
			if summary[key] != list["total"] {
				t.Fatal("workbench/list scope mismatch")
			}
		}
		own := data(s.req("GET", "/erp/order/workbench", a.owner, nil))
		other := data(s.req("GET", "/erp/order/workbench", b.owner, nil))
		if own["total"] != float64(4) || other["total"] != float64(0) {
			t.Fatal("workbench tenant isolation")
		}
		filtered := data(s.req("GET", "/erp/order/workbench?sourceOrderNo="+order(good)["sourceOrderNo"].(string), a.owner, nil))
		if filtered["total"] != float64(1) {
			t.Fatal("workbench source filter ignored")
		}
		deny(s.req("GET", "/erp/order/workbench", platform, nil))
		deny(s.req("GET", "/erp/order/workbench?shopId=-1", a.owner, nil))
	})
	run("review_aggregates_SKU_preserves_original_lines_and_replays", func(t *testing.T) {
		id := create([]string{"000A", "000B"}, []int64{2, 3})
		v := token(1)
		v["warehouseId"] = wa
		r := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v))
		shippedOrder = id
		shippedPackage = int64(r["packageId"].(float64))
		d := order(id)
		shippedDoc = int64(d["stockDocumentId"].(float64))
		if d["status"] != "READY_TO_SHIP" || d["version"] != float64(2) {
			t.Fatal("order review state")
		}
		for _, line := range d["lines"].([]any) {
			l := line.(map[string]any)
			if l["skuId"] != float64(x) || l["mappingVersion"] != float64(1) || l["unit"] != "件" {
				t.Fatal("review snapshot missing")
			}
		}
		p := pack(shippedPackage)
		if len(p["lines"].([]any)) != 2 {
			t.Fatal("package collapsed original lines")
		}
		raw, _ := json.Marshal(p)
		if strings.Contains(string(raw), "PRIVATE_") {
			t.Fatal("package leaked recipient")
		}
		stock := data(s.req("GET", fmt.Sprintf("/erp/sales/%d", shippedDoc), a.owner, nil))
		lines := stock["lines"].([]any)
		if len(lines) != 1 || lines[0].(map[string]any)["quantity"] != float64(5) {
			t.Fatal("SKU not aggregated")
		}
		h, res := balance(x)
		if h != 100 || res != 5 {
			t.Fatal("incorrect reservation")
		}
		r = data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v))
		if r["existing"] != true {
			t.Fatal("review replay not recognized")
		}
		v["warehouseId"] = wb
		deny(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v))
		if order(id)["version"] != float64(2) {
			t.Fatal("replay changed order")
		}
	})
	run("managed_voucher_old_write_entries_reject_even_owner", func(t *testing.T) {
		for _, action := range []string{"confirm", "ship", "cancel"} {
			deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d/%s", shippedDoc, action), a.owner, nil))
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d", shippedDoc), a.owner, map[string]any{"version": 2, "warehouseId": wa, "lines": []model.ERPLineInput{{SkuID: x, Quantity: 5}}}))
		ctx, c := context.WithTimeout(ctxFor(), 5*time.Second)
		defer c()
		var doc model.ERPDocument
		if e := repository.DB(ctx).Where("document_id=?", shippedDoc).Take(&doc).Error; e != nil {
			t.Fatal(e)
		}
		deny(s.req("POST", "/erp/sales", a.owner, map[string]any{"warehouseId": wa, "requestKey": doc.RequestKey, "lines": []model.ERPLineInput{{SkuID: x, Quantity: 5}}}))
	})
	run("logistics_versions_ship_and_terminal_idempotency", func(t *testing.T) {
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", shippedPackage), a.owner, token(1)))
		v := logistics(shippedPackage)
		if order(shippedOrder)["version"] != float64(3) {
			t.Fatal("logistics failed to fence order CAS")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", shippedPackage), a.owner, v))
		if pack(shippedPackage)["version"] != float64(2) {
			t.Fatal("logistics replay increments version")
		}
		v["trackingNo"] = "different"
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", shippedPackage), a.owner, v))
		ship := token(2)
		r := data(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", shippedPackage), a.owner, ship))
		if r["status"] != "SHIPPED" {
			t.Fatal("not shipped")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", shippedPackage), a.owner, ship))
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", shippedPackage), a.owner, token(3)))
		v = token(order(shippedOrder)["version"])
		v["reason"] = "不能取消已发货"
		deny(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", shippedOrder), a.owner, v))
		h, res := balance(x)
		if h != 95 || res != 0 {
			t.Fatal("ship not exactly once")
		}
		p := pack(shippedPackage)
		if p["trackingNo"] != "00000123456789" || p["shippedAt"] == nil {
			t.Fatal("tracking text/time contract")
		}
	})
	run("pending_cancel_and_unmapped_shortage_aggregate_failures", func(t *testing.T) {
		for _, tc := range []struct {
			codes []string
			qty   []int64
		}{{[]string{"UNKNOWN"}, []int64{1}}, {[]string{"000A", "000Y"}, []int64{1, 20}}, {[]string{"000A", "000B"}, []int64{600000000, 500000000}}} {
			id := create(tc.codes, tc.qty)
			v := token(1)
			v["warehouseId"] = wa
			deny(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v))
			d := order(id)
			if d["status"] != "PENDING_REVIEW" || d["stockDocumentId"] != nil || d["package"] != nil {
				t.Fatal("failed review partially committed")
			}
			for _, l := range d["lines"].([]any) {
				if l.(map[string]any)["mappingId"] != nil {
					t.Fatal("failed review left snapshot")
				}
			}
			v = token(1)
			v["reason"] = "待审核取消"
			s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", id), a.owner, v))
			s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", id), a.owner, v))
			if order(id)["stockDocumentId"] != nil {
				t.Fatal("pending cancel created voucher")
			}
		}
		h, res := balance(x)
		if h != 95 || res != 0 {
			t.Fatal("failed review changed inventory")
		}
	})
	run("faults_roll_back_review_cancel_ship_as_whole_transactions", func(t *testing.T) {
		ctx, c := context.WithTimeout(ctxFor(), 30*time.Second)
		defer c()
		db := repository.DB(ctx)
		hook := "stage6:transaction_fault"
		failTable := func(table string, fn func() error) {
			if e := db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("injected transaction fault"))
				}
			}); e != nil {
				t.Fatal(e)
			}
			err := fn()
			db.Callback().Create().Remove(hook)
			if err == nil {
				t.Fatal("fault did not fail transaction", table)
			}
		}
		for _, table := range []string{"erp_stock_ledger", "erp_package", "erp_order_event", "sys_oper_log"} {
			id := create([]string{"000A"}, []int64{1})
			failTable(table, func() error {
				_, e := service.ReviewERPOrder(ctx, id, model.ERPReviewInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, WarehouseID: wa}, "fault-test")
				return e
			})
			if o := order(id); o["status"] != "PENDING_REVIEW" || o["package"] != nil || o["stockDocumentId"] != nil {
				t.Fatal("review failure persisted partial state", table)
			}
		}
		id := create([]string{"000A"}, []int64{2})
		r := review(id)
		pid := int64(r["packageId"].(float64))
		failTable("erp_stock_ledger", func() error {
			_, e := service.CancelERPOrder(ctx, id, model.ERPCancelInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Reason: "fault cancel"}, "test")
			return e
		})
		if order(id)["status"] != "READY_TO_SHIP" {
			t.Fatal("cancel failure changed status")
		}
		logistics(pid)
		failTable("erp_order_event", func() error {
			_, e := service.ShipERPPackage(ctx, pid, model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, "test")
			return e
		})
		if pack(pid)["status"] != "READY_TO_SHIP" {
			t.Fatal("ship failure persisted package")
		}
		h, res := balance(x)
		if h != 95 || res != 2 {
			t.Fatal("stock rollback failed")
		}
		cancel(id)
	})
	run("concurrent_review_and_ship_cancel_have_single_outcomes", func(t *testing.T) {
		id := create([]string{"000A"}, []int64{1})
		v := token(1)
		v["warehouseId"] = wa
		rs := parallelJSON(s, 5, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, v
		})
		for _, r := range rs {
			if r.err != nil || r.code != 200 {
				t.Fatal("identical concurrent replay failed", r)
			}
		}
		if order(id)["version"] != float64(2) {
			t.Fatal("double review")
		}
		cancel(id)
		id = create([]string{"000A"}, []int64{1})
		rs = parallelJSON(s, 5, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/order/%d/review", id), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": wa}
		})
		wins := 0
		for _, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("review CAS winners", wins)
		}
		cancel(id)
		id = create([]string{"000A"}, []int64{4})
		r := review(id)
		pid := int64(r["packageId"].(float64))
		logistics(pid)
		before, _ := balance(x)
		ov := order(id)["version"]
		pv := pack(pid)["version"]
		rs = parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				v := token(ov)
				v["reason"] = "竞争取消"
				return "PUT", fmt.Sprintf("/erp/order/%d/cancel", id), a.owner, v
			}
			return "PUT", fmt.Sprintf("/erp/package/%d/ship", pid), a.owner, token(pv)
		})
		wins = 0
		for _, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("ship/cancel race", wins)
		}
		h, res := balance(x)
		status := order(id)["status"]
		if res != 0 || (status == "SHIPPED" && h != before-4) || (status == "CANCELLED" && h != before) {
			t.Fatal("race stock mismatch")
		}
		if pack(pid)["status"] != status {
			t.Fatal("order/package divergence")
		}
	})
	run("shop_warehouse_SKU_disabled_still_allows_cancel_release", func(t *testing.T) {
		id := create([]string{"000A"}, []int64{1})
		r := review(id)
		pid := int64(r["packageId"].(float64))
		logistics(pid)
		shopRow := data(s.req("GET", fmt.Sprintf("/erp/shop/%d", sa), a.owner, nil))
		shopRow["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", sa), a.owner, shopRow))
		skuRow := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", x), a.owner, nil))
		skuRow["status"] = "1"
		s.ok(s.req("PUT", "/erp/sku", a.owner, skuRow))
		whRow := data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		whRow["status"] = "1"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, whRow))
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", pid), a.owner, token(pack(pid)["version"])))
		cancel(id)
		if pack(pid)["status"] != "CANCELLED" {
			t.Fatal("disabled cancel did not release")
		}
		shopRow = data(s.req("GET", fmt.Sprintf("/erp/shop/%d", sa), a.owner, nil))
		shopRow["status"] = "0"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", sa), a.owner, shopRow))
		skuRow["status"] = "0"
		s.ok(s.req("PUT", "/erp/sku", a.owner, skuRow))
		whRow = data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		whRow["status"] = "0"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, whRow))
	})
	run("tenant_and_live_shop_warehouse_scope_cover_legacy_vouchers_ledger_and_SR", func(t *testing.T) {
		deny(s.req("GET", fmt.Sprintf("/erp/package/%d", shippedPackage), b.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", shippedOrder), b.owner, nil))
		deny(s.req("GET", "/erp/package/list", platform, nil))
		v := input([]string{"000A"}, []int64{1})
		v.ShopID = sb
		deny(s.req("POST", "/erp/order", a.owner, v))
		ctx, c := context.WithTimeout(tenantctx.System(context.Background()), 10*time.Second)
		defer c()
		var menus []model.SysMenu
		if e := repository.DB(ctx).Where("menu_id>=6000 AND menu_id<7600").Find(&menus).Error; e != nil {
			t.Fatal(e)
		}
		ids := []int64{}
		for _, m := range menus {
			ids = append(ids, m.MenuID)
		}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "履约测试员工", "roleKey": "fulfillment_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": ids}))
		r := s.req("GET", "/system/role/list?roleKey=fulfillment_operator", a.owner, nil)
		s.ok(r)
		roleID := r["rows"].([]any)[0].(map[string]any)["roleId"]
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "flowemployee", "nickName": "履约测试员工", "password": operationsPassword, "status": "0", "roleIds": []any{roleID}}))
		r = s.req("GET", "/system/user/list?userName=flowemployee", a.owner, nil)
		s.ok(r)
		uid := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		employee := s.loginAs(a.code, "flowemployee", operationsPassword)
		grant := func(kind string, values []int64) {
			path := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
			d := data(s.req("GET", path, a.owner, nil))
			s.ok(s.req("PUT", path, a.owner, map[string]any{"version": d["version"], kind + "Ids": values}))
		}
		grant("warehouse", []int64{wa})
		checkOrder := create([]string{"000A"}, []int64{1})
		checkPath := fmt.Sprintf("/erp/order/%d/readiness?warehouseId=%d", checkOrder, wa)
		deny(s.req("GET", checkPath, employee, nil))
		if data(s.req("GET", "/erp/order/workbench", employee, nil))["total"] != float64(0) {
			t.Fatal("warehouse-only workbench leaked shop counts")
		}
		for _, path := range []string{fmt.Sprintf("/erp/order/%d", shippedOrder), fmt.Sprintf("/erp/package/%d", shippedPackage), fmt.Sprintf("/erp/sales/%d", shippedDoc)} {
			deny(s.req("GET", path, employee, nil))
		}
		r = s.req("GET", fmt.Sprintf("/erp/ledger/list?documentId=%d", shippedDoc), employee, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("warehouse-only ledger bypass")
		}
		correction := map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "sourceDocumentId": shippedDoc, "remark": "仅库存纠错，不修改已发货订单", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}}
		deny(s.req("POST", "/erp/salesReturn", employee, correction))
		grant("shop", []int64{sa})
		s.ok(s.req("GET", fmt.Sprintf("/erp/package/%d", shippedPackage), employee, nil))
		if data(s.req("GET", checkPath, employee, nil))["ready"] != true {
			t.Fatal("authorized employee readiness rejected")
		}
		otherWarehouse := master(a, "warehouse", "FLOW-W2")
		grant("warehouse", []int64{otherWarehouse})
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d/readiness?warehouseId=%d", shippedOrder, otherWarehouse), employee, nil))
		grant("warehouse", []int64{wa})
		// The read-only review diagnostic must work without order:query, while
		// still withholding recipient data and enforcing both resource scopes.
		checkerMenus := []int64{}
		for _, m := range menus {
			if m.Perms != nil && (*m.Perms == "erp:order:list" || *m.Perms == "erp:order:review") {
				checkerMenus = append(checkerMenus, m.MenuID)
			}
		}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "仅审核检查", "roleKey": "readiness_checker", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": checkerMenus}))
		cr := s.req("GET", "/system/role/list?roleKey=readiness_checker", a.owner, nil)
		s.ok(cr)
		checkerRole := cr["rows"].([]any)[0].(map[string]any)["roleId"]
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "checker", "nickName": "仅审核检查", "password": operationsPassword, "status": "0", "roleIds": []any{checkerRole}}))
		cr = s.req("GET", "/system/user/list?userName=checker", a.owner, nil)
		s.ok(cr)
		checkerID := int64(cr["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		checker := s.loginAs(a.code, "checker", operationsPassword)
		for _, v := range []struct {
			kind string
			ids  []int64
		}{{"shop", []int64{sa}}, {"warehouse", []int64{wa}}} {
			path := fmt.Sprintf("/erp/%s/access/%d", v.kind, checkerID)
			d := data(s.req("GET", path, a.owner, nil))
			s.ok(s.req("PUT", path, a.owner, map[string]any{"version": d["version"], v.kind + "Ids": v.ids}))
		}
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", checkOrder), checker, nil))
		diagnostic := data(s.req("GET", checkPath, checker, nil))
		if diagnostic["ready"] != true {
			t.Fatal("review-only diagnostic rejected")
		}
		encoded, _ := json.Marshal(diagnostic)
		if strings.Contains(string(encoded), "receiver") || strings.Contains(string(encoded), "PRIVATE_") {
			t.Fatal("review-only diagnostic leaked recipient")
		}
		s.ok(s.req("GET", "/erp/order/workbench", checker, nil))
		cp := fmt.Sprintf("/erp/shop/access/%d", checkerID)
		cv := data(s.req("GET", cp, a.owner, nil))
		s.ok(s.req("PUT", cp, a.owner, map[string]any{"version": cv["version"], "shopIds": []int64{}}))
		deny(s.req("GET", checkPath, checker, nil))
		if data(s.req("GET", "/erp/order/workbench", checker, nil))["total"] != float64(0) {
			t.Fatal("revoked checker still sees counts")
		}
		s.req("POST", "/logout", checker, nil)
		// Distinct actors are essential: the actor lock alone must not be the
		// reason competing stock/order actions pass this test.
		raceID := create([]string{"000A"}, []int64{1})
		raceResult := review(raceID)
		racePackage := int64(raceResult["packageId"].(float64))
		logistics(racePackage)
		ov, pv := order(raceID)["version"], pack(racePackage)["version"]
		rs := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				v := token(ov)
				v["reason"] = "不同员工竞争取消"
				return "PUT", fmt.Sprintf("/erp/order/%d/cancel", raceID), a.owner, v
			}
			return "PUT", fmt.Sprintf("/erp/package/%d/ship", racePackage), employee, token(pv)
		})
		wins := 0
		for _, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("cross-actor ship/cancel divergence")
		}
		if pack(racePackage)["status"] != order(raceID)["status"] {
			t.Fatal("cross-actor package/order divergence")
		}
		// Two different actors and shops compete for the same last warehouse unit.
		otherShop := shop(a, "FLOW-S2")
		newItem := int64(data(s.req("POST", "/erp/shopItem", a.owner, map[string]any{"shopId": otherShop, "shopItemCode": "LAST-Z", "title": "最后一件", "status": "0"}))["itemId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", newItem), a.owner, map[string]any{"version": 1, "skuId": z}))
		grant("shop", []int64{sa, otherShop})
		inbound := data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: z, Quantity: 1}}}))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%.0f/confirm", inbound["documentId"]), a.owner, nil))
		one := create([]string{"000Z"}, []int64{1})
		other := input([]string{"LAST-Z"}, []int64{1})
		other.ShopID = otherShop
		two := int64(data(s.req("POST", "/erp/order", a.owner, other))["orderId"].(float64))
		rs = parallelJSON(s, 2, func(i int) (string, string, string, any) {
			id, user := one, a.owner
			if i == 1 {
				id, user = two, employee
			}
			return "PUT", fmt.Sprintf("/erp/order/%d/review", id), user, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": wa}
		})
		wins = 0
		winner := one
		for i, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
				if i == 1 {
					winner = two
				}
			}
		}
		if wins != 1 {
			t.Fatal("last unit cross-shop race", wins)
		}
		h, held := balance(z)
		if h != 1 || held != 1 {
			t.Fatal("last unit oversold")
		}
		cancel(winner)
		sr := int64(data(s.req("POST", "/erp/salesReturn", employee, correction))["documentId"].(float64))
		grant("shop", []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/salesReturn/%d", sr), employee, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", sr), employee, nil))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", sr), a.owner, nil))
		if order(shippedOrder)["status"] != "SHIPPED" {
			t.Fatal("manual correction rewrote order history")
		}
		r = s.req("GET", fmt.Sprintf("/erp/ledger/list?documentId=%d", sr), employee, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("correction ledger bypass")
		}
		grant("shop", []int64{sa})
		grant("warehouse", []int64{})
		deny(s.req("GET", checkPath, employee, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/package/%d", shippedPackage), employee, nil))
		s.ok(s.req("GET", fmt.Sprintf("/erp/order/%d", shippedOrder), employee, nil))
	})
	run("final_snapshots_and_transaction_audit_contain_no_recipient", func(t *testing.T) {
		d := data(s.req("GET", fmt.Sprintf("/erp/shopItem/%d", firstItem), a.owner, nil))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", firstItem), a.owner, map[string]any{"version": d["version"], "skuId": y}))
		o := order(shippedOrder)
		if o["lines"].([]any)[0].(map[string]any)["skuId"] != float64(x) {
			t.Fatal("historical SKU changed with mapping")
		}
		p := pack(shippedPackage)
		if p["lines"].([]any)[0].(map[string]any)["skuId"] != float64(x) {
			t.Fatal("package historical SKU changed")
		}
		ctx, c := context.WithTimeout(ctxFor(), 10*time.Second)
		defer c()
		var logs []model.SysOperLog
		if e := repository.DB(ctx).Where("title='ERP订单履约'").Find(&logs).Error; e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(logs)
		if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "00000123456789") {
			t.Fatal("audit leaked private fields")
		}
	})
}
