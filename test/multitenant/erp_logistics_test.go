package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

func TestERPLogistics(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit local acceptance required")
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
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
		t.Fatal("local dedicated DB only")
	}
	rh, _, e := net.SplitHostPort(cfg.Redis.Addr)
	if e != nil || (rh != "localhost" && rh != "127.0.0.1" && rh != "::1") || cfg.Redis.DB != 13 {
		t.Fatal("dedicated local Redis only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	state, e := repository.InspectERPLogistics(ctx)
	cancel()
	if e != nil || state.InstalledTables != 4 || !state.Recorded {
		t.Fatal("install011 explicitly first", e)
	}
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { _ = cache.Close() })
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	t.Logf("logistics fixtures tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
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
		if !ok || n < 1 {
			s.t.Fatalf("positive %s required", k)
		}
		return int64(n)
	}
	deny := func(v map[string]any) {
		s.t.Helper()
		if v["code"] == float64(200) {
			s.t.Fatal("invalid/unauthorized request accepted")
		}
	}
	get := func(path string) map[string]any { return data(s.req("GET", path, a.owner, nil)) }
	post := func(path string, in any) map[string]any { return data(s.req("POST", path, a.owner, in)) }
	run := func(name string, f func(*testing.T)) bool {
		return t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); f(t) })
	}
	carrierInput := model.LogisticsWrite{RequestKey: uuid.NewString(), Code: "L11", Name: "物流公司原名称", Status: "0", Remark: "PRIVATE_LOGISTICS_REMARK"}
	carrier := id(post("/erp/logistics/carriers", carrierInput), "id")
	methodInput := model.LogisticsWrite{RequestKey: uuid.NewString(), Code: "STANDARD", Name: "标准方式原名称", CarrierID: carrier, Status: "0"}
	method := id(post("/erp/logistics/shippingMethods", methodInput), "id")
	if !run("tenant_isolation_original_results_and_validation", func(t *testing.T) {
		r := post("/erp/logistics/carriers", carrierInput)
		if r["existing"] != true || id(r, "id") != carrier {
			t.Fatal("original result")
		}
		bad := carrierInput
		bad.Name = "different"
		deny(s.req("POST", "/erp/logistics/carriers", a.owner, bad))
		deny(s.req("GET", fmt.Sprintf("/erp/logistics/carriers/%d", carrier), b.owner, nil))
		foreign := carrierInput
		foreign.RequestKey = uuid.NewString()
		s.ok(s.req("POST", "/erp/logistics/carriers", b.owner, foreign))
		deny(s.req("POST", "/erp/logistics/shippingMethods", b.owner, methodInput))
		deny(s.req("GET", "/erp/logistics/carriers", p, nil))
		bad.Code = "bad/code"
		bad.RequestKey = uuid.NewString()
		if s.req("POST", "/erp/logistics/carriers", a.owner, bad)["code"] != float64(400) {
			t.Fatal("explicit validation refusal must be400")
		}
		if get(fmt.Sprintf("/erp/logistics/shippingMethods/%d", method))["carrierName"] != "物流公司原名称" {
			t.Fatal("joined carrier")
		}
	}) {
		return
	}
	master := func(kind, code string) int64 {
		s.t.Helper()
		in := map[string]any{"status": "0"}
		if kind == "sku" {
			in["skuCode"] = code
			in["productName"] = "物流商品"
			in["unit"] = "件"
		} else {
			in["warehouseCode"] = code
			in["warehouseName"] = code
		}
		s.ok(s.req("POST", "/erp/"+kind, a.owner, in))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, a.owner, nil)
		s.ok(r)
		return id(obj(r["rows"].([]any)[0]), kind+"Id")
	}
	sku, w1, w2 := master("sku", "L11-SKU"), master("warehouse", "L11-W1"), master("warehouse", "L11-W2")
	newShop := func(code string) int64 {
		v := post("/erp/shop", map[string]any{"shopCode": code, "shopName": code, "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})
		shop := id(v, "shopId")
		item := id(post("/erp/shopItem", map[string]any{"shopId": shop, "shopItemCode": "ITEM", "title": "物流商品", "status": "0"}), "itemId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", item), a.owner, map[string]any{"version": 1, "skuId": sku}))
		return shop
	}
	s1, s2 := newShop("L11-S1"), newShop("L11-S2")
	for _, w := range []int64{w1, w2} {
		r := id(post("/erp/receipt", map[string]any{"warehouseId": w, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: sku, Quantity: 100}}}), "documentId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", r), a.owner, nil))
	}
	makePackage := func(w, shop int64) int64 {
		s.t.Helper()
		order := id(post("/erp/order", map[string]any{"shopId": shop, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "PRIVATE_L11_NAME", "receiverPhone": "PRIVATE_L11_PHONE", "receiverCountry": "CN", "receiverAddress": "PRIVATE_L11_ADDRESS", "orderedAt": "2026-10-25 08:00:00", "currency": "CNY", "lines": []model.ERPOrderLineInput{{SourceLineNo: "1", ShopItemCode: "ITEM", SourceTitle: "商品", Quantity: 1}}}), "orderId")
		r := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", order), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": w}))
		return id(r, "packageId")
	}
	currentPackage := makePackage(w1, s1)
	selection := model.PackageShippingInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, ShippingMethodID: method, TrackingNo: "000KNOWN"}
	savePath := fmt.Sprintf("/erp/package/%d/shipping", currentPackage)
	if !run("selection_unknown_outcome_replays_and_stock_does_not_change", func(t *testing.T) {
		before := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", w1, sku), a.owner, nil)
		loseERPResponseAfterCommit(t, s.base, a.owner, "PUT", savePath, selection)
		snapshot := get(savePath)
		if snapshot["packageVersion"] != float64(2) || obj(snapshot["snapshot"])["carrierName"] != "物流公司原名称" {
			t.Fatal("selection not committed before retry")
		}
		r := data(s.req("PUT", savePath, a.owner, selection))
		if r["existing"] != true {
			t.Fatal("selection retry executed twice")
		}
		after := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", w1, sku), a.owner, nil)
		x, _ := json.Marshal(before["rows"])
		y, _ := json.Marshal(after["rows"])
		if !bytes.Equal(x, y) {
			t.Fatal("logistics changed stock")
		}
		if get(fmt.Sprintf("/erp/package/%d", currentPackage))["version"] != snapshot["packageVersion"] {
			t.Fatal("snapshot version mismatch")
		}
	}) {
		return
	}
	// An employee may select minimal options without reading/editing company setup.
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "物流作业员", "roleKey": "l11_editor", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7500, 7501, 7502, 7503, 9300}}))
	roles := s.req("GET", "/system/role/list?roleKey=l11_editor", a.owner, nil)
	s.ok(roles)
	role := obj(roles["rows"].([]any)[0])
	rid := id(role, "roleId")
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "logistics_editor", "nickName": "物流作业测试", "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
	users := s.req("GET", "/system/user/list?userName=logistics_editor", a.owner, nil)
	s.ok(users)
	uid := id(obj(users["rows"].([]any)[0]), "userId")
	editor := s.loginAs(a.code, "logistics_editor", operationsPassword)
	t.Cleanup(func() { s.req("POST", "/logout", editor, nil) })
	grant := func(kind string, ids []int64) {
		s.t.Helper()
		path := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
		v := get(path)
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], kind + "Ids": ids}))
	}
	t.Cleanup(func() { grant("warehouse", []int64{}); grant("shop", []int64{}) })
	grant("warehouse", []int64{w1})
	grant("shop", []int64{s1})
	hidden := makePackage(w2, s2)
	hiddenIn := selection
	hiddenIn.RequestKey = uuid.NewString()
	hiddenIn.TrackingNo = "000HIDDEN"
	s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/shipping", hidden), a.owner, hiddenIn))
	checkPackage := makePackage(w1, s1)
	if !run("minimal_options_duplicate_warning_and_live_scope_do_not_leak", func(t *testing.T) {
		options := s.req("GET", "/erp/logistics/options?pageSize=999", editor, nil)
		s.ok(options)
		raw, _ := json.Marshal(options)
		if bytes.Contains(raw, []byte("PRIVATE_LOGISTICS_REMARK")) || bytes.Contains(raw, []byte("requestHash")) || bytes.Contains(raw, []byte("\"version\"")) {
			t.Fatal("options expose management fields")
		}
		deny(s.req("GET", "/erp/logistics/carriers", editor, nil))
		probe := model.TrackingCheckInput{PackageID: checkPackage, ShippingMethodID: method, TrackingNo: "000HIDDEN"}
		if data(s.req("POST", "/erp/logistics/check-tracking", editor, probe))["duplicate"] != false {
			t.Fatal("hidden duplicate leaked")
		}
		if post("/erp/logistics/check-tracking", probe)["duplicate"] != true {
			t.Fatal("owner should see duplicate")
		}
		probe.TrackingNo = "000KNOWN"
		if data(s.req("POST", "/erp/logistics/check-tracking", editor, probe))["duplicate"] != true {
			t.Fatal("visible duplicate missing")
		}
		same := selection
		same.RequestKey = uuid.NewString()
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/shipping", checkPackage), editor, same))
		grant("warehouse", []int64{})
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/shipping", checkPackage), editor, same))
		deny(s.req("GET", fmt.Sprintf("/erp/package/%d/shipping", checkPackage), editor, nil))
		grant("warehouse", []int64{w1})
		grant("shop", []int64{})
		deny(s.req("POST", "/erp/logistics/check-tracking", editor, probe))
		grant("shop", []int64{s1})
	}) {
		return
	}
	if !run("four_concurrent_master_edits_have_one_CAS_winner", func(t *testing.T) {
		v := get(fmt.Sprintf("/erp/logistics/carriers/%d", carrier))
		results := parallelJSON(s, 4, func(i int) (string, string, string, any) {
			in := carrierInput
			in.Version = id(v, "version")
			in.RequestKey = uuid.NewString()
			in.Name = fmt.Sprintf("并发名称%d", i)
			return "PUT", fmt.Sprintf("/erp/logistics/carriers/%d", carrier), a.owner, in
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
			t.Fatal("CAS winners", wins)
		}
	}) {
		return
	}
	if !run("renaming_and_disabling_preserve_saved_snapshot_and_original_replay", func(t *testing.T) {
		v := get(fmt.Sprintf("/erp/logistics/carriers/%d", carrier))
		in := carrierInput
		in.Version = id(v, "version")
		in.RequestKey = uuid.NewString()
		in.Name = "已改名物流"
		in.Status = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/logistics/carriers/%d", carrier), a.owner, in))
		m := methodInput
		m.Version = 1
		m.RequestKey = uuid.NewString()
		m.Name = "已改名方式"
		m.Status = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/logistics/shippingMethods/%d", method), a.owner, m))
		if obj(get(savePath)["snapshot"])["methodName"] != "标准方式原名称" {
			t.Fatal("snapshot renamed")
		}
		r := data(s.req("PUT", savePath, a.owner, selection))
		if r["existing"] != true {
			t.Fatal("disabled original replay")
		}
		bad := selection
		bad.Version = 2
		bad.RequestKey = uuid.NewString()
		bad.TrackingNo = "000KNOWN2"
		s.ok(s.req("PUT", savePath, a.owner, bad))
		if obj(get(savePath)["snapshot"])["carrierName"] != "物流公司原名称" {
			t.Fatal("tracking edit refreshed old name")
		}
		fresh := makePackage(w1, s1)
		newSelection := selection
		newSelection.RequestKey = uuid.NewString()
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/shipping", fresh), a.owner, newSelection))
		opts := s.req("GET", "/erp/logistics/options", editor, nil)
		s.ok(opts)
		if opts["total"] != float64(0) {
			t.Fatal("disabled options visible")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", currentPackage), a.owner, model.ERPActionToken{Version: 3, RequestKey: uuid.NewString()}))
	}) {
		return
	}
	legacy := makePackage(w1, s1)
	legacyInput := model.ERPLogisticsInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, CarrierCode: "L11", TrackingNo: "000LEGACY"}
	s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", legacy), a.owner, legacyInput))
	s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", legacy), a.owner, model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}))
	if !run("legacy_remains_unmapped_and_shipments_paginate_filter_scope", func(t *testing.T) {
		if get(fmt.Sprintf("/erp/package/%d/shipping", legacy))["snapshot"] != nil {
			t.Fatal("legacy implicitly mapped")
		}
		v := s.req("GET", "/erp/logistics/shipments?pageSize=1", a.owner, nil)
		s.ok(v)
		if v["total"] != float64(2) || len(v["rows"].([]any)) != 1 {
			t.Fatal("pagination")
		}
		raw, _ := json.Marshal(v)
		if bytes.Contains(raw, []byte("PRIVATE_L11")) || bytes.Contains(raw, []byte("receiverName")) {
			t.Fatal("shipment DTO leaks recipient")
		}
		manual := s.req("GET", "/erp/logistics/shipments?mode=MANUAL", a.owner, nil)
		s.ok(manual)
		if manual["total"] != float64(1) || obj(manual["rows"].([]any)[0])["carrierName"] != "" {
			t.Fatal("legacy name filled from current master")
		}
		catalog := s.req("GET", fmt.Sprintf("/erp/logistics/shipments?shippingMethodId=%d", method), a.owner, nil)
		s.ok(catalog)
		if catalog["total"] != float64(1) || obj(catalog["rows"].([]any)[0])["carrierName"] != "物流公司原名称" {
			t.Fatal("historical snapshot changed")
		}
		foreign := s.req("GET", "/erp/logistics/shipments", b.owner, nil)
		s.ok(foreign)
		if foreign["total"] != float64(0) {
			t.Fatal("foreign rows")
		}
		grant("shop", []int64{})
		r := s.req("GET", "/erp/logistics/shipments", editor, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("revoked shop list")
		}
		grant("shop", []int64{s1})
	}) {
		return
	}
	binary := func(token, path string) (http.Header, []byte) {
		s.t.Helper()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, e := http.NewRequestWithContext(c, "POST", s.base+path, nil)
		if e != nil {
			s.t.Fatal(e)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		res, e := s.client.Do(r)
		if e != nil {
			s.t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			s.t.Fatal(e)
		}
		if res.StatusCode != 200 {
			s.t.Fatal("binary envelope HTTP", res.StatusCode)
		}
		return res.Header, raw
	}
	if !run("export_exact_filters_text_tracking_and_binary_permission_errors", func(t *testing.T) {
		header, raw := binary(a.owner, "/erp/logistics/shipments/export?mode=CATALOG")
		if header.Get("Cache-Control") != "no-store" {
			t.Fatal("export cache")
		}
		book, e := excelize.OpenReader(bytes.NewReader(raw))
		if e != nil {
			t.Fatal("XLSX", e)
		}
		defer book.Close()
		sheet := book.GetSheetList()[0]
		cells, e := book.GetRows(sheet)
		if e != nil || len(cells) != 2 {
			t.Fatal("filtered export rows")
		}
		found := false
		for _, row := range cells {
			for _, v := range row {
				if v == "000KNOWN2" {
					found = true
				}
				if strings.Contains(v, "PRIVATE_L11") {
					t.Fatal("export recipient leak")
				}
			}
		}
		if !found {
			t.Fatal("tracking text changed")
		}
		for _, token := range []string{editor, "", p} {
			h, raw := binary(token, "/erp/logistics/shipments/export")
			var env map[string]any
			if h.Get("Content-Type") != "application/json" || json.Unmarshal(raw, &env) != nil || env["code"] == float64(200) {
				t.Fatal("binary refusal must remain exact JSON")
			}
		}
	}) {
		return
	}
	if !run("permission_revocation_and_minimal_audit", func(t *testing.T) {
		role["menuIds"] = []int64{}
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		deny(s.req("GET", "/erp/logistics/options", editor, nil))
		deny(s.req("GET", savePath, editor, nil))
		deny(s.req("PUT", savePath, editor, selection))
		ctx, c := context.WithTimeout(tenantctx.With(context.Background(), a.tenant.TenantID, false), 5*time.Second)
		defer c()
		var logs []model.SysOperLog
		if e := repository.DB(ctx).Where("method LIKE ?", "erp.%").Limit(1000).Find(&logs).Error; e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(logs)
		for _, secret := range []string{"000KNOWN", "000HIDDEN", "000LEGACY", "PRIVATE_L11", "PRIVATE_LOGISTICS_REMARK"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("private field in success audit")
			}
		}
	}) {
		return
	}
}
