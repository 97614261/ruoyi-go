package multitenant_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"io"
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

func TestERPSalesFinance(t *testing.T) {
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
	wa := master("warehouse", "SF9-W")
	x, y := master("sku", "SF9-X"), master("sku", "SF9-Y")
	shop := id(data(s.req("POST", "/erp/shop", a.owner, map[string]any{"shopCode": "SF9-S", "shopName": "订单退货店铺", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})), "shopId")
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
	create := func(shipped bool, amounts ...string) shipment {
		v := map[string]any{"shopId": shop, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "PRIVATE_SF9_RECIPIENT", "receiverPhone": "PRIVATE_SF9_PHONE", "receiverCountry": "CN", "receiverAddress": "PRIVATE_SF9_ADDRESS", "originalAmount": "100.0000", "currency": "CNY", "orderedAt": "2026-10-23 12:00:00", "lines": []model.ERPOrderLineInput{{SourceLineNo: "001", ShopItemCode: "A", SourceTitle: "原始A", Quantity: 2}, {SourceLineNo: "002", ShopItemCode: "B", SourceTitle: "原始B", Quantity: 3}}}
		if len(amounts) > 0 {
			if amounts[0] == "" {
				delete(v, "originalAmount")
			} else {
				v["originalAmount"] = amounts[0]
			}
		}
		o := id(data(s.req("POST", "/erp/order", a.owner, v)), "orderId")
		review := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", o), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": wa}))
		p := id(review, "packageId")
		if shipped {
			s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": "PRIVATE_SF9_TRACKING"}))
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

	account := id(data(s.req("POST", "/erp/finance/accounts", a.owner, model.FinAccountWrite{RequestKey: uuid.NewString(), AccountCode: "SF9-A", AccountName: "内部收款账户", AccountType: "BANK", Currency: "CNY", Status: "0"})), "id")
	register := func(sh shipment) int64 {
		return id(data(s.req("POST", "/erp/salesFinance/receivables", a.owner, model.SalesFinReceivableCreate{OrderID: sh.order, RequestKey: uuid.NewString()})), "id")
	}
	getAR := func(p int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/salesFinance/receivables/%d", p), a.owner, nil))
	}
	input := func(p int64, amount string) model.SalesFinCollectionWrite {
		return model.SalesFinCollectionWrite{RequestKey: uuid.NewString(), AccountID: account, ShopID: shop, Currency: "CNY", Amount: amount, Allocations: []model.SalesFinAllocationInput{{ReceivableID: p, Version: id(getAR(p), "version"), Amount: amount}}}
	}
	collect := func(in model.SalesFinCollectionWrite) map[string]any {
		return data(s.req("POST", "/erp/salesFinance/collections", a.owner, in))
	}
	money := func(p int64, credit, received, due, over string) {
		v := getAR(p)
		for k, w := range map[string]string{"grossAmount": "100.0000", "creditAmount": credit, "receivedAmount": received, "dueAmount": due, "overreceivedAmount": over} {
			if v[k] != w {
				t.Fatalf("%s got%v want%s", k, v[k], w)
			}
		}
	}
	physical := func(sh shipment) int64 {
		v := data(s.req("GET", fmt.Sprintf("/erp/package/%d/returnable", sh.pkg), a.owner, nil))
		return id(data(s.req("POST", fmt.Sprintf("/erp/package/%d/returns", sh.pkg), a.owner, model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: id(v, "version"), RequestKey: uuid.NewString()}, Reason: "合成实物退货", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: sh.lines[0], Quantity: 1}}})), "returnId")
	}
	credit := func(p, r int64, amount string) model.SalesFinAdjust {
		return model.SalesFinAdjust{FinToken: model.FinToken{Version: id(getAR(p), "version"), RequestKey: uuid.NewString()}, ReturnID: r, Amount: amount, Reason: "人工核对抵减；非数量分摊/银行退款"}
	}
	run("explicit_shipped_source_missing_is_not_zero_and_frozen_total", func(t *testing.T) {
		missing := create(true, "")
		deny(s.req("POST", "/erp/salesFinance/receivables", a.owner, model.SalesFinReceivableCreate{OrderID: missing.order, RequestKey: uuid.NewString()}))
		notShipped := create(false)
		deny(s.req("POST", "/erp/salesFinance/receivables", a.owner, model.SalesFinReceivableCreate{OrderID: notShipped.order, RequestKey: uuid.NewString()}))
		z := register(create(true, "0"))
		if getAR(z)["grossAmount"] != "0.0000" {
			t.Fatal("explicit zero lost")
		}
		sh := create(true)
		before := s.req("GET", fmt.Sprintf("/erp/salesFinance/sources?orderId=%d", sh.order), a.owner, nil)
		s.ok(before)
		if before["total"] != float64(1) {
			t.Fatal("source missing")
		}
		in := model.SalesFinReceivableCreate{OrderID: sh.order, RequestKey: uuid.NewString()}
		first := data(s.req("POST", "/erp/salesFinance/receivables", a.owner, in))
		again := data(s.req("POST", "/erp/salesFinance/receivables", a.owner, in))
		if again["existing"] != true || again["id"] != first["id"] {
			t.Fatal("registration original replay")
		}
		p := id(first, "id")
		money(p, "0.0000", "0.0000", "100.0000", "0.0000")
		deny(s.req("POST", "/erp/salesFinance/receivables", a.owner, model.SalesFinReceivableCreate{OrderID: sh.order, RequestKey: uuid.NewString()}))
		raw, _ := json.Marshal(getAR(p))
		for _, x := range []string{"receiverPhone", "receiverAddress", "requestHash", "unitPrice", "PRIVATE_SF9"} {
			if strings.Contains(string(raw), x) {
				t.Fatal("private/invented field", x)
			}
		}
	})
	run("partial_multi_order_collection_and_full_bookkeeping_reverse", func(t *testing.T) {
		p, q := register(create(true)), register(create(true))
		in := input(p, "50")
		in.Allocations[0].Amount = "20"
		in.Allocations = append(in.Allocations, model.SalesFinAllocationInput{ReceivableID: q, Version: 1, Amount: "30"})
		first := collect(in)
		money(p, "0.0000", "20.0000", "80.0000", "0.0000")
		money(q, "0.0000", "30.0000", "70.0000", "0.0000")
		next := input(p, "80")
		collect(next)
		money(p, "0.0000", "100.0000", "0.0000", "0.0000")
		deny(s.req("POST", "/erp/salesFinance/collections", a.owner, input(p, "1")))
		rev := model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "错误记账整笔冲销，不是银行退款"}
		path := fmt.Sprintf("/erp/salesFinance/collections/%d/reverse", id(first, "id"))
		s.ok(s.req("POST", path, a.owner, rev))
		s.ok(s.req("POST", path, a.owner, rev))
		money(p, "0.0000", "80.0000", "20.0000", "0.0000")
		money(q, "0.0000", "0.0000", "100.0000", "0.0000")
		again := collect(in)
		if again["existing"] != true || again["status"] != "POSTED" || id(again, "version") != 1 {
			t.Fatal("must return original collection result")
		}
	})
	run("manual_credit_pending_barrier_zero_and_overcollection", func(t *testing.T) {
		sh := create(true)
		p := register(sh)
		collect(input(p, "80"))
		old := input(p, "1")
		r := physical(sh)
		deny(s.req("POST", "/erp/salesFinance/collections", a.owner, old))
		over := credit(p, r, "101")
		deny(s.req("POST", fmt.Sprintf("/erp/salesFinance/receivables/%d/adjust", p), a.owner, over))
		in := credit(p, r, "50")
		path := fmt.Sprintf("/erp/salesFinance/receivables/%d/adjust", p)
		first := data(s.req("POST", path, a.owner, in))
		s.ok(s.req("POST", path, a.owner, in))
		money(p, "50.0000", "80.0000", "0.0000", "30.0000")
		duplicate := credit(p, r, "0")
		deny(s.req("POST", path, a.owner, duplicate))
		r2 := physical(sh)
		zero := credit(p, r2, "0")
		s.ok(s.req("POST", path, a.owner, zero))
		money(p, "50.0000", "80.0000", "0.0000", "30.0000")
		again := data(s.req("POST", path, a.owner, in))
		if again["existing"] != true || again["version"] != first["version"] {
			t.Fatal("credit original result")
		}
	})
	run("same_key_parallel_once_and_distinct_key_version_winner", func(t *testing.T) {
		p := register(create(true))
		in := input(p, "10")
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) { return "POST", "/erp/salesFinance/collections", a.owner, in }) {
			if r.code != 200 {
				t.Fatal("parallel same-key collection rejected", r.code)
			}
		}
		money(p, "0.0000", "10.0000", "90.0000", "0.0000")
		base := input(p, "10")
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			v := base
			v.RequestKey = uuid.NewString()
			return "POST", "/erp/salesFinance/collections", a.owner, v
		})
		wins := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("version winners", wins)
		}
		money(p, "0.0000", "20.0000", "80.0000", "0.0000")
	})
	menus := []int64{6000, 8700, 8701, 8702, 8703, 8704, 8800, 8801, 8802, 8803, 8900, 8901}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "仅销售财务", "roleKey": "sf9_only", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menus}))
	role := s.req("GET", "/system/role/list?roleKey=sf9_only", a.owner, nil)
	s.ok(role)
	rid := id(role["rows"].([]any)[0].(map[string]any), "roleId")
	grant := func(kind string, uid int64, ids []int64) {
		path := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
		v := data(s.req("GET", path, a.owner, nil))
		key := "warehouseIds"
		if kind == "shop" {
			key = "shopIds"
		}
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], key: ids}))
	}
	tokens := []string{}
	uids := []int64{}
	for _, name := range []string{"sfone", "sftwo"} {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
		v := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(v)
		uid := id(v["rows"].([]any)[0].(map[string]any), "userId")
		uids = append(uids, uid)
		grant("shop", uid, []int64{shop})
		grant("warehouse", uid, []int64{wa})
		tokens = append(tokens, s.loginAs(a.code, name, operationsPassword))
	}
	t.Cleanup(func() {
		for i, u := range uids {
			grant("shop", u, []int64{})
			grant("warehouse", u, []int64{})
			s.req("POST", "/logout", tokens[i], nil)
		}
	})
	run("independent_price_permission_two_employee_race_tenant_and_live_scopes", func(t *testing.T) {
		p := register(create(true))
		in := input(p, "10")
		out := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			v := in
			v.RequestKey = fmt.Sprintf("sf9-race-%s-%d", in.RequestKey, i)
			return "POST", "/erp/salesFinance/collections", tokens[i], v
		})
		wins := 0
		var winner int
		for i, r := range out {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
				winner = i
			}
		}
		if wins != 1 {
			t.Fatal("employee race", wins)
		}
		money(p, "0.0000", "10.0000", "90.0000", "0.0000")
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", create(true).order), tokens[0], nil))
		deny(s.req("GET", fmt.Sprintf("/erp/salesFinance/receivables/%d", p), b.owner, nil))
		deny(s.req("GET", "/erp/salesFinance/receivables", platform, nil))
		grant("warehouse", uids[winner], []int64{})
		retry := in
		retry.RequestKey = fmt.Sprintf("sf9-race-%s-%d", in.RequestKey, winner)
		deny(s.req("POST", "/erp/salesFinance/collections", tokens[winner], retry))
		v := s.req("GET", "/erp/salesFinance/collections", tokens[winner], nil)
		s.ok(v)
		if v["total"] != float64(0) {
			t.Fatal("withdrawn whole collection leaked")
		}
		grant("warehouse", uids[winner], []int64{wa})
		grant("shop", uids[winner], []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/salesFinance/receivables/%d", p), tokens[winner], nil))
		grant("shop", uids[winner], []int64{shop})
	})
	run("physical_return_and_other_employee_collection_serialize", func(t *testing.T) {
		sh := create(true)
		p := register(sh)
		in := input(p, "100")
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				return "POST", "/erp/salesFinance/collections", tokens[0], in
			}
			return "POST", fmt.Sprintf("/erp/package/%d/returns", sh.pkg), a.owner, model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "实物退货与收款竞争", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: sh.lines[0], Quantity: 1}}}
		})
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
		}
		if results[1].code != 200 {
			t.Fatal("physical return rejected")
		}
		v := getAR(p)
		if v["pendingReturnCount"] != float64(1) {
			t.Fatal("return escaped pending barrier")
		}
		if results[0].code == 200 {
			money(p, "0.0000", "100.0000", "0.0000", "0.0000")
		} else {
			money(p, "0.0000", "0.0000", "100.0000", "0.0000")
		}
		deny(s.req("POST", "/erp/salesFinance/collections", tokens[0], input(p, "1")))
	})
	run("disabled_account_original_replay_and_reverse", func(t *testing.T) {
		ac := model.FinAccountWrite{RequestKey: uuid.NewString(), AccountCode: "SF9-DIS", AccountName: "停用收款账户", AccountType: "BANK", Currency: "CNY", Status: "0"}
		aid := id(data(s.req("POST", "/erp/finance/accounts", a.owner, ac)), "id")
		p := register(create(true))
		in := input(p, "5")
		in.AccountID = aid
		posted := collect(in)
		ac.Version = 1
		ac.Status = "1"
		ac.RequestKey = uuid.NewString()
		s.ok(s.req("PUT", fmt.Sprintf("/erp/finance/accounts/%d", aid), a.owner, ac))
		again := collect(in)
		if again["existing"] != true || again["id"] != posted["id"] {
			t.Fatal("disabled account replay changed")
		}
		fresh := input(p, "1")
		fresh.AccountID = aid
		deny(s.req("POST", "/erp/salesFinance/collections", a.owner, fresh))
		s.ok(s.req("POST", fmt.Sprintf("/erp/salesFinance/collections/%d/reverse", id(posted, "id")), a.owner, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "停用账户冲销旧记账"}))
		money(p, "0.0000", "0.0000", "100.0000", "0.0000")
	})
	run("whole_collection_requires_every_warehouse_and_replay_current_scope", func(t *testing.T) {
		wb := master("warehouse", "SF9-WB")
		stock := id(data(s.req("POST", "/erp/receipt", a.owner, map[string]any{"warehouseId": wb, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: 100}}})), "documentId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", stock), a.owner, nil))
		p := register(create(true))
		sh := func() shipment {
			oldWarehouse := wa
			wa = wb
			defer func() { wa = oldWarehouse }()
			return create(true)
		}()
		q := register(sh)
		grant("warehouse", uids[0], []int64{wa, wb})
		in := input(p, "50")
		in.Allocations[0].Amount = "20"
		in.Allocations = append(in.Allocations, model.SalesFinAllocationInput{ReceivableID: q, Version: 1, Amount: "30"})
		result := data(s.req("POST", "/erp/salesFinance/collections", tokens[0], in))
		cid := id(result, "id")
		grant("warehouse", uids[0], []int64{wa})
		deny(s.req("GET", fmt.Sprintf("/erp/salesFinance/collections/%d", cid), tokens[0], nil))
		deny(s.req("POST", "/erp/salesFinance/collections", tokens[0], in))
		deny(s.req("POST", fmt.Sprintf("/erp/salesFinance/collections/%d/reverse", cid), tokens[0], model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "缺范围应拒绝"}))
		v := s.req("GET", "/erp/salesFinance/collections", tokens[0], nil)
		s.ok(v)
		for _, r := range v["rows"].([]any) {
			if id(r.(map[string]any), "collectionId") == cid {
				t.Fatal("partial total leaked")
			}
		}
		deny(s.req("GET", fmt.Sprintf("/erp/salesFinance/receivables/%d", q), tokens[0], nil))
	})
	run("all_create_faults_rollback_financial_and_stock_facts", func(t *testing.T) {
		sh := create(true)
		p := register(sh)
		ret := physical(sh)
		pay := register(create(true))
		posted := id(collect(input(pay, "10")), "id")
		fresh := create(true)
		snapshot := func() map[string]any {
			out := map[string]any{}
			for _, table := range []string{"erp_sf_receivable", "erp_sf_receivable_line", "erp_sf_credit", "erp_sf_collection", "erp_sf_allocation", "erp_sf_action", "sys_oper_log", "erp_stock", "erp_stock_ledger"} {
				var rows []map[string]any
				if e := repository.DB(ctx).Table(table).Order("1").Find(&rows).Error; e != nil {
					t.Fatal("snapshot", table)
				}
				out[table] = rows
			}
			return out
		}
		cases := []struct {
			tables []string
			call   func() error
		}{{[]string{"erp_sf_receivable", "erp_sf_receivable_line", "erp_sf_action", "sys_oper_log"}, func() error {
			_, e := service.CreateSalesFinanceReceivable(ctx, model.SalesFinReceivableCreate{OrderID: fresh.order, RequestKey: uuid.NewString()}, "fault")
			return e
		}}, {[]string{"erp_sf_collection", "erp_sf_allocation", "erp_sf_action", "sys_oper_log"}, func() error { _, e := service.CreateSalesFinanceCollection(ctx, input(pay, "1"), "fault"); return e }}, {[]string{"erp_sf_credit", "erp_sf_action", "sys_oper_log"}, func() error {
			_, e := service.AdjustSalesFinanceReceivable(ctx, p, credit(p, ret, "1"), "fault")
			return e
		}}, {[]string{"erp_sf_action", "sys_oper_log"}, func() error {
			_, e := service.ReverseSalesFinanceCollection(ctx, posted, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "注入失败"}, "fault")
			return e
		}}}
		db := repository.DB(ctx)
		for _, c := range cases {
			for _, table := range c.tables {
				before := snapshot()
				name := "sf9-fault-" + uuid.NewString()
				if e := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(errors.New("synthetic finance fault"))
					}
				}); e != nil {
					t.Fatal(e)
				}
				e := c.call()
				db.Callback().Create().Remove(name)
				if e == nil {
					t.Fatal("fault not hit", table)
				}
				if !reflect.DeepEqual(before, snapshot()) {
					t.Fatal("partial bookkeeping", table)
				}
			}
		}
	})
	run("shop_statement_exports_and_no_private_receiver", func(t *testing.T) {
		for _, kind := range []string{"receivables", "statements"} {
			req, _ := http.NewRequest("POST", s.base+"/erp/salesFinance/"+kind+"/export", nil)
			req.Header.Set("Authorization", "Bearer "+tokens[0])
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := io.ReadAll(res.Body)
			res.Body.Close()
			if e != nil || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("export response")
			}
			f, e := excelize.OpenReader(strings.NewReader(string(raw)))
			if e != nil {
				t.Fatal("XLSX")
			}
			rows, e := f.GetRows(f.GetSheetList()[0])
			f.Close()
			if e != nil || len(rows) < 2 {
				t.Fatal("export rows")
			}
			for _, r := range rows {
				for _, v := range r {
					if strings.Contains(v, "PRIVATE_SF9") {
						t.Fatal("private recipient leaked")
					}
				}
			}
		}
		v := s.req("GET", fmt.Sprintf("/erp/salesFinance/statements?shopId=%d", shop), tokens[0], nil)
		s.ok(v)
		if v["total"] != float64(1) {
			t.Fatal("shop statement grouping")
		}
	})
}
