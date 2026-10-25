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
	"ruoyi-go/pkg/types"
	"strings"
	"testing"
	"time"
)

func TestERPOverview(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit local acceptance required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	local := func(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || !local(h) {
		t.Fatal("local dedicated DB only")
	}
	rh, _, e := net.SplitHostPort(cfg.Redis.Addr)
	if e != nil || !local(rh) || cfg.Redis.DB != 13 {
		t.Fatal("local Redis13 only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	t.Cleanup(func() { _ = repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { _ = cache.Close() })
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	t.Logf("overview fixtures tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
	obj := func(v any) map[string]any {
		s.t.Helper()
		r, ok := v.(map[string]any)
		if !ok {
			s.t.Fatal("object required")
		}
		return r
	}
	data := func(v map[string]any) map[string]any { s.t.Helper(); s.ok(v); return obj(v["data"]) }
	id := func(v map[string]any, k string) int64 {
		s.t.Helper()
		n, ok := v[k].(float64)
		if !ok || n < 1 || n != float64(int64(n)) {
			s.t.Fatalf("positive %s required", k)
		}
		return int64(n)
	}
	get := func(path string) map[string]any { return data(s.req("GET", path, a.owner, nil)) }
	post := func(path string, v any) map[string]any { return data(s.req("POST", path, a.owner, v)) }
	deny := func(v map[string]any) {
		s.t.Helper()
		if v["code"] == float64(200) {
			s.t.Fatal("unauthorized or invalid request accepted")
		}
	}
	run := func(name string, f func(*testing.T)) bool {
		return t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); f(t) })
	}
	all := []string{"orders", "shipments", "stocks", "purchases", "payables", "receivables"}
	if !run("empty_is_zero_tenant_isolation_and_platform_denial", func(t *testing.T) {
		for _, k := range all {
			r := get("/erp/overview/" + k)
			if r["available"] != true {
				t.Fatal(k, "not available")
			}
			deny(s.req("GET", "/erp/overview/"+k, p, nil))
			v := data(s.req("GET", "/erp/overview/"+k, b.owner, nil))
			if k == "payables" || k == "receivables" {
				if v["total"] != float64(0) || len(v["rows"].([]any)) != 0 {
					t.Fatal("empty finances")
				}
			}
		}
		if obj(get("/erp/overview/orders")["counts"])["total"] != float64(0) {
			t.Fatal("empty order count")
		}
		for _, c := range []string{"usd", "CNY%27%20OR%201=1", "CNY%20"} {
			deny(s.req("GET", "/erp/overview/payables?currency="+c, a.owner, nil))
		}
	}) {
		return
	}
	master := func(kind, code, unit string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"], v["productName"], v["unit"] = code, "概览测试商品", unit
		} else {
			v["warehouseCode"], v["warehouseName"] = code, code
		}
		s.ok(s.req("POST", "/erp/"+kind, a.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, a.owner, nil)
		s.ok(r)
		return id(obj(r["rows"].([]any)[0]), kind+"Id")
	}
	w1, w2, sku, other := master("warehouse", "OV-W1", ""), master("warehouse", "OV-W2", ""), master("sku", "OV-SKU", "件"), master("sku", "OV-BOX", "箱")
	for _, w := range []int64{w1, w2} {
		lines := []model.ERPLineInput{{SkuID: sku, Quantity: 100}}
		if w == w1 {
			lines = append(lines, model.ERPLineInput{SkuID: other, Quantity: 3})
		}
		doc := id(post("/erp/receipt", map[string]any{"warehouseId": w, "requestKey": uuid.NewString(), "lines": lines}), "documentId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", doc), a.owner, nil))
	}
	shop := func(code, currency string) int64 {
		sh := id(post("/erp/shop", map[string]any{"shopCode": code, "shopName": code, "channelCode": "MANUAL", "siteCode": "CN", "currency": currency, "status": "0"}), "shopId")
		it := id(post("/erp/shopItem", map[string]any{"shopId": sh, "shopItemCode": "ITEM", "title": "概览商品", "status": "0"}), "itemId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", it), a.owner, map[string]any{"version": 1, "skuId": sku}))
		return sh
	}
	s1, s2 := shop("OV-USD", "USD"), shop("OV-CNY", "CNY")
	supplier := id(post("/erp/supplier", map[string]any{"supplierCode": "OV-S", "supplierName": "概览供应商", "phone": "OV_PRIVATE_CONTACT", "status": "0"}), "supplierId")
	account := id(post("/erp/finance/accounts", model.FinAccountWrite{RequestKey: uuid.NewString(), AccountCode: "OV-USD", AccountName: "概览内部账户", AccountType: "OTHER", Currency: "USD", Status: "0"}), "id")
	type batch struct{ purchase, receipt, line, payable int64 }
	purchase := func(w, ordered, received int64, price, currency string) batch {
		po := id(post("/erp/purchase", model.ERPPurchaseWrite{SupplierID: supplier, WarehouseID: w, Currency: currency, RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: ordered, UnitPrice: price}}}), "purchaseId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", po), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		l := id(obj(get(fmt.Sprintf("/erp/purchase/%d", po))["lines"].([]any)[0]), "lineId")
		r := id(post(fmt.Sprintf("/erp/purchase/%d/receipts", po), model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: l, Quantity: received}}}), "receiptId")
		line := id(obj(get(fmt.Sprintf("/erp/purchaseReceipt/%d", r))["lines"].([]any)[0]), "lineId")
		ap := id(post("/erp/finance/payables", model.FinPayableCreate{ReceiptID: r, RequestKey: uuid.NewString()}), "id")
		return batch{po, r, line, ap}
	}
	full := purchase(w1, 10, 10, "1", "USD")
	purchase(w1, 1, 1, "5", "USD")
	purchase(w2, 1, 1, "7", "CNY")
	payment := id(post("/erp/finance/payments", model.FinPaymentWrite{RequestKey: uuid.NewString(), AccountID: account, SupplierID: supplier, Currency: "USD", Amount: "10", Allocations: []model.FinAllocationInput{{PayableID: full.payable, Version: 1, Amount: "10"}}}), "id")
	retver := id(get(fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", full.receipt)), "version")
	ret := id(post(fmt.Sprintf("/erp/purchaseReceipt/%d/returns", full.receipt), model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: retver, RequestKey: uuid.NewString()}, Reason: "概览退货口径验收", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: full.line, Quantity: 2}}}), "returnId")
	post(fmt.Sprintf("/erp/finance/payables/%d/adjust", full.payable), model.FinAdjust{FinToken: model.FinToken{Version: id(get(fmt.Sprintf("/erp/finance/payables/%d", full.payable)), "version"), RequestKey: uuid.NewString()}, ReturnIDs: []int64{ret}})
	partial := purchase(w1, 4, 2, "1", "EUR") // Separate currency; deliberately remains partially received.
	type shipment struct {
		order, pkg, ar int64
		at             string
	}
	makeOrder := func(wh, sh int64, currency, amount, mode string) shipment {
		v := map[string]any{"shopId": sh, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "OV_PRIVATE_NAME", "receiverPhone": "OV_PRIVATE_PHONE", "receiverCountry": "CN", "receiverAddress": "OV_PRIVATE_ADDRESS", "currency": currency, "orderedAt": "2026-10-25 08:00:00", "lines": []model.ERPOrderLineInput{{SourceLineNo: "1", ShopItemCode: "ITEM", SourceTitle: "商品", Quantity: 2}}}
		if amount != "" {
			v["originalAmount"] = amount
		}
		oid := id(post("/erp/order", v), "orderId")
		if mode == "pending" {
			return shipment{order: oid}
		}
		pid := id(data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", oid), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": wh})), "packageId")
		if mode == "ready" {
			return shipment{order: oid, pkg: pid}
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", pid), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": uuid.NewString()}))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", pid), a.owner, map[string]any{"version": 2, "requestKey": uuid.NewString()}))
		ar := id(post("/erp/salesFinance/receivables", model.SalesFinReceivableCreate{OrderID: oid, RequestKey: uuid.NewString()}), "id")
		at, _ := get(fmt.Sprintf("/erp/package/%d", pid))["shippedAt"].(string)
		return shipment{oid, pid, ar, at}
	}
	one := makeOrder(w1, s1, "USD", "10", "shipped")
	two := makeOrder(w1, s1, "USD", "5", "shipped")
	three := makeOrder(w2, s2, "CNY", "7", "shipped")
	makeOrder(w1, s1, "USD", "", "pending")
	makeOrder(w1, s1, "USD", "", "ready")
	collection := id(post("/erp/salesFinance/collections", model.SalesFinCollectionWrite{RequestKey: uuid.NewString(), AccountID: account, ShopID: s1, Currency: "USD", Amount: "10", Allocations: []model.SalesFinAllocationInput{{ReceivableID: one.ar, Version: 1, Amount: "10"}}}), "id")
	returnable := get(fmt.Sprintf("/erp/package/%d/returnable", one.pkg))
	packageLine := id(obj(returnable["lines"].([]any)[0]), "lineId")
	sr := id(post(fmt.Sprintf("/erp/package/%d/returns", one.pkg), model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: id(returnable, "version"), RequestKey: uuid.NewString()}, Reason: "概览超收验收", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: packageLine, Quantity: 1}}}), "returnId")
	post(fmt.Sprintf("/erp/salesFinance/receivables/%d/adjust", one.ar), model.SalesFinAdjust{FinToken: model.FinToken{Version: id(get(fmt.Sprintf("/erp/salesFinance/receivables/%d", one.ar)), "version"), RequestKey: uuid.NewString()}, ReturnID: sr, Amount: "2", Reason: "明确抵减两元"})
	s.ok(s.req("PUT", "/erp/stock/alert", a.owner, map[string]any{"warehouseId": w1, "skuId": sku, "version": 0, "thresholdQuantity": 1000000000000}))
	money := func(token, kind, currency, due, excess string) {
		s.t.Helper()
		v := data(s.req("GET", "/erp/overview/"+kind+"?currency="+currency, token, nil))
		r := v["rows"].([]any)
		if len(r) != 1 {
			s.t.Fatalf("one %s %s group required", kind, currency)
		}
		x := obj(r[0])
		if x["currency"] != currency || x["dueAmount"] != due || x["excessAmount"] != excess {
			s.t.Fatalf("%s %s got=%v due=%s excess=%s", kind, currency, x, due, excess)
		}
	}
	if !run("totals_use_rows_gross_receipts_and_per_bill_currency_balances", func(t *testing.T) {
		orders := obj(get("/erp/overview/orders")["counts"])
		if orders["pendingReview"] != float64(1) || orders["readyToShip"] != float64(1) || orders["shipped"] != float64(3) {
			t.Fatal("order counts", orders)
		}
		stocks := obj(get("/erp/overview/stocks")["counts"])
		if stocks["withStock"] != float64(3) || stocks["withReserved"] != float64(1) || stocks["low"] != float64(1) {
			t.Fatal("stock record counts", stocks)
		}
		for key, state := range map[string]string{"withStock": "HAS_STOCK", "withReserved": "HAS_RESERVED", "low": "LOW"} {
			r := s.req("GET", "/erp/stock/list?stockState="+state, a.owner, nil)
			s.ok(r)
			if r["total"] != stocks[key] {
				t.Fatal("stock drilldown mismatch", key)
			}
		}
		if get("/erp/overview/purchases")["awaitingReceipt"] != float64(1) {
			t.Fatal("return incorrectly reopened received purchase")
		}
		r := s.req("GET", "/erp/purchase/list?awaitingReceipt=true", a.owner, nil)
		s.ok(r)
		if r["total"] != float64(1) || id(obj(r["rows"].([]any)[0]), "purchaseId") != partial.purchase {
			t.Fatal("purchase drilldown")
		}
		money(a.owner, "payables", "USD", "5.0000", "2.0000")
		money(a.owner, "payables", "CNY", "7.0000", "0.0000")
		money(a.owner, "receivables", "USD", "5.0000", "2.0000")
		money(a.owner, "receivables", "CNY", "7.0000", "0.0000")
		for _, kind := range []string{"payables", "receivables"} {
			v := get("/erp/overview/" + kind + "?pageSize=1&pageNum=2")
			if len(v["rows"].([]any)) != 1 || v["total"].(float64) < 2 {
				t.Fatal("money pagination")
			}
		}
	}) {
		return
	}
	if !run("shipment_half_open_boundaries_and_original_list_match", func(t *testing.T) {
		v := get("/erp/overview/shipments")
		counts := obj(v["counts"])
		if counts["today"] != float64(3) || counts["sevenDays"] != float64(3) {
			t.Fatal("today shipments", counts)
		}
		through, e := time.ParseInLocation(types.Layout, v["through"].(string), types.Location)
		if e != nil {
			t.Fatal(e)
		}
		r := s.req("GET", "/erp/logistics/shipments?shippedFrom="+strings.ReplaceAll(v["sevenDaysFrom"].(string), " ", "%20")+"&shippedTo="+strings.ReplaceAll(through.Add(-time.Second).Format(types.Layout), " ", "%20"), a.owner, nil)
		s.ok(r)
		if r["total"] != counts["sevenDays"] {
			t.Fatal("shipment drilldown")
		}
		ctx, c := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 10*time.Second)
		defer c()
		for _, sample := range []shipment{one, two, three} {
			at, e := time.ParseInLocation(types.Layout, sample.at, types.Location)
			if e != nil {
				t.Fatal(e)
			}
			from, end := at.Format(types.Layout), at.Add(time.Second).Format(types.Layout)
			want := int64(0)
			for _, x := range []shipment{one, two, three} {
				if x.at >= from && x.at < end {
					want++
				}
			}
			out, e := repository.SelectOverviewShipments(ctx, from, from, end)
			if e != nil || out.Counts.Today != want || out.Counts.SevenDays != want {
				t.Fatal("inclusive lower boundary", e)
			}
			out, e = repository.SelectOverviewShipments(ctx, from, from, from)
			if e != nil || out.Counts.SevenDays != 0 {
				t.Fatal("exclusive upper boundary", e)
			}
		}
	}) {
		return
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "概览验收员", "roleKey": "ov_reader", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 9400}}))
	rs := s.req("GET", "/system/role/list?roleKey=ov_reader", a.owner, nil)
	s.ok(rs)
	role := id(obj(rs["rows"].([]any)[0]), "roleId")
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "overview_reader", "nickName": "概览验收员", "password": operationsPassword, "status": "0", "roleIds": []int64{role}}))
	us := s.req("GET", "/system/user/list?userName=overview_reader", a.owner, nil)
	s.ok(us)
	uid := id(obj(us["rows"].([]any)[0]), "userId")
	employee := s.loginAs(a.code, "overview_reader", operationsPassword)
	t.Cleanup(func() { s.req("POST", "/logout", employee, nil) })
	grant := func(kind string, ids []int64) {
		path := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
		v := get(path)
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], kind + "Ids": ids}))
	}
	t.Cleanup(func() { grant("warehouse", []int64{}); grant("shop", []int64{}) })
	menus := []int64{6000, 9400, 7300, 6500, 7500, 7501, 9300, 7700, 8400, 8700}
	setRole := func(ids []int64) {
		v := get(fmt.Sprintf("/system/role/%d", role))
		v["menuIds"] = ids
		s.ok(s.req("PUT", "/system/role", a.owner, v))
	}
	if !run("overview_permission_does_not_grant_business_data_and_scopes_are_live", func(t *testing.T) {
		for _, k := range all {
			deny(s.req("GET", "/erp/overview/"+k, employee, nil))
		}
		setRole(menus)
		grant("warehouse", []int64{w1})
		grant("shop", []int64{s1})
		if obj(data(s.req("GET", "/erp/overview/stocks", employee, nil))["counts"])["withStock"] != float64(2) {
			t.Fatal("hidden warehouse leaked")
		}
		for _, kind := range []string{"payables", "receivables"} {
			v := data(s.req("GET", "/erp/overview/"+kind+"?currency=CNY", employee, nil))
			if v["total"] != float64(0) {
				t.Fatal("hidden financial group leaked")
			}
			money(employee, kind, "USD", "5.0000", "2.0000")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/finance/payables/%d", full.payable), employee, nil))
		for _, k := range all {
			v := s.req("GET", fmt.Sprintf("/erp/overview/%s?tenantId=%d&userId=%d", k, b.tenant.TenantID, *b.tenant.OwnerUserID), employee, nil)
			s.ok(v)
			raw, _ := json.Marshal(v)
			for _, private := range []string{"OV_PRIVATE", "receiverName", "trackingNo", "supplierName", "warehouseId", "shopId", "tenantId"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("aggregate exposed private fields", private)
				}
			}
		}
		grant("warehouse", []int64{})
		if obj(data(s.req("GET", "/erp/overview/stocks", employee, nil))["counts"])["withStock"] != float64(0) {
			t.Fatal("old scope retained")
		}
		if data(s.req("GET", "/erp/overview/payables", employee, nil))["total"] != float64(0) {
			t.Fatal("old financial scope retained")
		}
		grant("shop", []int64{})
		if obj(data(s.req("GET", "/erp/overview/orders", employee, nil))["counts"])["total"] != float64(0) {
			t.Fatal("old shop scope retained")
		}
		setRole([]int64{6000, 7300})
		deny(s.req("GET", "/erp/overview/orders", employee, nil))
		s.ok(s.req("GET", "/erp/order/list", employee, nil))
	}) {
		return
	}
	if !run("reversal_and_close_update_overview_without_mutating_business", func(t *testing.T) {
		s.ok(s.req("POST", fmt.Sprintf("/erp/finance/payments/%d/reverse", payment), a.owner, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "概览冲销核验"}))
		s.ok(s.req("POST", fmt.Sprintf("/erp/salesFinance/collections/%d/reverse", collection), a.owner, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "概览冲销核验"}))
		money(a.owner, "payables", "USD", "13.0000", "0.0000")
		money(a.owner, "receivables", "USD", "13.0000", "0.0000")
		po := get(fmt.Sprintf("/erp/purchase/%d", partial.purchase))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/close", partial.purchase), a.owner, map[string]any{"version": po["version"], "requestKey": uuid.NewString(), "reason": "关闭剩余采购"}))
		if get("/erp/overview/purchases")["awaitingReceipt"] != float64(0) {
			t.Fatal("closed purchase retained")
		}
		before := get(fmt.Sprintf("/erp/purchase/%d", full.purchase))
		for _, k := range all {
			get("/erp/overview/" + k)
		}
		after := get(fmt.Sprintf("/erp/purchase/%d", full.purchase))
		x, _ := json.Marshal(before)
		y, _ := json.Marshal(after)
		if string(x) != string(y) {
			t.Fatal("readonly overview mutated purchase")
		}
	}) {
		return
	}
	// Optional bounded GUI lease reuses ONLY this currently running fresh
	// fixture. Default regressions never wait or disable captcha.
	if os.Getenv("ERP_OVERVIEW_GUI") == "1" {
		dir := filepath.Join(root, "logs/erp-overview-20261025")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(dir, "gui-fixture.json")
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("existing GUI receipt retained; never reopen old fixture")
		}
		setRole(menus)
		grant("warehouse", []int64{w1})
		grant("shop", []int64{s1})
		purchase(w1, 3, 1, "1", "USD")
		configs := s.req("GET", "/system/config/list?configKey=sys.account.captchaEnabled", a.owner, nil)
		s.ok(configs)
		var captcha map[string]any
		for _, v := range configs["rows"].([]any) {
			r := obj(v)
			if r["configKey"] == "sys.account.captchaEnabled" {
				captcha = r
				break
			}
		}
		if captcha == nil {
			t.Fatal("new fixture captcha config missing")
		}
		original := captcha["configValue"]
		t.Cleanup(func() { captcha["configValue"] = original; s.ok(s.req("PUT", "/system/config", a.owner, captcha)) })
		captcha["configValue"] = "false"
		s.ok(s.req("PUT", "/system/config", a.owner, captcha))
		evidence := map[string]any{"atUtc": time.Now().UTC(), "tenantId": a.tenant.TenantID, "tenantCode": a.code, "employeeId": uid, "employeeName": "overview_reader", "roleId": role, "warehouseId": w1, "shopId": s1, "originalCaptcha": original, "ownerUserId": *a.tenant.OwnerUserID, "deadlineUtc": time.Now().Add(35 * time.Minute).UTC(), "expectedPayableUSD": "14.0000", "expectedReceivableUSD": "13.0000", "expectedAwaiting": 1}
		raw, e := json.MarshalIndent(evidence, "", "  ")
		if e != nil || os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("GUI evidence")
		}
		t.Logf("OVERVIEW_GUI_READY tenant=%s employee=overview_reader tenantId=%d", a.code, a.tenant.TenantID)
		deadline := time.Now().Add(35 * time.Minute)
		complete := false
		for time.Now().Before(deadline) {
			if _, e := os.Stat(filepath.Join(dir, "gui.done")); e == nil {
				complete = true
				break
			}
			time.Sleep(time.Second)
		}
		if !complete {
			t.Fatal("GUI lease expired; cleanup still runs")
		}
	}
	receipt := map[string]any{"atUtc": time.Now().UTC(), "tenantId": a.tenant.TenantID, "tenantCode": a.code, "ownerUserId": *a.tenant.OwnerUserID, "otherTenantId": b.tenant.TenantID, "test": "TestERPOverview", "warehouseId": w1, "shopId": s1}
	raw, e := json.MarshalIndent(receipt, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "logs/erp-overview-20261025")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, fmt.Sprintf("http-fixtures-%d.json", a.tenant.TenantID)), raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "latest-fixtures.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}
