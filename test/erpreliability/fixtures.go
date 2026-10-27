package main

import (
	"context"
	"fmt"
	"time"

	"ruoyi-go/internal/repository"
)

type relFixture struct {
	ID                                            int64  `json:"tenantId"`
	Code                                          string `json:"code"`
	OwnerID                                       int64  `json:"ownerId"`
	Warehouse, SKU, Shop, Item, Supplier, Account int64
	ReaderID                                      int64
	Owner, Reader                                 string `json:"-"`
}
type relState struct {
	Watermark   int64         `json:"watermark"`
	Fixtures    []*relFixture `json:"fixtures"`
	SmokePassed bool          `json:"smokePassed"`
}

func fixtureContext(f *relFixture) context.Context { return context.Background() }

func readRelState() relState {
	var s relState
	must(load(relRoot+"/fixtures.json", &s))
	var audit struct {
		Watermark int64 `json:"watermarkTenantId"`
	}
	must(load(relRoot+"/pre-round-audit.json", &audit))
	if s.Watermark != audit.Watermark || len(s.Fixtures) != 3 {
		panic(fail("current-run fixture state invalid"))
	}
	for _, f := range s.Fixtures {
		if !ownedFixture(s.Watermark, f) {
			panic(fail("old fixture refused"))
		}
	}
	return s
}
func reloadFixtures(a api, s *relState) {
	for _, f := range s.Fixtures {
		t, e := repository.ResolveTenant(context.Background(), f.Code)
		must(e)
		if !ownedFixture(s.Watermark, f) || t == nil || t.TenantID != f.ID || t.OwnerUserID == nil || *t.OwnerUserID != f.OwnerID || t.Status != "0" {
			panic(fail("inactive/foreign fixture refused; no reopening"))
		}
		f.Owner = a.login(f.Code, "owner", testPassword)
		if f.ReaderID > 0 {
			f.Reader = a.login(f.Code, "reader", testPassword)
		}
	}
}
func setupFixtures(a api) relState {
	var audit struct {
		Watermark int64 `json:"watermarkTenantId"`
	}
	must(load(relRoot+"/pre-round-audit.json", &audit))
	s := relState{Watermark: audit.Watermark}
	p := platform(a)
	defer a.call("POST", "/logout", p, nil)
	for i := 0; i < 3; i++ {
		f := &relFixture{Code: fmt.Sprintf("%s-%d", runPrefix(), i)}
		a.ok("POST", "/platform/tenant", p, map[string]any{"tenantCode": f.Code, "tenantName": "Local synthetic reliability", "userName": "owner", "password": testPassword, "status": "0"})
		t, e := repository.ResolveTenant(context.Background(), f.Code)
		must(e)
		if t == nil || t.OwnerUserID == nil || t.TenantID <= s.Watermark {
			panic(fail("new tenant missing"))
		}
		f.ID, f.OwnerID = t.TenantID, *t.OwnerUserID
		s.Fixtures = append(s.Fixtures, f)
		must(save(relRoot+"/fixtures.json", s))
		f.Owner = a.login(f.Code, "owner", testPassword)
		product := data(a.ok("POST", "/erp/product", f.Owner, map[string]any{"requestKey": fmt.Sprintf("rel-product-%d", i), "productCode": "REL-P", "productName": "Synthetic reliability product", "variants": []map[string]any{{"skuCode": "REL-SKU", "specification": "test", "unit": "pcs", "status": "0", "barcode": "0009876543210"}}}))
		ids := product["productIds"].([]any)
		prodID := int64(ids[0].(float64))
		pd := data(a.ok("GET", fmt.Sprintf("/erp/product/%d", prodID), f.Owner, nil))
		f.SKU = field(pd["variants"].([]any)[0].(map[string]any), "skuId")
		a.ok("POST", "/erp/warehouse", f.Owner, map[string]any{"warehouseCode": "REL-W", "warehouseName": "Synthetic warehouse", "status": "0"})
		wh := a.ok("GET", "/erp/warehouse/list?warehouseCode=REL-W", f.Owner, nil)
		f.Warehouse = field(wh["rows"].([]any)[0].(map[string]any), "warehouseId")
		f.Shop = field(data(a.ok("POST", "/erp/shop", f.Owner, map[string]any{"shopCode": "REL-S", "shopName": "Synthetic shop", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})), "shopId")
		f.Item = field(data(a.ok("POST", "/erp/shopItem", f.Owner, map[string]any{"shopId": f.Shop, "shopItemCode": "REL-ITEM", "title": "Synthetic item", "status": "0"})), "itemId")
		a.ok("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", f.Item), f.Owner, map[string]any{"version": 1, "skuId": f.SKU})
		f.Supplier = field(data(a.ok("POST", "/erp/supplier", f.Owner, map[string]any{"supplierCode": "REL-SUP", "supplierName": "Synthetic supplier", "status": "0"})), "supplierId")
		f.Account = field(data(a.ok("POST", "/erp/finance/accounts", f.Owner, map[string]any{"requestKey": fmt.Sprintf("rel-account-%d", i), "accountCode": "REL-A", "accountName": "Synthetic internal account", "accountType": "OTHER", "currency": "CNY", "status": "0"})), "id")
		opening := data(a.ok("POST", "/erp/receipt", f.Owner, map[string]any{"warehouseId": f.Warehouse, "requestKey": fmt.Sprintf("rel-opening-%d", i), "lines": []map[string]any{{"skuId": f.SKU, "quantity": 100}}}))
		a.ok("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", field(opening, "documentId")), f.Owner, nil)
		must(save(relRoot+"/fixtures.json", s))
	}
	f := s.Fixtures[0]
	_, dsn := sourceConfig()
	db := openDB(dsn)
	defer db.Close()
	rows, e := db.Query("SELECT menu_id FROM sys_menu WHERE menu_id=6000 OR perms IN ('erp:order:list','erp:stock:list','erp:overview:list') ORDER BY menu_id")
	must(e)
	menus := []int64{}
	for rows.Next() {
		var n int64
		must(rows.Scan(&n))
		menus = append(menus, n)
	}
	must(rows.Err())
	must(rows.Close())
	a.ok("POST", "/system/role", f.Owner, map[string]any{"roleName": "Reliability reader", "roleKey": "rel_reader", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": menus})
	roles := a.ok("GET", "/system/role/list?roleKey=rel_reader", f.Owner, nil)
	rid := field(roles["rows"].([]any)[0].(map[string]any), "roleId")
	a.ok("POST", "/system/user", f.Owner, map[string]any{"userName": "reader", "nickName": "Synthetic reader", "password": testPassword, "status": "0", "roleIds": []int64{rid}})
	users := a.ok("GET", "/system/user/list?userName=reader", f.Owner, nil)
	f.ReaderID = field(users["rows"].([]any)[0].(map[string]any), "userId")
	for _, kind := range []string{"shop", "warehouse"} {
		id := f.Shop
		if kind == "warehouse" {
			id = f.Warehouse
		}
		setRelRange(a, f, kind, []int64{id})
	}
	f.Reader = a.login(f.Code, "reader", testPassword)
	for _, path := range []string{"/erp/order/list", "/erp/order/workbench", "/erp/stock/list", "/erp/overview/orders"} {
		a.ok("GET", path, f.Reader, nil)
	}
	must(save(relRoot+"/fixtures.json", s))
	return s
}
func setRelRange(a api, f *relFixture, kind string, ids []int64) {
	path := fmt.Sprintf("/erp/%s/access/%d", kind, f.ReaderID)
	v := data(a.ok("GET", path, f.Owner, nil))
	a.ok("PUT", path, f.Owner, map[string]any{"version": v["version"], kind + "Ids": ids})
}
func closeFixtures(a api, s relState) {
	p := platform(a)
	defer a.call("POST", "/logout", p, nil)
	for _, f := range s.Fixtures {
		validateFixture(s.Watermark, f)
		if f.Owner == "" {
			f.Owner = a.login(f.Code, "owner", testPassword)
		}
		if f.ReaderID > 0 {
			setRelRange(a, f, "shop", []int64{})
			setRelRange(a, f, "warehouse", []int64{})
			a.call("POST", "/logout", f.Reader, nil)
		}
		v := data(a.ok("GET", fmt.Sprintf("/platform/tenant/%d", f.ID), p, nil))
		if v["tenantCode"] != f.Code {
			panic(fail("cleanup identity drift"))
		}
		v["status"] = "1"
		a.ok("PUT", "/platform/tenant", p, v)
		a.call("POST", "/logout", f.Owner, nil)
	}
	must(save(relRoot+"/fixture-cleanup.json", map[string]any{"atUtc": time.Now().UTC(), "disabled": true, "ranges": 0, "reopened": false, "fixtures": s.Fixtures}))
}
