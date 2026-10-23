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

func TestERPPurchases(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("invalid local config")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
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
	run := func(name string, f func(*testing.T)) {
		t.Run(name, func(t *testing.T) { before := s.t; s.t = t; defer func() { s.t = before }(); f(t) })
	}
	data := func(r map[string]any) map[string]any { s.ok(r); return r["data"].(map[string]any) }
	deny := func(r map[string]any) {
		s.t.Helper()
		if r["code"] == float64(200) {
			s.t.Fatal("unexpected success")
		}
	}
	master := func(f operationsFixture, kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "采购SKU"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "采购仓库"
		}
		s.ok(s.req("POST", "/erp/"+kind, f.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, f.owner, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)[kind+"Id"].(float64))
	}
	supplier := func(f operationsFixture) int64 {
		return int64(data(s.req("POST", "/erp/supplier", f.owner, map[string]any{"supplierCode": "P-SUP", "supplierName": "采购供应商", "status": "0", "phone": "PRIVATE_PURCHASE_CONTACT"}))["supplierId"].(float64))
	}
	wa, wb := master(a, "warehouse", "P-W"), master(b, "warehouse", "P-W")
	x, y, z := master(a, "sku", "P-X"), master(a, "sku", "P-Y"), master(b, "sku", "P-X")
	sa, sb := supplier(a), supplier(b)
	input := func(sup, wh, sku int64) model.ERPPurchaseWrite {
		return model.ERPPurchaseWrite{SupplierID: sup, WarehouseID: wh, Currency: "usd", RequestKey: uuid.NewString(), Remark: "PRIVATE_PURCHASE_REMARK", Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: 3, UnitPrice: "0.1001"}}}
	}
	create := func(in model.ERPPurchaseWrite) int64 {
		return int64(data(s.req("POST", "/erp/purchase", a.owner, in))["purchaseId"].(float64))
	}
	detail := func(id int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", id), a.owner, nil))
	}
	action := func(id int64, verb string, version int64) map[string]any {
		v := map[string]any{"version": version, "requestKey": uuid.NewString()}
		if verb != "confirm" {
			v["reason"] = "PRIVATE_PURCHASE_REASON"
		}
		return s.req("PUT", fmt.Sprintf("/erp/purchase/%d/%s", id, verb), a.owner, v)
	}
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 3*time.Minute)
	defer cancel()
	run("decimal_types_zero_bounds_and_server_fields", func(t *testing.T) {
		in := input(sa, wa, x)
		in.Lines = append(in.Lines, model.ERPPurchaseLineInput{SkuID: y, Quantity: 1, UnitPrice: "0"})
		p := create(in)
		v := detail(p)
		if v["orderedAmount"] != "0.3003" || v["currency"] != "USD" || v["status"] != "DRAFT" {
			t.Fatal(v)
		}
		for _, raw := range v["lines"].([]any) {
			l := raw.(map[string]any)
			if _, ok := l["unitPrice"].(string); !ok {
				t.Fatal("float price")
			}
			if l["receivedQuantity"] != float64(0) {
				t.Fatal("client receipt")
			}
		}
		for _, price := range []any{0, 1.5, nil, "-1", "1e2", "1.00001", "9999999999999999.9999"} {
			deny(s.req("POST", "/erp/purchase", a.owner, map[string]any{"supplierId": sa, "warehouseId": wa, "currency": "USD", "requestKey": uuid.NewString(), "lines": []any{map[string]any{"skuId": x, "quantity": 3, "unitPrice": price}}}))
		}
		for _, price := range []string{"0", "9999999999999999.9999"} {
			q := input(sa, wa, x)
			q.Lines[0].Quantity = 1
			q.Lines[0].UnitPrice = price
			id := create(q)
			expected := price
			if price == "0" {
				expected = "0.0000"
			}
			if detail(id)["orderedAmount"] != expected {
				t.Fatal("exact maximum/zero not retained")
			}
		}
		q := input(sa, wa, x)
		q.Lines = []model.ERPPurchaseLineInput{{SkuID: x, Quantity: 1, UnitPrice: "9999999999999999.9999"}, {SkuID: y, Quantity: 1, UnitPrice: "0.0001"}}
		deny(s.req("POST", "/erp/purchase", a.owner, q))
		q = input(sa, wa, x)
		q.Lines = append(q.Lines, q.Lines[0])
		deny(s.req("POST", "/erp/purchase", a.owner, q))
		forged := map[string]any{"supplierId": sa, "warehouseId": wa, "currency": "USD", "requestKey": uuid.NewString(), "tenantId": b.tenant.TenantID, "purchaseId": 123, "status": "CLOSED", "version": 88, "orderedAmount": "999", "purchaseNo": "FORGED", "lines": []any{map[string]any{"skuId": x, "quantity": 1, "unitPrice": "0", "receivedQuantity": 77, "lineAmount": "999"}}}
		id := int64(data(s.req("POST", "/erp/purchase", a.owner, forged))["purchaseId"].(float64))
		v = detail(id)
		if v["status"] != "DRAFT" || v["version"] != float64(1) || v["orderedAmount"] != "0.0000" || v["purchaseNo"] == "FORGED" {
			t.Fatal("server fields overwritten")
		}
	})
	run("create_edit_action_replays_return_original_result", func(t *testing.T) {
		in := input(sa, wa, x)
		p := create(in)
		edited := model.ERPPurchaseEdit{ERPPurchaseWrite: in, Version: 1}
		edited.RequestKey = uuid.NewString()
		edited.Lines = []model.ERPPurchaseLineInput{{SkuID: x, Quantity: 7, UnitPrice: "0.2"}}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d", p), a.owner, edited))
		old := data(s.req("POST", "/erp/purchase", a.owner, in))
		if old["version"] != float64(1) || old["status"] != "DRAFT" || old["existing"] != true {
			t.Fatal("create replay returned latest instead of original")
		}
		confirm := map[string]any{"version": 2, "requestKey": uuid.NewString()}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, confirm))
		replay := data(s.req("PUT", fmt.Sprintf("/erp/purchase/%d", p), a.owner, edited))
		if replay["version"] != float64(2) || replay["status"] != "DRAFT" || replay["existing"] != true {
			t.Fatal("edit replay lost original")
		}
		s.ok(action(p, "close", 3))
		replay = data(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, confirm))
		if replay["version"] != float64(3) || replay["status"] != "CONFIRMED" {
			t.Fatal("action replay lost original")
		}
		confirm["version"] = 3
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, confirm))
		in.Lines[0].Quantity = 99
		deny(s.req("POST", "/erp/purchase", a.owner, in))
	})
	run("tenant_platform_isolation_and_no_stock_side_effects", func(t *testing.T) {
		p := create(input(sa, wa, x))
		for _, url := range []string{fmt.Sprintf("/erp/purchase/%d", p), fmt.Sprintf("/erp/purchase/%d/events", p)} {
			deny(s.req("GET", url, b.owner, nil))
			deny(s.req("GET", url, platform, nil))
		}
		deny(s.req("GET", "/erp/purchase/list", platform, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/cancel", p), b.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "reason": "foreign"}))
		deny(s.req("POST", "/erp/purchase", a.owner, input(sb, wa, x)))
		deny(s.req("POST", "/erp/purchase", a.owner, input(sa, wb, x)))
		deny(s.req("POST", "/erp/purchase", a.owner, input(sa, wa, z)))
		s.ok(s.req("POST", "/erp/purchase", b.owner, input(sb, wb, z)))
		s.ok(action(p, "confirm", 1))
		s.ok(action(p, "cancel", 2))
		for _, table := range []any{&model.ERPStock{}, &model.ERPLedger{}, &model.ERPDocument{}} {
			var n int64
			if e := repository.DB(ctx).Model(table).Count(&n).Error; e != nil || n != 0 {
				t.Fatal("purchase touched stock", e, n)
			}
		}
	})
	run("snapshot_freeze_unit_reference_and_disabled_masters", func(t *testing.T) {
		p := create(input(sa, wa, y))
		sku := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", y), a.owner, nil))
		sku["unit"] = "箱"
		deny(s.req("PUT", "/erp/sku", a.owner, sku))
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", y), a.owner, nil))
		sku["unit"] = "件"
		sku["productName"] = "确认时商品名"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		sv := data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sv["supplierName"] = "确认时供应商名"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sv))
		s.ok(action(p, "confirm", 1))
		v := detail(p)
		if v["supplierName"] != "确认时供应商名" || v["lines"].([]any)[0].(map[string]any)["productName"] != "确认时商品名" {
			t.Fatal("confirmation snapshots stale")
		}
		sku["productName"] = "之后商品名"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		if detail(p)["lines"].([]any)[0].(map[string]any)["productName"] != "确认时商品名" {
			t.Fatal("frozen name drifted")
		}
		draft := create(input(sa, wa, x))
		sv = data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sv["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sv))
		deny(s.req("POST", "/erp/purchase", a.owner, input(sa, wa, x)))
		deny(action(draft, "confirm", 1))
		s.ok(action(draft, "cancel", 1))
		s.ok(action(p, "close", 2))
		sv = data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sv["status"] = "0"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sv))
	})
	run("state_machine_fixed_identity_and_concurrent_versions", func(t *testing.T) {
		in := input(sa, wa, x)
		p := create(in)
		deny(action(p, "close", 1))
		changed := model.ERPPurchaseEdit{ERPPurchaseWrite: in, Version: 1}
		changed.RequestKey = uuid.NewString()
		changed.WarehouseID = wb
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d", p), a.owner, changed))
		changed.WarehouseID = wa
		changed.SupplierID = sb
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d", p), a.owner, changed))
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}
		})
		success := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				success++
			}
		}
		if success != 1 {
			t.Fatal("CAS winners", success)
		}
		deny(action(p, "confirm", 2))
		edit := model.ERPPurchaseEdit{ERPPurchaseWrite: in, Version: 2}
		edit.RequestKey = uuid.NewString()
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d", p), a.owner, edit))
		s.ok(action(p, "close", 2))
		deny(action(p, "cancel", 3))
		deny(action(p, "close", 3))
		in = input(sa, wa, x)
		p = create(in)
		results = parallelJSON(s, 2, func(i int) (string, string, string, any) {
			verb := "cancel"
			if i == 0 {
				verb = "confirm"
			}
			v := map[string]any{"version": 1, "requestKey": uuid.NewString()}
			if verb == "cancel" {
				v["reason"] = "competition"
			}
			return "PUT", fmt.Sprintf("/erp/purchase/%d/%s", p, verb), a.owner, v
		})
		success = 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				success++
			}
		}
		if success != 1 {
			t.Fatal("state race winners", success)
		}
	})
	run("duplicate_create_and_same_key_action_are_exactly_once", func(t *testing.T) {
		in := input(sa, wa, x)
		results := parallelJSON(s, 4, func(i int) (string, string, string, any) { return "POST", "/erp/purchase", a.owner, in })
		for _, r := range results {
			if r.err != nil || r.code != 200 {
				t.Fatal("same create failed", r.err, r.code)
			}
		}
		id := int64(data(s.req("POST", "/erp/purchase", a.owner, in))["purchaseId"].(float64))
		key := uuid.NewString()
		results = parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/purchase/%d/confirm", id), a.owner, map[string]any{"version": 1, "requestKey": key}
		})
		for _, r := range results {
			if r.err != nil || r.code != 200 {
				t.Fatal("same confirm failed", r.err, r.code)
			}
		}
		var n int64
		if e := repository.DB(ctx).Model(&model.ERPPurchase{}).Where("create_request_key=?", in.RequestKey).Count(&n).Error; e != nil || n != 1 {
			t.Fatal("duplicate headers", n, e)
		}
		if e := repository.DB(ctx).Model(&model.ERPPurchaseEvent{}).Where("purchase_id=?", id).Count(&n).Error; e != nil || n != 2 {
			t.Fatal("duplicate events", n, e)
		}
		if detail(id)["version"] != float64(2) {
			t.Fatal("repeated mutation")
		}
	})
	run("partial_and_terminal_state_boundaries_with_synthetic_rows", func(t *testing.T) {
		// Seed only this new fixture to exercise future partial/completed boundaries.
		// This is NOT a receiving operation or proof that receiving is implemented.
		id := create(input(sa, wa, x))
		s.ok(action(id, "confirm", 1))
		if e := repository.DB(ctx).Model(&model.ERPPurchase{}).Where("purchase_id=?", id).Update("status", "PARTIAL_RECEIVED").Error; e != nil {
			t.Fatal(e)
		}
		if e := repository.DB(ctx).Model(&model.ERPPurchaseLine{}).Where("purchase_id=?", id).Update("received_quantity", 1).Error; e != nil {
			t.Fatal(e)
		}
		deny(action(id, "cancel", 2))
		s.ok(action(id, "close", 2))
		v := detail(id)
		l := v["lines"].([]any)[0].(map[string]any)
		if l["receivedQuantity"] != float64(1) || l["orderedQuantity"] != float64(3) || v["status"] != "CLOSED" {
			t.Fatal("close fabricated receipt quantities")
		}
		for _, status := range []string{"COMPLETED", "CLOSED", "CANCELLED"} {
			id = create(input(sa, wa, x))
			if e := repository.DB(ctx).Model(&model.ERPPurchase{}).Where("purchase_id=?", id).Update("status", status).Error; e != nil {
				t.Fatal(e)
			}
			for _, verb := range []string{"confirm", "cancel", "close"} {
				deny(action(id, verb, 1))
			}
		}
	})
	run("audit_and_event_faults_rollback_every_write", func(t *testing.T) {
		for _, faultTable := range []string{"sys_oper_log", "erp_purchase_event"} {
			in := input(sa, wa, x)
			p := create(in)
			before := detail(p)
			fault := errors.New("injected purchase failure")
			hook := "purchase:fault"
			db := repository.DB(ctx)
			if e := db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == faultTable {
					tx.AddError(fault)
				}
			}); e != nil {
				t.Fatal(e)
			}
			bad := input(sa, wa, x)
			_, createErr := service.CreateERPPurchase(ctx, bad, "purchase-fault")
			edit := model.ERPPurchaseEdit{ERPPurchaseWrite: in, Version: 1}
			edit.RequestKey = uuid.NewString()
			edit.Lines = []model.ERPPurchaseLineInput{{SkuID: y, Quantity: 9, UnitPrice: "2"}}
			_, editErr := service.EditERPPurchase(ctx, p, edit, "purchase-fault")
			_, actErr := service.ActERPPurchase(ctx, p, "CONFIRM", model.ERPPurchaseAction{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}}, "purchase-fault")
			db.Callback().Create().Remove(hook)
			if !errors.Is(createErr, fault) || !errors.Is(editErr, fault) || !errors.Is(actErr, fault) {
				t.Fatal("fault path not reached", createErr, editErr, actErr)
			}
			after := detail(p)
			aJSON, _ := json.Marshal(before)
			bJSON, _ := json.Marshal(after)
			if string(aJSON) != string(bJSON) {
				t.Fatal("failed write partially changed header/lines")
			}
			var n int64
			if e := db.Model(&model.ERPPurchase{}).Where("create_request_key=?", bad.RequestKey).Count(&n).Error; e != nil || n != 0 {
				t.Fatal("failed create persisted", e)
			}
		}
	})
	run("line_and_header_faults_rollback_draft_and_confirm", func(t *testing.T) {
		in := input(sa, wa, x)
		id := create(in)
		before := detail(id)
		db := repository.DB(ctx)
		fault := errors.New("injected purchase line/header failure")
		hook := "purchase:line-fault"
		if e := db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "erp_purchase_order_line" {
				tx.AddError(fault)
			}
		}); e != nil {
			t.Fatal(e)
		}
		edit := model.ERPPurchaseEdit{ERPPurchaseWrite: in, Version: 1}
		edit.RequestKey = uuid.NewString()
		edit.Lines = []model.ERPPurchaseLineInput{{SkuID: y, Quantity: 9, UnitPrice: "2"}}
		_, e := service.EditERPPurchase(ctx, id, edit, "line-fault")
		db.Callback().Create().Remove(hook)
		if !errors.Is(e, fault) {
			t.Fatal("line fault not reached", e)
		}
		hook = "purchase:header-fault"
		if e := db.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "erp_purchase_order" {
				tx.AddError(fault)
			}
		}); e != nil {
			t.Fatal(e)
		}
		_, e = service.ActERPPurchase(ctx, id, "CONFIRM", model.ERPPurchaseAction{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}}, "header-fault")
		db.Callback().Update().Remove(hook)
		if !errors.Is(e, fault) {
			t.Fatal("header fault not reached", e)
		}
		after := detail(id)
		aJSON, _ := json.Marshal(before)
		bJSON, _ := json.Marshal(after)
		if string(aJSON) != string(bJSON) {
			t.Fatal("partial line/header write")
		}
	})
	run("employee_live_scope_replay_and_function_revocation", func(t *testing.T) {
		role := map[string]any{"roleName": "采购测试员工", "roleKey": "purchase_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7700, 7701, 7702, 7703, 7704, 7705, 7706}}
		s.ok(s.req("POST", "/system/role", a.owner, role))
		rr := s.req("GET", "/system/role/list?roleKey=purchase_operator", a.owner, nil)
		s.ok(rr)
		rid := rr["rows"].([]any)[0].(map[string]any)["roleId"]
		role["roleId"] = rid
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "purchaseemployee", "nickName": "采购测试员工", "password": operationsPassword, "status": "0", "roleIds": []any{rid}}))
		rr = s.req("GET", "/system/user/list?userName=purchaseemployee", a.owner, nil)
		s.ok(rr)
		uid := int64(rr["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		employee := s.loginAs(a.code, "purchaseemployee", operationsPassword)
		grant := func(ids []int64) {
			path := fmt.Sprintf("/erp/warehouse/access/%d", uid)
			v := data(s.req("GET", path, a.owner, nil))
			s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], "warehouseIds": ids}))
		}
		in := input(sa, wa, x)
		deny(s.req("POST", "/erp/purchase", employee, in))
		rr = s.req("GET", "/erp/purchase/list", employee, nil)
		s.ok(rr)
		if rr["total"] != float64(0) {
			t.Fatal("ungranted rows")
		}
		grant([]int64{wa})
		p := int64(data(s.req("POST", "/erp/purchase", employee, in))["purchaseId"].(float64))
		deny(s.req("POST", "/erp/purchase", a.owner, in))
		grant([]int64{})
		deny(s.req("POST", "/erp/purchase", employee, in))
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), employee, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), employee, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		grant([]int64{wa})
		s.ok(s.req("POST", "/erp/purchase", employee, in))
		role["menuIds"] = []int64{6000, 7700}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), employee, nil))
		deny(s.req("POST", "/erp/purchase", employee, in))
		rr = s.req("GET", "/erp/purchase/list", employee, nil)
		s.ok(rr)
		raw, _ := json.Marshal(rr)
		if strings.Contains(string(raw), "orderedAmount") || strings.Contains(string(raw), "PRIVATE_PURCHASE") {
			t.Fatal("list-only private fields")
		}
		role["menuIds"] = []int64{6000}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(s.req("GET", "/erp/purchase/list", employee, nil))
		grant([]int64{})
	})
	run("pagination_minimal_event_audit_headers_no_extra_routes", func(t *testing.T) {
		p := create(input(sa, wa, x))
		r := s.req("GET", "/erp/purchase/list?pageSize=999999&orderByColumn=orderedAmount%20desc", a.owner, nil)
		s.ok(r)
		if len(r["rows"].([]any)) > 100 {
			t.Fatal("page cap")
		}
		r = s.req("GET", fmt.Sprintf("/erp/purchase/%d/events?pageSize=1", p), a.owner, nil)
		s.ok(r)
		if len(r["rows"].([]any)) != 1 {
			t.Fatal("event pagination")
		}
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "PRIVATE_PURCHASE") || strings.Contains(string(raw), "requestHash") || strings.Contains(string(raw), "resultJSON") {
			t.Fatal("private event")
		}
		var logs []model.SysOperLog
		if e := repository.DB(ctx).Where("method LIKE 'erp.purchase.%'").Find(&logs).Error; e != nil || len(logs) < 1 {
			t.Fatal(e)
		}
		raw, _ = json.Marshal(logs)
		if strings.Contains(string(raw), "PRIVATE_PURCHASE") {
			t.Fatal("private audit")
		}
		for _, path := range []string{"/erp/purchase/list", fmt.Sprintf("/erp/purchase/%d", p), fmt.Sprintf("/erp/purchase/%d/events", p)} {
			req, _ := http.NewRequest("GET", s.base+path, nil)
			req.Header.Set("Authorization", "Bearer "+a.owner)
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable purchase")
			}
		}
		for _, v := range []struct{ method, path string }{{"DELETE", fmt.Sprintf("/erp/purchase/%d", p)}, {"POST", "/erp/purchase/export"}} {
			req, _ := http.NewRequest(v.method, s.base+v.path, nil)
			req.Header.Set("Authorization", "Bearer "+a.owner)
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.StatusCode != 404 {
				t.Fatal("unexpected delete/export/receiving route", res.StatusCode)
			}
		}
	})
}
