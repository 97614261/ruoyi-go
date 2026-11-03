package multitenant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

func TestERPReplenishment(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local fixtures required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("configuration unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("invalid configuration")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
		t.Fatal("nonlocal/original DB refused")
	}
	rh, _, e := net.SplitHostPort(cfg.Redis.Addr)
	if e != nil || (rh != "localhost" && rh != "127.0.0.1") || cfg.Redis.DB != 13 {
		t.Fatal("local Redis13 required")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("database unavailable")
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Logf("fresh replenishment tenants=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
	obj := func(v any) map[string]any { return v.(map[string]any) }
	data := func(v map[string]any) map[string]any { s.ok(v); return obj(v["data"]) }
	id := func(v map[string]any, k string) int64 { return int64(v[k].(float64)) }
	deny := func(v map[string]any) {
		t.Helper()
		if v["code"] == float64(200) {
			t.Fatal("unexpected success")
		}
	}
	master := func(f operationsFixture, kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "补货商品"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "补货仓库"
		}
		s.ok(s.req("POST", "/erp/"+kind, f.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, f.owner, nil)
		s.ok(r)
		return id(obj(r["rows"].([]any)[0]), kind+"Id")
	}
	w, wb := master(a, "warehouse", "REP-W"), master(b, "warehouse", "REP-W")
	sku := master(a, "sku", "REP-X")
	zero := master(a, "sku", "REP-Z")
	supplier := func(code string) int64 {
		return id(data(s.req("POST", "/erp/supplier", a.owner, map[string]any{"supplierCode": code, "supplierName": code, "status": "0"})), "supplierId")
	}
	sup, sup2 := supplier("REP-S1"), supplier("REP-S2")
	alert := func(x, n, version int64) {
		data(s.req("PUT", "/erp/stock/alert", a.owner, map[string]any{"warehouseId": w, "skuId": x, "thresholdQuantity": n, "version": version}))
	}
	alert(sku, 5, 0)
	alert(zero, 0, 0)
	list := func(token string, x int64) map[string]any {
		r := s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d&skuId=%d&pageSize=999", w, x), token, nil)
		s.ok(r)
		if r["total"] != float64(1) {
			t.Fatal("missing configured pair", r)
		}
		return obj(r["rows"].([]any)[0])
	}
	row := list(a.owner, sku)
	if row["neededQuantity"] != float64(6) || row["draftQuantity"] != float64(0) {
		t.Fatal(row)
	}
	if list(a.owner, zero)["neededQuantity"] != float64(1) {
		t.Fatal("zero threshold ignored")
	}
	for _, q := range []string{"", "?warehouseId=-1", "?warehouseId=abc"} {
		deny(s.req("GET", "/erp/replenishment/list"+q, a.owner, nil))
	}
	deny(s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d", wb), a.owner, nil))
	deny(s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d", w), b.owner, nil))
	deny(s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d", w), p, nil))
	makeInput := func(row map[string]any, quantity, supplier int64) map[string]any {
		return map[string]any{"warehouseId": w, "supplierId": supplier, "currency": "CNY", "requestKey": uuid.NewString(), "lines": []map[string]any{{"skuId": sku, "quantity": quantity, "unitPrice": "0"}}, "basis": []map[string]any{{"skuId": sku, "basis": row["basis"]}}}
	}
	manual := data(s.req("POST", "/erp/purchase", a.owner, makeInput(row, 2, sup)))
	manualID := id(manual, "purchaseId")
	deny(s.req("POST", "/erp/replenishment", a.owner, makeInput(row, 4, sup)))
	row = list(a.owner, sku)
	if row["draftQuantity"] != float64(2) || row["neededQuantity"] != float64(4) {
		t.Fatal(row)
	}
	body := makeInput(row, 4, sup)
	created := data(s.req("POST", "/erp/replenishment", a.owner, body))
	pid := id(created, "purchaseId")
	if created["status"] != "DRAFT" || data(s.req("POST", "/erp/replenishment", a.owner, body))["existing"] != true {
		t.Fatal("lost-response replay failed")
	}
	if list(a.owner, sku)["neededQuantity"] != float64(0) {
		t.Fatal("draft did not cover recommendation")
	}
	cancel := func(id int64) {
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/cancel", id), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "reason": "补货验收取消草稿"}))
	}
	cancel(manualID)
	cancel(pid)
	row = list(a.owner, sku)
	if row["neededQuantity"] != float64(6) {
		t.Fatal("canceled draft still counted")
	}
	results := parallelJSON(s, 8, func(i int) (string, string, string, any) {
		supplier := sup
		if i%2 == 1 {
			supplier = sup2
		}
		return "POST", "/erp/replenishment", a.owner, makeInput(row, 6, supplier)
	})
	success := 0
	for _, r := range results {
		if r.err != nil {
			t.Fatal("concurrent transport failure")
		}
		if r.code == 200 {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent stale suggestions created %d drafts", success)
	}
	r := s.req("GET", "/erp/purchase/list?status=DRAFT", a.owner, nil)
	s.ok(r)
	if r["total"] != float64(1) {
		t.Fatal(r)
	}
	pid = id(obj(r["rows"].([]any)[0]), "purchaseId")
	s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", pid), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
	row = list(a.owner, sku)
	if row["draftQuantity"] != float64(0) || row["incomingQuantity"] != float64(6) || row["neededQuantity"] != float64(0) {
		t.Fatal(row)
	}
	detail := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", pid), a.owner, nil))
	line := id(obj(detail["lines"].([]any)[0]), "lineId")
	s.ok(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", pid), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString(), "lines": []map[string]any{{"purchaseLineId": line, "quantity": 2}}}))
	row = list(a.owner, sku)
	if row["available"] != float64(2) || row["incomingQuantity"] != float64(4) || row["neededQuantity"] != float64(0) {
		t.Fatal("partial receipt double counted", row)
	}
	s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/close", pid), a.owner, map[string]any{"version": 3, "requestKey": uuid.NewString(), "reason": "补货验收关闭余量"}))
	row = list(a.owner, sku)
	if row["available"] != float64(2) || row["incomingQuantity"] != float64(0) || row["neededQuantity"] != float64(4) {
		t.Fatal("closed remainder still counted", row)
	}
	// A real employee gets exactly existing list/add permissions and one warehouse.
	ctx, cancelCtx := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), time.Minute)
	defer cancelCtx()
	var menus []model.SysMenu
	if e := repository.DB(ctx).Where("perms IN ?", []string{"erp:purchase:list", "erp:purchase:add", "erp:stock:list"}).Find(&menus).Error; e != nil || len(menus) != 3 {
		t.Fatal("fixture permissions unavailable", e)
	}
	ids := []int64{}
	for _, m := range menus {
		ids = append(ids, m.MenuID)
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "补货专员", "roleKey": "replenisher", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": ids}))
	r = s.req("GET", "/system/role/list?roleKey=replenisher", a.owner, nil)
	s.ok(r)
	rid := obj(r["rows"].([]any)[0])["roleId"]
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "replenisher", "nickName": "补货专员", "password": operationsPassword, "status": "0", "roleIds": []any{rid}}))
	r = s.req("GET", "/system/user/list?userName=replenisher", a.owner, nil)
	s.ok(r)
	uid := id(obj(r["rows"].([]any)[0]), "userId")
	employee := s.loginAs(a.code, "replenisher", operationsPassword)
	grant := func(ids []int64) {
		url := fmt.Sprintf("/erp/warehouse/access/%d", uid)
		v := data(s.req("GET", url, a.owner, nil))
		s.ok(s.req("PUT", url, a.owner, map[string]any{"version": v["version"], "warehouseIds": ids}))
	}
	t.Cleanup(func() { grant([]int64{}); s.req("POST", "/logout", employee, nil) })
	deny(s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d", w), employee, nil))
	grant([]int64{w})
	row = list(employee, sku)
	raw, _ := json.Marshal(row)
	for _, private := range []string{"unitPrice", "orderedAmount", "supplierName", "supplierId", "phone"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("recommendation price/contact leak")
		}
	}
	body = makeInput(row, 1, sup)
	data(s.req("POST", "/erp/replenishment", employee, body))
	grant([]int64{})
	deny(s.req("POST", "/erp/replenishment", employee, body))
	deny(s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d", w), employee, nil))
	alert(sku, 6, 1)
	deny(s.req("POST", "/erp/replenishment", a.owner, makeInput(row, 1, sup)))
	s.ok(s.req("PUT", "/erp/stock/alert", a.owner, map[string]any{"warehouseId": w, "skuId": zero, "thresholdQuantity": nil, "version": 1}))
	r = s.req("GET", fmt.Sprintf("/erp/replenishment/list?warehouseId=%d&skuId=%d", w, zero), a.owner, nil)
	s.ok(r)
	if r["total"] != float64(0) {
		t.Fatal("cleared alert retained")
	}
	var st model.ERPStock
	if e := repository.DB(ctx).Where("warehouse_id=? AND sku_id=?", w, sku).Take(&st).Error; e != nil || st.OnHand != 2 || st.Reserved != 0 {
		t.Fatal("recommendation changed physical stock", e)
	}
}
