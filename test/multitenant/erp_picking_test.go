package multitenant_test

import (
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestERPPicking(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	host, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("local dedicated database only")
	}
	rh, _, e := net.SplitHostPort(cfg.Redis.Addr)
	if e != nil || (rh != "localhost" && rh != "127.0.0.1") || cfg.Redis.DB != 13 {
		t.Fatal("local Redis13 only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	t.Cleanup(func() { repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { cache.Close() })
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	t.Logf("picking fixtures tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
	obj := func(v any) map[string]any { return v.(map[string]any) }
	data := func(v map[string]any) map[string]any { s.ok(v); return obj(v["data"]) }
	id := func(v map[string]any, k string) int64 { return int64(v[k].(float64)) }
	deny := func(v map[string]any) {
		if v["code"] == float64(200) {
			s.t.Fatal("unexpected success")
		}
	}
	get := func(path string) map[string]any { return data(s.req("GET", path, a.owner, nil)) }
	post := func(path string, in any) map[string]any { return data(s.req("POST", path, a.owner, in)) }
	run := func(name string, f func(*testing.T)) bool {
		return t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); f(t) })
	}
	barcode := "000P01"
	cw := model.CatalogWrite{RequestKey: uuid.NewString(), ProductCode: "PICK-P", ProductName: "拣货验收商品", Variants: []model.CatalogVariant{{SkuCode: "PICK-SKU", Specification: "蓝色", Unit: "件", Status: "0", Barcode: &barcode}}}
	created := post("/erp/product", cw)
	pid := int64(created["productIds"].([]any)[0].(float64))
	product := get(fmt.Sprintf("/erp/product/%d", pid))
	sku := id(obj(product["variants"].([]any)[0]), "skuId")
	master := func(code string) int64 {
		s.ok(s.req("POST", "/erp/warehouse", a.owner, map[string]any{"warehouseCode": code, "warehouseName": "拣货仓" + code, "status": "0"}))
		v := s.req("GET", "/erp/warehouse/list?warehouseCode="+code, a.owner, nil)
		s.ok(v)
		return id(obj(v["rows"].([]any)[0]), "warehouseId")
	}
	w1, w2 := master("PICK-W1"), master("PICK-W2")
	inbound := func(w, sku int64) {
		doc := id(post("/erp/receipt", map[string]any{"warehouseId": w, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: sku, Quantity: 20}}}), "documentId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", doc), a.owner, nil))
	}
	inbound(w1, sku)
	inbound(w2, sku)
	shop := func(code string) int64 {
		sid := id(post("/erp/shop", map[string]any{"shopCode": code, "shopName": code, "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"}), "shopId")
		item := id(post("/erp/shopItem", map[string]any{"shopId": sid, "shopItemCode": "ITEM", "title": "拣货商品", "status": "0"}), "itemId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", item), a.owner, map[string]any{"version": 1, "skuId": sku}))
		return sid
	}
	s1, s2 := shop("PICK-S1"), shop("PICK-S2")
	makePackage := func(w, shop int64, extra bool) int64 {
		lines := []model.ERPOrderLineInput{{SourceLineNo: "1", ShopItemCode: "ITEM", SourceTitle: "商品1", Quantity: 2}, {SourceLineNo: "2", ShopItemCode: "ITEM", SourceTitle: "同SKU第二行", Quantity: 1}}
		if extra {
			lines = append(lines, model.ERPOrderLineInput{SourceLineNo: "3", ShopItemCode: "FALLBACK", SourceTitle: "无条码商品", Quantity: 1})
		}
		order := id(post("/erp/order", map[string]any{"shopId": shop, "sourceOrderNo": uuid.NewString(), "requestKey": uuid.NewString(), "receiverName": "PICK_PRIVATE_NAME", "receiverPhone": "PICK_PRIVATE_PHONE", "receiverCountry": "CN", "receiverAddress": "PICK_PRIVATE_ADDRESS", "orderedAt": "2026-10-26 08:00:00", "currency": "CNY", "lines": lines}), "orderId")
		v := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", order), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": w}))
		pack := id(v, "packageId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", pack), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": "PICK_PRIVATE_TRACKING"}))
		return pack
	}
	path := func(pack int64) string { return fmt.Sprintf("/erp/picking/%d", pack) }
	stock := func() map[string]any {
		v := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", w1, sku), a.owner, nil)
		s.ok(v)
		return obj(v["rows"].([]any)[0])
	}
	checkBody := func(pack int64) model.PickingCheckInput {
		sheet := get(path(pack))
		scans := []model.PickingScan{}
		for _, row := range sheet["items"].([]any) {
			v := obj(row)
			scans = append(scans, model.PickingScan{ScanCode: v["scanCode"].(string), Quantity: id(v, "quantity")})
		}
		return model.PickingCheckInput{ERPActionToken: model.ERPActionToken{Version: id(sheet, "version"), RequestKey: uuid.NewString()}, Scans: scans}
	}
	first := makePackage(w1, s1, false)
	if !run("aggregation_privacy_rejections_and_unknown_check_replay", func(t *testing.T) {
		sheet := get(path(first))
		items := sheet["items"].([]any)
		if len(items) != 1 || obj(items[0])["quantity"] != float64(3) || obj(items[0])["scanCode"] != "000P01" {
			t.Fatal("package rows not aggregated or leading zero lost")
		}
		raw, _ := json.Marshal(sheet)
		for _, secret := range []string{"PICK_PRIVATE", "trackingNo", "carrierCode", "receiver", "originalAmount"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("private fields leaked")
			}
		}
		deny(s.req("GET", path(first), b.owner, nil))
		deny(s.req("GET", path(first), p, nil))
		deny(s.req("GET", "/erp/picking/list", p, nil))
		before := stock()
		good := checkBody(first)
		for _, count := range []int64{0, 2, 4} {
			bad := good
			bad.RequestKey = uuid.NewString()
			bad.Scans = []model.PickingScan{{ScanCode: "000P01", Quantity: count}}
			deny(s.req("PUT", path(first)+"/check", a.owner, bad))
		}
		bad := good
		bad.RequestKey = uuid.NewString()
		bad.Scans = []model.PickingScan{{ScanCode: "WRONG", Quantity: 3}}
		deny(s.req("PUT", path(first)+"/check", a.owner, bad))
		loseERPResponseAfterCommit(t, s.base, a.owner, "PUT", path(first)+"/check", good)
		if get(path(first))["checked"] != true {
			t.Fatal("check not committed before retry")
		}
		if data(s.req("PUT", path(first)+"/check", a.owner, good))["existing"] != true {
			t.Fatal("check retry duplicated")
		}
		if !reflect.DeepEqual(before, stock()) {
			t.Fatal("check/reject changed inventory")
		}
	}) {
		return
	}
	if !run("barcode_and_package_version_changes_invalidate_then_checked_ship_is_atomic", func(t *testing.T) {
		before := stock()
		current := get(fmt.Sprintf("/erp/product/%d", pid))
		barcode = "000P02"
		cw.RequestKey = uuid.NewString()
		cw.Version = id(current, "version")
		cw.Variants[0].SkuID = sku
		cw.Variants[0].Barcode = &barcode
		s.ok(s.req("PUT", fmt.Sprintf("/erp/product/%d", pid), a.owner, cw))
		sheet := get(path(first))
		if sheet["checked"] != false {
			t.Fatal("barcode change kept proof valid")
		}
		deny(s.req("PUT", path(first)+"/ship", a.owner, model.ERPActionToken{Version: id(sheet, "version"), RequestKey: uuid.NewString()}))
		if !reflect.DeepEqual(before, stock()) {
			t.Fatal("invalid ship changed stock")
		}
		s.ok(s.req("PUT", path(first)+"/check", a.owner, checkBody(first)))
		sheet = get(path(first))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", first), a.owner, map[string]any{"version": sheet["version"], "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": "PICK_CHANGED_TRACKING"}))
		sheet = get(path(first))
		if sheet["checked"] != false {
			t.Fatal("package change kept proof valid")
		}
		deny(s.req("PUT", path(first)+"/ship", a.owner, model.ERPActionToken{Version: id(sheet, "version"), RequestKey: uuid.NewString()}))
		s.ok(s.req("PUT", path(first)+"/check", a.owner, checkBody(first)))
		sheet = get(path(first))
		ship := model.ERPActionToken{Version: id(sheet, "version"), RequestKey: uuid.NewString()}
		loseERPResponseAfterCommit(t, s.base, a.owner, "PUT", path(first)+"/ship", ship)
		if get(path(first))["status"] != "SHIPPED" {
			t.Fatal("ship not committed before retry")
		}
		after := stock()
		if after["onHand"] != float64(17) || after["reserved"] != float64(0) {
			t.Fatal("ship quantity")
		}
		if data(s.req("PUT", path(first)+"/ship", a.owner, ship))["existing"] != true || !reflect.DeepEqual(after, stock()) {
			t.Fatal("ship retry duplicated inventory")
		}
	}) {
		return
	}
	second, hidden := makePackage(w1, s1, false), makePackage(w2, s2, false)
	menus := []int64{6000, 6200, 9600, 9601, 9602, 9603, 7500, 7501, 7502}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "扫码作业员", "roleKey": "picking_worker", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": menus}))
	rr := s.req("GET", "/system/role/list?roleKey=picking_worker", a.owner, nil)
	s.ok(rr)
	rid := id(obj(rr["rows"].([]any)[0]), "roleId")
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "picker", "nickName": "扫码验收员", "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
	ur := s.req("GET", "/system/user/list?userName=picker", a.owner, nil)
	s.ok(ur)
	uid := id(obj(ur["rows"].([]any)[0]), "userId")
	worker := s.loginAs(a.code, "picker", operationsPassword)
	t.Cleanup(func() { s.req("POST", "/logout", worker, nil) })
	grant := func(kind string, ids []int64) {
		p := fmt.Sprintf("/erp/%s/access/%d", kind, uid)
		v := get(p)
		s.ok(s.req("PUT", p, a.owner, map[string]any{"version": v["version"], kind + "Ids": ids}))
	}
	t.Cleanup(func() { grant("warehouse", []int64{}); grant("shop", []int64{}) })
	grant("warehouse", []int64{w1})
	grant("shop", []int64{s1})
	if !run("live_scopes_separate_manual_permission_and_cross_actor_retry", func(t *testing.T) {
		s.ok(s.req("GET", path(second), worker, nil))
		deny(s.req("GET", path(hidden), worker, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", second), worker, model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}))
		check := checkBody(second)
		s.ok(s.req("PUT", path(second)+"/check", a.owner, check))
		deny(s.req("PUT", path(second)+"/check", worker, check))
		grant("warehouse", []int64{})
		deny(s.req("GET", path(second), worker, nil))
		deny(s.req("PUT", path(second)+"/ship", worker, model.ERPActionToken{Version: 3, RequestKey: uuid.NewString()}))
		listed := s.req("GET", "/erp/picking/list", worker, nil)
		s.ok(listed)
		if listed["total"] != float64(0) {
			t.Fatal("revoked ranges leaked list")
		}
		grant("warehouse", []int64{w1})
		grant("shop", []int64{})
		deny(s.req("GET", path(second), worker, nil))
		grant("shop", []int64{s1})
		withoutCheck := []int64{}
		for _, menu := range menus {
			if menu != 9602 {
				withoutCheck = append(withoutCheck, menu)
			}
		}
		role := get(fmt.Sprintf("/system/role/%d", rid))
		role["menuIds"] = withoutCheck
		s.ok(s.req("PUT", "/system/role", a.owner, role))
		denied := s.req("PUT", path(second)+"/check", worker, checkBody(second))
		if denied["code"] != float64(403) {
			t.Fatal("old worker token retained revoked check permission")
		}
		role["menuIds"] = menus
		s.ok(s.req("PUT", "/system/role", a.owner, role))
	}) {
		return
	}
	if !run("concurrent_checked_shipping_has_one_winner", func(t *testing.T) {
		version := id(get(path(second)), "version")
		commands := []model.ERPActionToken{{Version: version, RequestKey: uuid.NewString()}, {Version: version, RequestKey: uuid.NewString()}}
		tokens := []string{a.owner, worker}
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "PUT", path(second) + "/ship", tokens[i], commands[i]
		})
		wins := 0
		for i, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
				if data(s.req("PUT", path(second)+"/ship", tokens[i], commands[i]))["existing"] != true {
					t.Fatal("winner not replayed")
				}
				deny(s.req("PUT", path(second)+"/ship", tokens[1-i], commands[i]))
			}
		}
		if wins != 1 || stock()["onHand"] != float64(14) || stock()["reserved"] != float64(0) {
			t.Fatal("concurrent ship double deducted")
		}
	}) {
		return
	}
	if !run("manual_and_checked_shipping_share_one_stock_transition", func(t *testing.T) {
		ledger := func() []any {
			v := s.req("GET", fmt.Sprintf("/erp/ledger/list?warehouseId=%d&skuId=%d&pageSize=100", w1, sku), a.owner, nil)
			s.ok(v)
			return v["rows"].([]any)
		}
		// Both authorized callers are ready before either sends its request.
		// Reverse launch order on a second package, without requiring the
		// scheduler to choose a particular winner or relaxing any permission.
		for round := 0; round < 2; round++ {
			pack := makePackage(w1, s1, false)
			s.ok(s.req("PUT", path(pack)+"/check", a.owner, checkBody(pack)))
			version := id(get(path(pack)), "version")
			commands := []model.ERPActionToken{{Version: version, RequestKey: uuid.NewString()}, {Version: version, RequestKey: uuid.NewString()}}
			paths := []string{fmt.Sprintf("/erp/package/%d/ship", pack), path(pack) + "/ship"}
			tokens := []string{a.owner, worker}
			if round == 1 {
				paths[0], paths[1] = paths[1], paths[0]
				tokens[0], tokens[1] = tokens[1], tokens[0]
			}
			before, beforeLedger := stock(), ledger()
			var ready sync.WaitGroup
			ready.Add(2)
			start := make(chan struct{})
			go func() { ready.Wait(); close(start) }()
			results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
				ready.Done()
				<-start
				return "PUT", paths[i], tokens[i], commands[i]
			})
			wins := 0
			for i, r := range results {
				if r.err != nil {
					t.Fatal(r.err)
				}
				if r.code == 200 {
					wins++
					if data(s.req("PUT", paths[i], tokens[i], commands[i]))["existing"] != true {
						t.Fatal("winning shipping path did not replay original result")
					}
				} else {
					deny(s.req("PUT", paths[i], tokens[i], commands[i]))
				}
			}
			after, afterLedger := stock(), ledger()
			if wins != 1 || after["onHand"] != before["onHand"].(float64)-3 || after["reserved"] != before["reserved"].(float64)-3 || len(afterLedger) != len(beforeLedger)+1 || get(path(pack))["status"] != "SHIPPED" {
				t.Fatal("manual/checked race duplicated or lost stock transition")
			}
			// Retry both endpoints once more after shipment, proving that the
			// losing action cannot become a new independent shipment later.
			for i := range paths {
				s.req("PUT", paths[i], tokens[i], commands[i])
			}
			if !reflect.DeepEqual(after, stock()) || !reflect.DeepEqual(afterLedger, ledger()) {
				t.Fatal("cross-path retry duplicated stock or ledger")
			}
		}
	}) {
		return
	}
	// A current barcode may legally equal a different legacy SKU code. Within
	// one package this is ambiguous and must never silently choose one SKU.
	s.ok(s.req("POST", "/erp/sku", a.owner, map[string]any{"skuCode": "000P02", "productName": "无条码冲突商品", "unit": "箱", "status": "0"}))
	skus := s.req("GET", "/erp/sku/list?skuCode=000P02", a.owner, nil)
	s.ok(skus)
	legacy := id(obj(skus["rows"].([]any)[0]), "skuId")
	inbound(w1, legacy)
	item := id(post("/erp/shopItem", map[string]any{"shopId": s1, "shopItemCode": "FALLBACK", "title": "无条码商品", "status": "0"}), "itemId")
	s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", item), a.owner, map[string]any{"version": 1, "skuId": legacy}))
	ambiguous := makePackage(w1, s1, true)
	if get(path(ambiguous))["scannable"] != false {
		t.Fatal("ambiguous barcode was scannable")
	}
	deny(s.req("PUT", path(ambiguous)+"/check", a.owner, model.PickingCheckInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Scans: []model.PickingScan{{ScanCode: "000P02", Quantity: 4}}}))
	if os.Getenv("ERP_PICKING_GUI") == "1" {
		guiPack := makePackage(w1, s1, false)
		guiBefore := stock()
		dir := filepath.Join(root, "logs/erp-picking-20261026")
		if os.MkdirAll(dir, 0700) != nil {
			t.Fatal("gui dir")
		}
		fp := filepath.Join(dir, "gui-fixture.json")
		if _, e := os.Stat(fp); !os.IsNotExist(e) {
			t.Fatal("existing GUI receipt retained")
		}
		configs := s.req("GET", "/system/config/list?configKey=sys.account.captchaEnabled", a.owner, nil)
		s.ok(configs)
		var captcha map[string]any
		for _, r := range configs["rows"].([]any) {
			v := obj(r)
			if v["configKey"] == "sys.account.captchaEnabled" {
				captcha = v
			}
		}
		if captcha == nil {
			t.Fatal("captcha config")
		}
		original := captcha["configValue"]
		t.Cleanup(func() { captcha["configValue"] = original; s.ok(s.req("PUT", "/system/config", a.owner, captcha)) })
		captcha["configValue"] = "false"
		s.ok(s.req("PUT", "/system/config", a.owner, captcha))
		raw, _ := json.MarshalIndent(map[string]any{"tenantId": a.tenant.TenantID, "tenantCode": a.code, "ownerUserId": *a.tenant.OwnerUserID, "employeeId": uid, "roleId": rid, "packageId": guiPack, "warehouseId": w1, "shopId": s1, "scanCode": barcode, "quantity": 3, "originalCaptcha": original, "deadlineUtc": time.Now().Add(30 * time.Minute).UTC()}, "", "  ")
		if os.WriteFile(fp, raw, 0600) != nil {
			t.Fatal("GUI receipt")
		}
		t.Logf("PICKING_GUI_READY tenant=%s picker=picker package=%d", a.code, guiPack)
		complete := false
		deadline := time.Now().Add(30 * time.Minute)
		for time.Now().Before(deadline) {
			if _, e := os.Stat(filepath.Join(dir, "gui.done")); e == nil {
				complete = true
				break
			}
			time.Sleep(time.Second)
		}
		if !complete {
			t.Fatal("GUI expired; cleanup still runs")
		}
		guiAfter := stock()
		if get(path(guiPack))["status"] != "SHIPPED" || guiAfter["onHand"] != guiBefore["onHand"].(float64)-3 || guiAfter["reserved"] != guiBefore["reserved"].(float64)-3 {
			t.Fatal("GUI did not ship exactly once")
		}
		result, _ := json.MarshalIndent(map[string]any{"packageId": guiPack, "shipped": true, "stockOnHand": guiAfter["onHand"], "stockReserved": guiAfter["reserved"], "exactlyThreeDeducted": true}, "", "  ")
		if os.WriteFile(filepath.Join(dir, "gui-business-result.json"), result, 0600) != nil {
			t.Fatal("GUI business evidence")
		}
	}
	dir := filepath.Join(root, "logs/erp-picking-20261026")
	if os.MkdirAll(dir, 0700) != nil {
		t.Fatal("evidence dir")
	}
	raw, _ := json.MarshalIndent(map[string]any{"tenantId": a.tenant.TenantID, "tenantCode": a.code, "ownerUserId": *a.tenant.OwnerUserID, "test": "TestERPPicking", "packageId": first}, "", "  ")
	if os.WriteFile(filepath.Join(dir, "latest-fixtures.json"), raw, 0600) != nil {
		t.Fatal("evidence")
	}
}
