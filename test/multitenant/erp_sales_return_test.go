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
	"reflect"
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

func TestERPSalesReturns(t *testing.T) {
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
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository unavailable")
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
			s.t.Fatal("unauthorized/invalid operation accepted")
		}
	}
	id := func(r map[string]any, k string) int64 { return int64(r[k].(float64)) }
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 10*time.Minute)
	defer cancel()
	master := func(kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "订单退货快照"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = "订单退货仓"
		}
		s.ok(s.req("POST", "/erp/"+kind, a.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, a.owner, nil)
		s.ok(r)
		return id(r["rows"].([]any)[0].(map[string]any), kind+"Id")
	}
	wa := master("warehouse", "SR6-W")
	x, y := master("sku", "SR6-X"), master("sku", "SR6-Y")
	shop := id(data(s.req("POST", "/erp/shop", a.owner, map[string]any{"shopCode": "SR6-S", "shopName": "订单退货店铺", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})), "shopId")
	for _, code := range []string{"A", "B"} {
		it := id(data(s.req("POST", "/erp/shopItem", a.owner, map[string]any{"shopId": shop, "shopItemCode": code, "title": "变种" + code, "status": "0"})), "itemId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", it), a.owner, map[string]any{"version": 1, "skuId": x}))
	}
	initial := id(data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wa, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 10000}, {SkuID: y, Quantity: 100}}})), "documentId")
	s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", initial), a.owner, nil))
	type shipment struct {
		order, pkg, source             int64
		lines                          []int64
		originalOrder, originalPackage []byte
	}
	orderGet := func(oid int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/order/%d", oid), a.owner, nil))
	}
	packGet := func(pid int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/package/%d", pid), a.owner, nil))
	}
	create := func(shipped bool) shipment {
		v := map[string]any{"shopId": shop, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "PRIVATE_SR6_RECIPIENT", "receiverPhone": "PRIVATE_SR6_PHONE", "receiverCountry": "CN", "receiverAddress": "PRIVATE_SR6_ADDRESS", "originalAmount": "45678.1234", "currency": "CNY", "orderedAt": "2026-10-23 12:00:00", "lines": []model.ERPOrderLineInput{{SourceLineNo: "001", ShopItemCode: "A", SourceTitle: "原始A", Quantity: 2}, {SourceLineNo: "002", ShopItemCode: "B", SourceTitle: "原始B", Quantity: 3}}}
		o := id(data(s.req("POST", "/erp/order", a.owner, v)), "orderId")
		review := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", o), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": wa}))
		p := id(review, "packageId")
		if shipped {
			s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": "PRIVATE_SR6_TRACKING"}))
			s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", p), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString()}))
		}
		od, pd := orderGet(o), packGet(p)
		f := shipment{order: o, pkg: p, source: id(od, "stockDocumentId")}
		for _, l := range pd["lines"].([]any) {
			f.lines = append(f.lines, id(l.(map[string]any), "lineId"))
		}
		f.originalOrder, _ = json.Marshal(od)
		f.originalPackage, _ = json.Marshal(pd)
		return f
	}
	get := func(pid int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/package/%d/returnable", pid), a.owner, nil))
	}
	input := func(line, qty, version int64) model.ERPSalesReturnInput {
		return model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: version, RequestKey: uuid.NewString()}, Reason: "PRIVATE_SR6_REASON", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: line, Quantity: qty}}}
	}
	post := func(token string, pid int64, in model.ERPSalesReturnInput) map[string]any {
		return s.req("POST", fmt.Sprintf("/erp/package/%d/returns", pid), token, in)
	}
	balance := func() (int64, int64) {
		var st model.ERPStock
		if e := repository.DB(ctx).Where("warehouse_id=? AND sku_id=?", wa, x).Take(&st).Error; e != nil {
			t.Fatal(e)
		}
		return st.OnHand, st.Reserved
	}
	role := map[string]any{"roleName": "仅订单退货", "roleKey": "sales_return_only", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 8100, 8101, 8102}}
	s.ok(s.req("POST", "/system/role", a.owner, role))
	rr := s.req("GET", "/system/role/list?roleKey=sales_return_only", a.owner, nil)
	s.ok(rr)
	rid := id(rr["rows"].([]any)[0].(map[string]any), "roleId")
	role["roleId"] = rid
	grant := func(kind string, uid int64, ids []int64) {
		path := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
		v := data(s.req("GET", path, a.owner, nil))
		key := "warehouseIds"
		if kind == "shop" {
			key = "shopIds"
		}
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], key: ids}))
	}
	users := []string{}
	uids := []int64{}
	for _, name := range []string{"salesreturnone", "salesreturntwo"} {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
		r := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(r)
		uid := id(r["rows"].([]any)[0].(map[string]any), "userId")
		uids = append(uids, uid)
		grant("warehouse", uid, []int64{wa})
		grant("shop", uid, []int64{shop})
		users = append(users, s.loginAs(a.code, name, operationsPassword))
	}
	t.Cleanup(func() {
		for i, uid := range uids {
			grant("warehouse", uid, []int64{})
			grant("shop", uid, []int64{})
			s.req("POST", "/logout", users[i], nil)
		}
	})
	var original shipment
	var firstInput model.ERPSalesReturnInput
	var firstResult map[string]any
	run("same_sku_distinct_original_lines_partial_full_and_immutable_facts", func(t *testing.T) {
		reserved := create(false)
		original = create(true)
		h, r := balance()
		firstInput = input(original.lines[0], 1, 1)
		firstResult = data(post(users[0], original.pkg, firstInput))
		if ah, ar := balance(); ah != h+1 || ar != r || r != 5 {
			t.Fatal("stock/reserved mismatch")
		}
		v := get(original.pkg)
		if v["shippedQuantity"] != float64(5) || v["returnedQuantity"] != float64(1) || v["netShippedQuantity"] != float64(4) || v["version"] != float64(2) {
			t.Fatal("gross/return/net/version mismatch")
		}
		deny(post(users[0], original.pkg, input(original.lines[0], 2, 2)))
		multi := input(original.lines[0], 1, 2)
		multi.Lines = append(multi.Lines, model.ERPSalesReturnLineInput{PackageLineID: original.lines[1], Quantity: 3})
		result := data(post(users[0], original.pkg, multi))
		var doc model.ERPDocument
		if e := repository.DB(ctx).Where("document_id=?", id(result, "stockDocumentId")).Take(&doc).Error; e != nil {
			t.Fatal(e)
		}
		var dl []model.ERPDocumentLine
		if e := repository.DB(ctx).Where("document_id=?", doc.DocumentID).Find(&dl).Error; e != nil || len(dl) != 1 || dl[0].Quantity != 4 {
			t.Fatal("same-SKU stock aggregation failed", e)
		}
		if get(original.pkg)["netShippedQuantity"] != float64(0) {
			t.Fatal("not fully returned")
		}
		deny(post(users[0], original.pkg, input(original.lines[1], 1, 3)))
		od, _ := json.Marshal(orderGet(original.order))
		pd, _ := json.Marshal(packGet(original.pkg))
		if string(od) != string(original.originalOrder) || string(pd) != string(original.originalPackage) {
			t.Fatal("original shipping facts modified")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", reserved.order), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString(), "reason": "测试释放预占"}))
	})
	run("replay_returns_original_result_and_binds_actor_content", func(t *testing.T) {
		f := create(true)
		in := input(f.lines[0], 1, 1)
		first := data(post(users[0], f.pkg, in))
		data(post(users[0], f.pkg, input(f.lines[1], 1, 2)))
		again := data(post(users[0], f.pkg, in))
		if again["existing"] != true || again["returnId"] != first["returnId"] || again["version"] != float64(2) {
			t.Fatal("not original result")
		}
		deny(post(users[1], f.pkg, in))
		in.Reason = "changed"
		deny(post(users[0], f.pkg, in))
	})
	run("invalid_types_wrong_original_line_unshipped_and_duplicate_lines", func(t *testing.T) {
		f := create(false)
		deny(post(users[0], f.pkg, input(f.lines[0], 1, 1)))
		other := create(true)
		deny(post(users[0], other.pkg, input(f.lines[0], 1, 1)))
		in := input(other.lines[0], 1, 1)
		in.Lines = append(in.Lines, in.Lines[0])
		deny(post(users[0], other.pkg, in))
		for _, qty := range []any{0, -1, 1.5, "1", nil, true, 1000000001} {
			deny(s.req("POST", fmt.Sprintf("/erp/package/%d/returns", other.pkg), users[0], map[string]any{"version": 1, "requestKey": uuid.NewString(), "reason": "bad", "lines": []map[string]any{{"packageLineId": other.lines[0], "quantity": qty}}}))
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/cancel", f.order), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString(), "reason": "释放"}))
	})
	run("concurrent_new_keys_single_revision_and_identical_key_once", func(t *testing.T) {
		f := create(true)
		h, _ := balance()
		rs := parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/package/%d/returns", f.pkg), users[i%2], input(f.lines[0], 2, 1)
		})
		ok := 0
		for _, r := range rs {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				ok++
			}
		}
		if ok != 1 {
			t.Fatal("revision race", ok)
		}
		if ah, _ := balance(); ah != h+2 {
			t.Fatal("race over-return")
		}
		f = create(true)
		in := input(f.lines[0], 2, 1)
		h, _ = balance()
		rs = parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/package/%d/returns", f.pkg), users[0], in
		})
		for _, r := range rs {
			if r.err != nil || r.code != 200 {
				t.Fatal("identical key failed", r.err, r.code)
			}
		}
		if ah, _ := balance(); ah != h+2 {
			t.Fatal("duplicate replay stock")
		}
	})
	legacy := func(f shipment) int64 {
		return id(data(s.req("POST", "/erp/salesReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": f.source, "requestKey": uuid.NewString(), "remark": "旧库存纠错验收", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}})), "documentId")
	}
	run("legacy_first_blocks_commercial_and_commercial_first_blocks_old_draft", func(t *testing.T) {
		f := create(true)
		old := legacy(f)
		s.ok(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", old), a.owner, nil))
		deny(post(users[0], f.pkg, input(f.lines[0], 1, 1)))
		deny(s.req("GET", fmt.Sprintf("/erp/package/%d/returnable", f.pkg), users[0], nil))
		rows := s.req("GET", fmt.Sprintf("/erp/salesReturnOrder/returnable/list?packageId=%d", f.pkg), users[0], nil)
		s.ok(rows)
		if rows["rows"].([]any)[0].(map[string]any)["legacyStockReturn"] != true {
			t.Fatal("legacy block not visible")
		}
		f = create(true)
		old = legacy(f)
		data(post(users[0], f.pkg, input(f.lines[0], 1, 1)))
		deny(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", old), a.owner, nil))
		r := s.req("POST", "/erp/salesReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": f.source, "requestKey": uuid.NewString(), "remark": "旧库存入口", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}})
		deny(r)
		if !strings.Contains(r["msg"].(string), "不可混用") {
			t.Fatal("not the commercial/legacy boundary")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/cancel", old), a.owner, nil))
	})
	run("legacy_and_commercial_competing_transactions_only_one_posts", func(t *testing.T) {
		f := create(true)
		old := legacy(f)
		h, r := balance()
		rs := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				return "PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", old), a.owner, nil
			}
			return "POST", fmt.Sprintf("/erp/package/%d/returns", f.pkg), users[0], input(f.lines[0], 1, 1)
		})
		ok := 0
		for _, v := range rs {
			if v.err != nil {
				t.Fatal(v.err)
			}
			if v.code == 200 {
				ok++
			}
		}
		if ok != 1 {
			t.Fatal("both return paths posted", ok)
		}
		if ah, ar := balance(); ah != h+1 || ar != r {
			t.Fatal("competing returns changed stock twice")
		}
	})
	run("managed_SR_legacy_edit_action_create_replay_and_source_denied", func(t *testing.T) {
		docid := id(firstResult, "stockDocumentId")
		for _, action := range []string{"confirm", "cancel"} {
			deny(s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d/%s", docid, action), a.owner, nil))
		}
		edited := s.req("PUT", fmt.Sprintf("/erp/salesReturn/%d", docid), a.owner, map[string]any{"version": 2, "warehouseId": wa, "sourceDocumentId": original.source, "requestKey": uuid.NewString(), "remark": "不可修改托管凭据", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}})
		deny(edited)
		if !strings.Contains(edited["msg"].(string), "托管") {
			t.Fatal("not managed edit boundary", edited["msg"])
		}
		var doc model.ERPDocument
		if e := repository.DB(ctx).Where("document_id=?", docid).Take(&doc).Error; e != nil {
			t.Fatal(e)
		}
		r := s.req("POST", "/erp/salesReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": original.source, "requestKey": doc.RequestKey, "remark": "旧键旁路", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}})
		deny(r)
		if !strings.Contains(r["msg"].(string), "托管") {
			t.Fatal("not managed replay boundary")
		}
		deny(s.req("POST", "/erp/salesReturn", a.owner, map[string]any{"warehouseId": wa, "sourceDocumentId": docid, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}}))
		s.ok(s.req("GET", fmt.Sprintf("/erp/salesReturn/%d", docid), a.owner, nil))
	})
	run("all_transaction_write_failures_roll_back_every_table", func(t *testing.T) {
		tables := []struct{ name, pk string }{{"erp_order", "order_id"}, {"erp_order_line", "line_id"}, {"erp_package", "package_id"}, {"erp_package_line", "line_id"}, {"erp_sales_return_order", "return_id"}, {"erp_sales_return_line", "line_id"}, {"erp_document", "document_id"}, {"erp_document_line", "line_id"}, {"erp_stock", "stock_id"}, {"erp_stock_ledger", "ledger_id"}, {"sys_oper_log", "oper_id"}}
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
		for _, f := range []struct{ table, kind string }{{"erp_sales_return_order", "create"}, {"erp_sales_return_line", "create"}, {"erp_document", "create"}, {"erp_document_line", "create"}, {"erp_stock", "create"}, {"erp_stock_ledger", "create"}, {"sys_oper_log", "create"}, {"erp_stock", "update"}, {"erp_document", "update"}, {"erp_sales_return_order", "update"}} {
			ship := create(true)
			in := input(ship.lines[0], 1, 1)
			before := fingerprint()
			db := repository.DB(ctx)
			fault := errors.New("injected sales return transaction fault")
			const hook = "salesreturn:fault"
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
			_, e = service.ReturnERPSalesPackage(ctx, ship.pkg, in, "sales-return-fault")
			if f.kind == "create" {
				db.Callback().Create().Remove(hook)
			} else {
				db.Callback().Update().Remove(hook)
			}
			if !errors.Is(e, fault) {
				t.Fatal("fault not reached", f, e)
			}
			if !reflect.DeepEqual(before, fingerprint()) {
				t.Fatal("transaction partially committed", f)
			}
		}
	})
	run("composite_FK_and_context_isolation_fail_closed", func(t *testing.T) {
		var l model.ERPSalesReturnLine
		if e := repository.DB(ctx).Where("return_id=?", id(firstResult, "returnId")).Take(&l).Error; e != nil {
			t.Fatal(e)
		}
		for _, v := range []map[string]any{{"sku_id": y}, {"package_id": l.PackageID + 999999}, {"order_id": l.OrderID + 999999}} {
			e := repository.DB(ctx).Transaction(func(tx *gorm.DB) error {
				e := tx.Model(&model.ERPSalesReturnLine{}).Where("line_id=?", l.LineID).Updates(v).Error
				var my *mysql.MySQLError
				if !errors.As(e, &my) || my.Number != 1452 {
					t.Fatal("wrong FK failure", e)
				}
				return errors.New("rollback probe")
			})
			if e == nil {
				t.Fatal("probe committed")
			}
		}
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, e := service.GetERPSalesReturnable(c, original.pkg); e == nil {
			t.Fatal("missing tenant accepted")
		}
	})
	run("response_privacy_no_store_cross_tenant_and_live_scope_revocation", func(t *testing.T) {
		paths := []string{"/erp/salesReturnOrder/list", "/erp/salesReturnOrder/returnable/list", fmt.Sprintf("/erp/package/%d/returnable", original.pkg), fmt.Sprintf("/erp/salesReturnOrder/%.0f", firstResult["returnId"])}
		for _, path := range paths {
			r := s.req("GET", path, users[0], nil)
			s.ok(r)
			raw, _ := json.Marshal(r)
			for _, bad := range []string{"receiverName", "receiverPhone", "receiverAddress", "trackingNo", "originalAmount", "PRIVATE_SR6_RECIPIENT", "PRIVATE_SR6_PHONE", "PRIVATE_SR6_ADDRESS", "PRIVATE_SR6_TRACKING", "45678.1234", "requestHash", "resultJson"} {
				if strings.Contains(string(raw), bad) {
					t.Fatal("private response field", bad)
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
				t.Fatal("private cache allowed")
			}
		}
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", original.order), users[0], nil))
		deny(s.req("GET", fmt.Sprintf("/erp/package/%d", original.pkg), users[0], nil))
		deny(s.req("GET", paths[3], b.owner, nil))
		deny(post(b.owner, original.pkg, firstInput))
		deny(s.req("GET", paths[0], platform, nil))
		for _, kind := range []string{"shop", "warehouse"} {
			grant(kind, uids[0], []int64{})
			deny(post(users[0], original.pkg, firstInput))
			deny(s.req("GET", paths[2], users[0], nil))
			deny(s.req("GET", paths[3], users[0], nil))
			for _, path := range paths[:2] {
				r := s.req("GET", path, users[0], nil)
				s.ok(r)
				if r["total"] != float64(0) {
					t.Fatal("revoked rows visible")
				}
			}
			ids := []int64{wa}
			if kind == "shop" {
				ids = []int64{shop}
			}
			grant(kind, uids[0], ids)
			s.ok(post(users[0], original.pkg, firstInput))
		}
		role["menuIds"] = []int64{6000, 8100, 8101}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(post(users[0], original.pkg, firstInput))
		deny(s.req("GET", paths[2], users[0], nil))
	})
}
