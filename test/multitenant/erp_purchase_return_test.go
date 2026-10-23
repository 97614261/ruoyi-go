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

func TestERPPurchaseReturns(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 5*time.Minute)
	defer cancel()
	master := func(f operationsFixture, kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "退货测试商品"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "退货仓"
		}
		s.ok(s.req("POST", "/erp/"+kind, f.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, f.owner, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)[kind+"Id"].(float64))
	}
	wa, wb := master(a, "warehouse", "PR5-W"), master(b, "warehouse", "PR5-W")
	x, z := master(a, "sku", "PR5-X"), master(b, "sku", "PR5-X")
	supplier := func(f operationsFixture) int64 {
		return int64(data(s.req("POST", "/erp/supplier", f.owner, map[string]any{"supplierCode": "PR5-SUP", "supplierName": "退货供应商", "status": "0", "phone": "PRIVATE_RETURN_CONTACT"}))["supplierId"].(float64))
	}
	sa, sb := supplier(a), supplier(b)
	type batch struct{ purchase, receipt, line, document int64 }
	create := func(f operationsFixture, sup, wh, sku, qty int64) batch {
		p := int64(data(s.req("POST", "/erp/purchase", f.owner, model.ERPPurchaseWrite{SupplierID: sup, WarehouseID: wh, Currency: "USD", RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: qty, UnitPrice: "12345.6789"}}}))["purchaseId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), f.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		v := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), f.owner, nil))
		l := int64(v["lines"].([]any)[0].(map[string]any)["lineId"].(float64))
		r := data(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), f.owner, model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: l, Quantity: qty}}}))
		receipt := int64(r["receiptId"].(float64))
		rd := data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d", receipt), f.owner, nil))
		return batch{p, receipt, int64(rd["lines"].([]any)[0].(map[string]any)["lineId"].(float64)), int64(r["stockDocumentId"].(float64))}
	}
	input := func(l, qty, version int64) model.ERPPurchaseReturnInput {
		return model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: version, RequestKey: uuid.NewString()}, Reason: "来货破损", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: l, Quantity: qty}}}
	}
	post := func(token string, receipt int64, in model.ERPPurchaseReturnInput) map[string]any {
		return s.req("POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", receipt), token, in)
	}
	get := func(receipt int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", receipt), a.owner, nil))
	}
	balance := func() (int64, int64) {
		var st model.ERPStock
		e := repository.DB(ctx).Where("warehouse_id=? AND sku_id=?", wa, x).Take(&st).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return 0, 0
		}
		if e != nil {
			t.Fatal(e)
		}
		return st.OnHand, st.Reserved
	}
	role := map[string]any{"roleName": "仅采购退货", "roleKey": "return_only", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 8000, 8001, 8002}}
	s.ok(s.req("POST", "/system/role", a.owner, role))
	roles := s.req("GET", "/system/role/list?roleKey=return_only", a.owner, nil)
	s.ok(roles)
	rid := roles["rows"].([]any)[0].(map[string]any)["roleId"]
	role["roleId"] = rid
	grant := func(uid int64, ids []int64) {
		path := fmt.Sprintf("/erp/warehouse/access/%d", uid)
		v := data(s.req("GET", path, a.owner, nil))
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], "warehouseIds": ids}))
	}
	users := []string{}
	uids := []int64{}
	for _, name := range []string{"returnone", "returntwo"} {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []any{rid}}))
		r := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(r)
		uid := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		uids = append(uids, uid)
		grant(uid, []int64{wa})
		users = append(users, s.loginAs(a.code, name, operationsPassword))
	}
	t.Cleanup(func() {
		for _, uid := range uids {
			grant(uid, []int64{})
		}
	})
	var original batch
	var firstInput model.ERPPurchaseReturnInput
	var firstResult map[string]any
	run("partial_return_gross_net_reserved_and_original_facts", func(t *testing.T) {
		original = create(a, sa, wa, x, 10)
		beforePurchase := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", original.purchase), a.owner, nil))
		beforeReceipt := data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d", original.receipt), a.owner, nil))
		sale := data(s.req("POST", "/erp/sales", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 3}}}))
		sid := int64(sale["documentId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/confirm", sid), a.owner, nil))
		h, r := balance()
		firstInput = input(original.line, 2, 1)
		firstResult = data(post(users[0], original.receipt, firstInput))
		if ah, ar := balance(); ah != h-2 || ar != r || r != 3 {
			t.Fatal("hand/reserved mismatch")
		}
		v := get(original.receipt)
		if v["receivedQuantity"] != float64(10) || v["returnedQuantity"] != float64(2) || v["netReceivedQuantity"] != float64(8) || v["version"] != float64(2) {
			t.Fatal("gross/net/version", v)
		}
		for path, before := range map[string]map[string]any{fmt.Sprintf("/erp/purchase/%d", original.purchase): beforePurchase, fmt.Sprintf("/erp/purchaseReceipt/%d", original.receipt): beforeReceipt} {
			after := data(s.req("GET", path, a.owner, nil))
			a, _ := json.Marshal(before)
			b, _ := json.Marshal(after)
			if string(a) != string(b) {
				t.Fatal("original purchasing facts changed", path)
			}
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/cancel", sid), a.owner, nil))
	})
	run("replay_original_result_actor_and_changed_content", func(t *testing.T) {
		f := create(a, sa, wa, x, 3)
		in := input(f.line, 1, 1)
		first := data(post(users[0], f.receipt, in))
		data(post(users[0], f.receipt, input(f.line, 1, 2)))
		again := data(post(users[0], f.receipt, in))
		if again["existing"] != true || again["returnId"] != first["returnId"] || again["version"] != float64(2) {
			t.Fatal("not original result")
		}
		deny(post(users[1], f.receipt, in))
		in.Reason = "不同原因"
		deny(post(users[0], f.receipt, in))
		deny(post(users[0], f.receipt, input(f.line, 2, 3)))
	})
	run("wrong_receipt_line_overreturn_and_input_types", func(t *testing.T) {
		f, g := create(a, sa, wa, x, 2), create(a, sa, wa, x, 6)
		deny(post(users[0], f.receipt, input(g.line, 1, 1)))
		deny(post(users[0], f.receipt, input(f.line, 3, 1)))
		in := input(f.line, 1, 1)
		in.Lines = append(in.Lines, in.Lines[0])
		deny(post(users[0], f.receipt, in))
		for _, qty := range []any{0, -1, 1.5, "1", nil, 1000000001} {
			deny(s.req("POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", f.receipt), users[0], map[string]any{"version": 1, "requestKey": uuid.NewString(), "reason": "破损", "lines": []any{map[string]any{"receiptLineId": f.line, "quantity": qty}}}))
		}
		in = input(f.line, 1, 1)
		in.Reason = " \n "
		deny(post(users[0], f.receipt, in))
		in.Reason = "x\ny"
		deny(post(users[0], f.receipt, in))
	})
	run("same_purchase_different_receipt_batches_do_not_share_allowance", func(t *testing.T) {
		p := int64(data(s.req("POST", "/erp/purchase", a.owner, model.ERPPurchaseWrite{SupplierID: sa, WarehouseID: wa, Currency: "USD", RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: x, Quantity: 10, UnitPrice: "1"}}}))["purchaseId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		v := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), a.owner, nil))
		l := int64(v["lines"].([]any)[0].(map[string]any)["lineId"].(float64))
		receipts := []batch{}
		for i, qty := range []int64{4, 6} {
			r := data(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), a.owner, model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: int64(i + 2), RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: l, Quantity: qty}}}))
			id := int64(r["receiptId"].(float64))
			rd := get(id)
			receipts = append(receipts, batch{purchase: p, receipt: id, line: int64(rd["lines"].([]any)[0].(map[string]any)["lineId"].(float64))})
		}
		deny(post(users[0], receipts[0].receipt, input(receipts[1].line, 1, 1)))
		deny(post(users[0], receipts[0].receipt, input(receipts[0].line, 5, 1)))
		s.ok(post(users[0], receipts[0].receipt, input(receipts[0].line, 4, 1)))
		s.ok(post(users[0], receipts[1].receipt, input(receipts[1].line, 6, 1)))
		if get(receipts[0].receipt)["netReceivedQuantity"] != float64(0) || get(receipts[1].receipt)["netReceivedQuantity"] != float64(0) {
			t.Fatal("batch remainder")
		}
	})
	run("last_available_two_actors_and_same_key_concurrency", func(t *testing.T) {
		f := create(a, sa, wa, x, 10)
		before, _ := balance()
		rs := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", f.receipt), users[i], input(f.line, 10, 1)
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
		if h, _ := balance(); wins != 1 || h != before-10 {
			t.Fatal("overreturn race", wins, h)
		}
		f = create(a, sa, wa, x, 3)
		in := input(f.line, 2, 1)
		before, _ = balance()
		rs = parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", f.receipt), users[0], in
		})
		for _, r := range rs {
			if r.err != nil || r.code != 200 {
				t.Fatal("same key failed", r.err, r.code, r.message)
			}
		}
		if h, _ := balance(); h != before-2 {
			t.Fatal("duplicate stock")
		}
		var n int64
		if e := repository.DB(ctx).Model(&model.ERPPurchaseReturn{}).Where("receipt_id=?", f.receipt).Count(&n).Error; e != nil || n != 1 {
			t.Fatal("duplicate return", n, e)
		}
	})
	run("unavailable_stock_rejected_reserved_unchanged", func(t *testing.T) {
		f := create(a, sa, wa, x, 2)
		h, r := balance()
		sale := data(s.req("POST", "/erp/sales", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: h - r}}}))
		sid := int64(sale["documentId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/confirm", sid), a.owner, nil))
		deny(post(users[0], f.receipt, input(f.line, 1, 1)))
		if ah, ar := balance(); ah != h || ar != h {
			t.Fatal("failed return changed stock")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/cancel", sid), a.owner, nil))
	})
	run("disabled_masters_allow_historical_correction", func(t *testing.T) {
		f := create(a, sa, wa, x, 2)
		sup := data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sup["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sup))
		sku := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", x), a.owner, nil))
		sku["status"] = "1"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		wh := data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		wh["status"] = "1"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, wh))
		s.ok(post(users[0], f.receipt, input(f.line, 1, 1)))
		sup = data(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, nil))
		sup["status"] = "0"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", sa), a.owner, sup))
		sku["status"] = "0"
		s.ok(s.req("PUT", "/erp/sku", a.owner, sku))
		wh["status"] = "0"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, wh))
	})
	run("managed_PR_legacy_actions_edit_create_and_source_blocked", func(t *testing.T) {
		id := int64(firstResult["stockDocumentId"].(float64))
		for _, action := range []string{"confirm", "cancel"} {
			r := s.req("PUT", fmt.Sprintf("/erp/purchaseReturn/%d/%s", id, action), a.owner, nil)
			deny(r)
			if !strings.Contains(r["msg"].(string), "托管") {
				t.Fatal("not managed boundary", r)
			}
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/purchaseReturn/%d", id), a.owner, map[string]any{"version": 2, "warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}}))
		var doc model.ERPDocument
		if e := repository.DB(ctx).Where("document_id=?", id).Take(&doc).Error; e != nil {
			t.Fatal(e)
		}
		deny(s.req("POST", "/erp/purchaseReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": original.document, "requestKey": doc.RequestKey, "lines": []model.ERPLineInput{{SkuID: x, Quantity: 2}}}))
		for _, source := range []int64{original.document, id} {
			deny(s.req("POST", "/erp/purchaseReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": source, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}}))
		}
		s.ok(s.req("GET", fmt.Sprintf("/erp/purchaseReturn/%d", id), a.owner, nil))
	})
	run("every_write_fault_rolls_back_stock_returns_audit_and_originals", func(t *testing.T) {
		tables := []struct{ name, pk string }{{"erp_purchase_order", "purchase_id"}, {"erp_purchase_order_line", "line_id"}, {"erp_purchase_receipt", "receipt_id"}, {"erp_purchase_receipt_line", "line_id"}, {"erp_purchase_return_order", "return_id"}, {"erp_purchase_return_line", "line_id"}, {"erp_document", "document_id"}, {"erp_document_line", "line_id"}, {"erp_stock", "stock_id"}, {"erp_stock_ledger", "ledger_id"}, {"sys_oper_log", "oper_id"}}
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
		for _, f := range []struct{ table, kind string }{{"erp_purchase_return_order", "create"}, {"erp_purchase_return_line", "create"}, {"erp_document", "create"}, {"erp_document_line", "create"}, {"erp_stock", "create"}, {"erp_stock_ledger", "create"}, {"sys_oper_log", "create"}, {"erp_stock", "update"}, {"erp_document", "update"}, {"erp_purchase_return_order", "update"}} {
			fixture := create(a, sa, wa, x, 2)
			in := input(fixture.line, 1, 1)
			before := fingerprint()
			db := repository.DB(ctx)
			fault := errors.New("injected purchase return transaction fault")
			const hook = "returns:fault"
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
			_, e = service.ReturnERPPurchaseReceipt(ctx, fixture.receipt, in, "return-fault")
			if f.kind == "create" {
				db.Callback().Create().Remove(hook)
			} else {
				db.Callback().Update().Remove(hook)
			}
			if !errors.Is(e, fault) {
				t.Fatal("fault not reached", f, e)
			}
			after := fingerprint()
			for table, hash := range before {
				if after[table] != hash {
					t.Fatal("partial commit", f, table)
				}
			}
		}
	})
	run("multiline_second_stock_shortage_rolls_back_first_sku", func(t *testing.T) {
		y := master(a, "sku", "PR5-Y")
		p := int64(data(s.req("POST", "/erp/purchase", a.owner, model.ERPPurchaseWrite{SupplierID: sa, WarehouseID: wa, Currency: "USD", RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: x, Quantity: 1, UnitPrice: "0"}, {SkuID: y, Quantity: 1, UnitPrice: "0"}}}))["purchaseId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		v := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), a.owner, nil))
		receiveLines := []model.ERPPurchaseReceiveLineInput{}
		for _, row := range v["lines"].([]any) {
			receiveLines = append(receiveLines, model.ERPPurchaseReceiveLineInput{PurchaseLineID: int64(row.(map[string]any)["lineId"].(float64)), Quantity: 1})
		}
		r := data(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), a.owner, model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Lines: receiveLines}))
		id := int64(r["receiptId"].(float64))
		rd := get(id)
		in := model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "整批破损"}
		for _, row := range rd["lines"].([]any) {
			in.Lines = append(in.Lines, model.ERPPurchaseReturnLineInput{ReceiptLineID: int64(row.(map[string]any)["lineId"].(float64)), Quantity: 1})
		}
		sale := data(s.req("POST", "/erp/sales", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: y, Quantity: 1}}}))
		sid := int64(sale["documentId"].(float64))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/confirm", sid), a.owner, nil))
		before, _ := balance()
		deny(post(users[0], id, in))
		if after, _ := balance(); after != before {
			t.Fatal("first sku partially deducted")
		}
		if get(id)["version"] != float64(1) {
			t.Fatal("failed return header survived")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/sales/%d/cancel", sid), a.owner, nil))
	})
	run("composite_fk_rejects_cross_sku_and_batch_links", func(t *testing.T) {
		var line model.ERPPurchaseReturnLine
		if e := repository.DB(ctx).Where("return_id=?", int64(firstResult["returnId"].(float64))).Take(&line).Error; e != nil {
			t.Fatal(e)
		}
		for _, values := range []map[string]any{{"sku_id": z}, {"receipt_id": original.receipt + 999999}, {"purchase_id": original.purchase + 999999}} {
			e := repository.DB(ctx).Transaction(func(tx *gorm.DB) error {
				e := tx.Model(&model.ERPPurchaseReturnLine{}).Where("line_id=?", line.LineID).Updates(values).Error
				var my *mysql.MySQLError
				if !errors.As(e, &my) || my.Number != 1452 {
					t.Fatal("wrong FK failure", e)
				}
				return errors.New("rollback test")
			})
			if e == nil {
				t.Fatal("test transaction committed")
			}
		}
	})
	run("privacy_isolation_no_store_and_live_revocation", func(t *testing.T) {
		foreign := create(b, sb, wb, z, 2)
		deny(post(users[0], foreign.receipt, input(foreign.line, 1, 1)))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", original.receipt), b.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReturnOrder/%.0f", firstResult["returnId"]), b.owner, nil))
		deny(s.req("GET", "/erp/purchaseReturnOrder/list", platform, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/purchase/%d", original.purchase), users[0], nil))
		deny(s.req("GET", fmt.Sprintf("/erp/supplier/%d", sa), users[0], nil))
		for _, path := range []string{"/erp/purchaseReturnOrder/list", "/erp/purchaseReturnOrder/returnable/list", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", original.receipt), fmt.Sprintf("/erp/purchaseReturnOrder/%.0f", firstResult["returnId"])} {
			r := s.req("GET", path, users[0], nil)
			s.ok(r)
			raw, _ := json.Marshal(r)
			for _, bad := range []string{"unitPrice", "orderedAmount", "contactName", "PRIVATE_RETURN", "12345.6789", "requestHash", "resultJson"} {
				if strings.Contains(string(raw), bad) {
					t.Fatal("return employee leak", bad)
				}
			}
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
		deny(post(users[0], original.receipt, firstInput))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", original.receipt), users[0], nil))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReturnOrder/%.0f", firstResult["returnId"]), users[0], nil))
		for _, path := range []string{"/erp/purchaseReturnOrder/list", "/erp/purchaseReturnOrder/returnable/list"} {
			r := s.req("GET", path, users[0], nil)
			s.ok(r)
			if r["total"] != float64(0) {
				t.Fatal("revoked rows visible")
			}
		}
		grant(uids[0], []int64{wa})
		s.ok(post(users[0], original.receipt, firstInput))
		role["menuIds"] = []int64{6000, 8000, 8001}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(post(users[0], original.receipt, firstInput))
		deny(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", original.receipt), users[0], nil))
		grant(uids[0], []int64{})
		grant(uids[1], []int64{})
	})
}
