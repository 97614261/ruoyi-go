package multitenant_test

import (
	"context"
	"crypto/sha256"
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

func TestERPPurchaseReceiving(t *testing.T) {
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
		t.Fatal("invalid DSN")
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
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { prev := s.t; s.t = t; defer func() { s.t = prev }(); fn(t) })
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
			v["productName"] = "收货测试商品"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "收货仓库"
		}
		s.ok(s.req("POST", "/erp/"+kind, f.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, f.owner, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)[kind+"Id"].(float64))
	}
	supplier := func(f operationsFixture) int64 {
		return int64(data(s.req("POST", "/erp/supplier", f.owner, map[string]any{"supplierCode": "REC-SUP", "supplierName": "收货供应商", "status": "0", "phone": "PRIVATE_RECEIVING_CONTACT"}))["supplierId"].(float64))
	}
	wa, wb := master(a, "warehouse", "REC-W"), master(b, "warehouse", "REC-W")
	x, y, z := master(a, "sku", "REC-X"), master(a, "sku", "REC-Y"), master(b, "sku", "REC-X")
	sa, sb := supplier(a), supplier(b)
	create := func(f operationsFixture, sup, wh, sku, qty int64) int64 {
		v := model.ERPPurchaseWrite{SupplierID: sup, WarehouseID: wh, Currency: "USD", RequestKey: uuid.NewString(), Remark: "PRIVATE_RECEIVING_REMARK", Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: qty, UnitPrice: "12345.6789"}}}
		id := int64(data(s.req("POST", "/erp/purchase", f.owner, v))["purchaseId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", id), f.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		return id
	}
	detail := func(id int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", id), a.owner, nil))
	}
	line := func(id int64) int64 {
		return int64(detail(id)["lines"].([]any)[0].(map[string]any)["lineId"].(float64))
	}
	input := func(l, qty, version int64) model.ERPPurchaseReceiveInput {
		return model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: version, RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: l, Quantity: qty}}}
	}
	receive := func(token string, id int64, in model.ERPPurchaseReceiveInput) map[string]any {
		return s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", id), token, in)
	}
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 5*time.Minute)
	defer cancel()
	balance := func(sku int64) (int64, int64) {
		var st model.ERPStock
		e := repository.DB(ctx).Where("warehouse_id=? AND sku_id=?", wa, sku).Take(&st).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return 0, 0
		}
		if e != nil {
			t.Fatal(e)
		}
		return st.OnHand, st.Reserved
	}
	role := map[string]any{"roleName": "仅采购收货", "roleKey": "receipt_only", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7800, 7801, 7900}}
	s.ok(s.req("POST", "/system/role", a.owner, role))
	rr := s.req("GET", "/system/role/list?roleKey=receipt_only", a.owner, nil)
	s.ok(rr)
	rid := rr["rows"].([]any)[0].(map[string]any)["roleId"]
	role["roleId"] = rid
	users := []string{}
	uids := []int64{}
	grant := func(uid int64, ids []int64) {
		path := fmt.Sprintf("/erp/warehouse/access/%d", uid)
		v := data(s.req("GET", path, a.owner, nil))
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], "warehouseIds": ids}))
	}
	for _, name := range []string{"receiverone", "receivertwo"} {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []any{rid}}))
		r := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(r)
		uid := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		uids = append(uids, uid)
		grant(uid, []int64{wa})
		users = append(users, s.loginAs(a.code, name, operationsPassword))
	}
	run("partial_then_complete_increases_hand_not_reserved", func(t *testing.T) {
		stock := data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 20}}}))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%.0f/confirm", stock["documentId"]), a.owner, nil))
		sale := data(s.req("POST", "/erp/sales", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 5}}}))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%.0f/confirm", sale["documentId"]), a.owner, nil))
		p := create(a, sa, wa, x, 10)
		l := line(p)
		first := data(receive(users[0], p, input(l, 4, 2)))
		if first["status"] != "PARTIAL_RECEIVED" || first["version"] != float64(3) {
			t.Fatal("partial state")
		}
		if h, r := balance(x); h != 24 || r != 5 {
			t.Fatal("partial balance", h, r)
		}
		second := data(receive(users[1], p, input(l, 6, 3)))
		if second["status"] != "COMPLETED" || second["version"] != float64(4) {
			t.Fatal("complete state")
		}
		if h, r := balance(x); h != 30 || r != 5 {
			t.Fatal("complete balance", h, r)
		}
		deny(receive(users[0], p, input(l, 1, 4)))
		d := detail(p)
		if d["lines"].([]any)[0].(map[string]any)["receivedQuantity"] != float64(10) {
			t.Fatal("progress mismatch")
		}
		for _, r := range []map[string]any{first, second} {
			rc := data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%.0f", r["receiptId"]), users[0], nil))
			if rc["stockDocumentId"] != r["stockDocumentId"] {
				t.Fatal("wrong voucher link")
			}
			var doc model.ERPDocument
			if e := repository.DB(ctx).Where("document_id=?", r["stockDocumentId"]).Take(&doc).Error; e != nil || doc.Status != "POSTED" || doc.Kind != "IN" || doc.WarehouseID != wa {
				t.Fatal("unposted/mismatched IN", e)
			}
		}
	})
	run("partial_close_keeps_received_fact", func(t *testing.T) {
		p := create(a, sa, wa, y, 10)
		l := line(p)
		before, _ := balance(y)
		s.ok(receive(users[0], p, input(l, 4, 2)))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/close", p), a.owner, map[string]any{"version": 3, "requestKey": uuid.NewString(), "reason": "余量不再采购"}))
		deny(receive(users[0], p, input(l, 6, 4)))
		if h, _ := balance(y); h != before+4 {
			t.Fatal("close changed stock")
		}
		d := detail(p)
		if d["status"] != "CLOSED" || d["lines"].([]any)[0].(map[string]any)["receivedQuantity"] != float64(4) {
			t.Fatal("close fabricated progress")
		}
	})
	run("receipt_original_result_and_actor_fingerprint", func(t *testing.T) {
		p := create(a, sa, wa, x, 2)
		l := line(p)
		in := input(l, 1, 2)
		first := data(receive(users[0], p, in))
		s.ok(receive(users[0], p, input(l, 1, 3)))
		again := data(receive(users[0], p, in))
		if again["existing"] != true || again["receiptId"] != first["receiptId"] || again["stockDocumentId"] != first["stockDocumentId"] || again["version"] != float64(3) || again["status"] != "PARTIAL_RECEIVED" {
			t.Fatal("not original result")
		}
		deny(receive(users[1], p, in))
		in.Lines[0].Quantity = 2
		deny(receive(users[0], p, in))
	})
	run("two_actors_last_remainder_and_close_race", func(t *testing.T) {
		p := create(a, sa, wa, x, 10)
		l := line(p)
		s.ok(receive(users[0], p, input(l, 4, 2)))
		before, _ := balance(x)
		rs := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), users[i], input(l, 6, 3)
		})
		n := 0
		for _, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				n++
			}
		}
		if n != 1 {
			t.Fatal("overreceive winners", n)
		}
		if h, _ := balance(x); h != before+6 {
			t.Fatal("double stock")
		}
		p = create(a, sa, wa, x, 3)
		l = line(p)
		before, _ = balance(x)
		rs = parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				return "POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), users[0], input(l, 3, 2)
			}
			return "PUT", fmt.Sprintf("/erp/purchase/%d/close", p), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString(), "reason": "收货与关闭竞争"}
		})
		n = 0
		received := false
		for i, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				n++
				received = i == 0
			}
		}
		if n != 1 {
			t.Fatal("race winners", n)
		}
		expected := before
		if received {
			expected += 3
		}
		if h, _ := balance(x); h != expected {
			t.Fatal("close race partial commit")
		}
	})
	run("same_key_concurrency_one_receipt_and_ledger", func(t *testing.T) {
		p := create(a, sa, wa, x, 3)
		l := line(p)
		in := input(l, 3, 2)
		before, _ := balance(x)
		rs := parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), users[0], in
		})
		for _, r := range rs {
			if r.err != nil || r.code != 200 {
				t.Fatal("same-key request failed", r.err, r.code)
			}
		}
		if h, _ := balance(x); h != before+3 {
			t.Fatal("duplicate stock")
		}
		var n int64
		if e := repository.DB(ctx).Model(&model.ERPPurchaseReceipt{}).Where("purchase_id=?", p).Count(&n).Error; e != nil || n != 1 {
			t.Fatal("duplicate receipts", e, n)
		}
		r := data(receive(users[0], p, in))
		if e := repository.DB(ctx).Model(&model.ERPLedger{}).Where("document_id=?", r["stockDocumentId"]).Count(&n).Error; e != nil || n != 1 {
			t.Fatal("duplicate ledger", e, n)
		}
	})
	run("line_validation_foreign_ids_and_extra_fields", func(t *testing.T) {
		p, q := create(a, sa, wa, x, 2), create(a, sa, wa, y, 2)
		l := line(p)
		deny(receive(users[0], p, input(line(q), 1, 2)))
		deny(receive(users[0], p, input(l, 3, 2)))
		in := input(l, 1, 2)
		in.Lines = append(in.Lines, in.Lines[0])
		deny(receive(users[0], p, in))
		for _, v := range []any{0, -1, 1.5, "1", nil} {
			deny(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), users[0], map[string]any{"version": 2, "requestKey": uuid.NewString(), "lines": []any{map[string]any{"purchaseLineId": l, "quantity": v}}}))
		}
		s.ok(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), users[0], map[string]any{"version": 2, "requestKey": uuid.NewString(), "warehouseId": wb, "supplierId": sb, "stockDocumentId": 999, "status": "CLOSED", "lines": []any{map[string]any{"purchaseLineId": l, "quantity": 1, "skuId": z, "unitPrice": "1"}}}))
		if detail(p)["warehouseId"] != float64(wa) {
			t.Fatal("forged warehouse")
		}
	})
	run("supplier_disable_allows_confirmed_receipt_but_sku_warehouse_disable_blocks", func(t *testing.T) {
		p := create(a, sa, wa, y, 3)
		l := line(p)
		sv := data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sv["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sv))
		s.ok(receive(users[0], p, input(l, 1, 2)))
		sv = data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sv["status"] = "0"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sv))
		sku := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", y), a.owner, nil))
		sku["status"] = "1"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		deny(receive(users[0], p, input(l, 1, 3)))
		sku["status"] = "0"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		wh := data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		wh["status"] = "1"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, wh))
		deny(receive(users[0], p, input(l, 1, 3)))
		wh = data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		wh["status"] = "0"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, wh))
	})
	run("managed_IN_and_source_PR_cannot_bypass", func(t *testing.T) {
		p := create(a, sa, wa, y, 2)
		r := data(receive(users[0], p, input(line(p), 2, 2)))
		id := int64(r["stockDocumentId"].(float64))
		var doc model.ERPDocument
		if e := repository.DB(ctx).Where("document_id=?", id).Take(&doc).Error; e != nil {
			t.Fatal(e)
		}
		for _, verb := range []string{"confirm", "cancel"} {
			v := s.req("PUT", fmt.Sprintf("/erp/receipt/%d/%s", id, verb), a.owner, nil)
			deny(v)
			if !strings.Contains(v["msg"].(string), "托管") {
				t.Fatal("not managed boundary", v["msg"])
			}
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/receipt/%d", id), a.owner, map[string]any{"warehouseId": wa, "version": 2, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: y, Quantity: 2}}}))
		deny(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wa, "requestKey": doc.RequestKey, "lines": []model.ERPLineInput{{SkuID: y, Quantity: 2}}}))
		v := s.req("POST", "/erp/purchaseReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": id, "requestKey": uuid.NewString(), "remark": "不允许采购退货", "lines": []model.ERPLineInput{{SkuID: y, Quantity: 1}}})
		deny(v)
		if !strings.Contains(v["msg"].(string), "托管") {
			t.Fatal("source PR bypass")
		}
		s.ok(s.req("GET", fmt.Sprintf("/erp/receipt/%d", id), a.owner, nil))
	})
	run("every_receiving_failure_rolls_back_all_business_tables", func(t *testing.T) {
		tables := []struct{ name, pk string }{{"erp_purchase_order", "purchase_id"}, {"erp_purchase_order_line", "line_id"}, {"erp_purchase_receipt", "receipt_id"}, {"erp_purchase_receipt_line", "line_id"}, {"erp_purchase_event", "event_id"}, {"erp_document", "document_id"}, {"erp_document_line", "line_id"}, {"erp_stock", "stock_id"}, {"erp_stock_ledger", "ledger_id"}}
		fingerprint := func() map[string][32]byte {
			out := map[string][32]byte{}
			for _, v := range tables {
				var rows []map[string]any
				if e := repository.DB(ctx).Table(v.name).Order(v.pk).Find(&rows).Error; e != nil {
					t.Fatal(e)
				}
				raw, _ := json.Marshal(rows)
				out[v.name] = sha256.Sum256(raw)
			}
			return out
		}
		for _, f := range []struct{ table, kind string }{{"erp_purchase_receipt", "create"}, {"erp_purchase_receipt_line", "create"}, {"erp_document", "create"}, {"erp_document_line", "create"}, {"erp_stock_ledger", "create"}, {"erp_purchase_event", "create"}, {"sys_oper_log", "create"}, {"erp_stock", "update"}, {"erp_document", "update"}, {"erp_purchase_order_line", "update"}, {"erp_purchase_order", "update"}} {
			p := create(a, sa, wa, x, 2)
			in := input(line(p), 2, 2)
			before := fingerprint()
			db := repository.DB(ctx)
			fault := errors.New("injected receiving transaction fault")
			hook := "receiving:fault"
			callback := func(tx *gorm.DB) {
				if tx.Statement.Table == f.table {
					tx.AddError(fault)
				}
			}
			var e error
			if f.kind == "create" {
				e = db.Callback().Create().Before("gorm:create").Register(hook, callback)
			} else {
				e = db.Callback().Update().Before("gorm:update").Register(hook, callback)
			}
			if e != nil {
				t.Fatal(e)
			}
			_, e = service.ReceiveERPPurchase(ctx, p, in, "receiving-fault")
			if f.kind == "create" {
				db.Callback().Create().Remove(hook)
			} else {
				db.Callback().Update().Remove(hook)
			}
			if !errors.Is(e, fault) {
				t.Fatal("fault not reached", f.table, f.kind, e)
			}
			after := fingerprint()
			for table, hash := range before {
				if after[table] != hash {
					t.Fatal("partial business commit", f.table, table)
				}
			}
		}
	})
	run("receive_only_privacy_isolation_headers_and_live_revoke", func(t *testing.T) {
		p := create(a, sa, wa, x, 2)
		l := line(p)
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), users[0], nil))
		deny(s.req("GET", "/erp/purchase/list", users[0], nil))
		r := s.req("GET", fmt.Sprintf("/erp/purchase/%d/receivable", p), users[0], nil)
		s.ok(r)
		raw, _ := json.Marshal(r)
		for _, v := range []string{"unitPrice", "lineAmount", "orderedAmount", "PRIVATE_RECEIVING", "contactName", "closeReason"} {
			if strings.Contains(string(raw), v) {
				t.Fatal("receive-only leak", v)
			}
		}
		in := input(l, 1, 2)
		result := data(receive(users[0], p, in))
		foreign := create(b, sb, wb, z, 2)
		foreignDetail := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", foreign), b.owner, nil))
		foreignLine := int64(foreignDetail["lines"].([]any)[0].(map[string]any)["lineId"].(float64))
		deny(receive(users[0], foreign, input(foreignLine, 1, 2)))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%.0f", result["receiptId"]), b.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d/receivable", p), platform, nil))
		deny(s.req("GET", "/erp/purchaseReceipt/list", platform, nil))
		for _, path := range []string{"/erp/purchase/receivable/list", "/erp/purchaseReceipt/list", fmt.Sprintf("/erp/purchaseReceipt/%.0f", result["receiptId"])} {
			req, _ := http.NewRequest("GET", s.base+path, nil)
			req.Header.Set("Authorization", "Bearer "+users[0])
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("private cache")
			}
		}
		grant(uids[0], []int64{})
		deny(receive(users[0], p, in))
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d/receivable", p), users[0], nil))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%.0f", result["receiptId"]), users[0], nil))
		grant(uids[0], []int64{wa})
		s.ok(receive(users[0], p, in))
		role["menuIds"] = []int64{6000, 7800, 7801}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(receive(users[0], p, in))
		deny(s.req("GET", "/erp/purchase/receivable/list", users[0], nil))
		grant(uids[0], []int64{})
		grant(uids[1], []int64{})
		var logs []model.SysOperLog
		if e := repository.DB(ctx).Where("method='erp.purchase.RECEIVE'").Find(&logs).Error; e != nil || len(logs) == 0 {
			t.Fatal("missing receive audit", e)
		}
		raw, _ = json.Marshal(logs)
		if strings.Contains(string(raw), "PRIVATE_RECEIVING") || strings.Contains(string(raw), "12345.6789") {
			t.Fatal("price/contact in audit")
		}
	})
}
