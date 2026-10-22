package multitenant_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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

// Minimum direct fixtures exercise future-reference guards, not order features.
// They are deleted child-first by exact IDs, only inside new test customers.
type mappingOrderFixture struct {
	model.ERPRecord
	OrderID                                                                         int64 `gorm:"column:order_id;primaryKey;autoIncrement"`
	ShopID                                                                          int64
	OrderNo, SourceType, SourceOrderNo, CreateRequestKey, CreateRequestHash, Status string
	ReceiverName, ReceiverCountry, ReceiverAddress, Currency                        string
	OrderedAt                                                                       types.Time
}

func (mappingOrderFixture) TableName() string { return "erp_order" }

type mappingLineFixture struct {
	model.ERPRecord
	LineID                                                 int64 `gorm:"column:line_id;primaryKey;autoIncrement"`
	ShopID, OrderID                                        int64
	SourceLineNo, ShopItemCode, SourceTitle                string
	Quantity, ShopItemID, MappingID, MappingVersion, SkuID int64
	SkuCode, ProductName, Unit                             string
}

func (mappingLineFixture) TableName() string { return "erp_order_line" }

func TestERPShopItems(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local DB acceptance only")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local configuration unavailable")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid config")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || dsn.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("original/remote DB refused")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	deny := func(v map[string]any) {
		t.Helper()
		if v["code"] == float64(200) {
			t.Fatal("request should be rejected")
		}
	}
	shop := func(token, code string) int64 {
		t.Helper()
		r := s.req("POST", "/erp/shop", token, map[string]any{"shopCode": code, "shopName": "商品测试店铺", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})
		s.ok(r)
		return int64(r["data"].(map[string]any)["shopId"].(float64))
	}
	shopA, shopB, foreignShop := shop(a.owner, "ITEM-A"), shop(a.owner, "ITEM-B"), shop(b.owner, "ITEM-A")
	sku := func(token, code string) int64 {
		t.Helper()
		s.ok(s.req("POST", "/erp/sku", token, map[string]any{"skuCode": code, "productName": "配对验收SKU", "unit": "件", "status": "0"}))
		r := s.req("GET", "/erp/sku/list?skuCode="+code, token, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)["skuId"].(float64))
	}
	skuA, skuB, foreignSKU := sku(a.owner, "MAP-A"), sku(a.owner, "MAP-B"), sku(b.owner, "MAP-A")
	body := func(shopID int64, code string) map[string]any {
		return map[string]any{"shopId": shopID, "shopItemCode": code, "title": "变种标题", "status": "0", "externalProductId": "00001", "externalVariantId": nil, "remark": ""}
	}
	create := func(token string, shopID int64, code string) int64 {
		t.Helper()
		r := s.req("POST", "/erp/shopItem", token, body(shopID, code))
		s.ok(r)
		return int64(r["data"].(map[string]any)["itemId"].(float64))
	}
	detail := func(token string, id int64) map[string]any {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/shopItem/%d", id), token, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	mapping := func(token string, id, skuID int64) map[string]any {
		t.Helper()
		d := detail(token, id)
		r := s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), token, map[string]any{"version": d["version"], "skuId": skuID})
		s.ok(r)
		return r["data"].(map[string]any)
	}
	unmap := func(token string, id int64) map[string]any {
		t.Helper()
		d := detail(token, id)
		r := s.req("DELETE", fmt.Sprintf("/erp/shopItem/%d/mapping?version=%.0f", id, d["version"]), token, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	itemA, itemB, foreignItem := create(a.owner, shopA, "000001"), create(a.owner, shopB, "000001"), create(b.owner, foreignShop, "000001")
	ctxFor := func() context.Context {
		return erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID)
	}
	var menus []model.SysMenu
	ctx, cancel := context.WithTimeout(tenantctx.System(context.Background()), 5*time.Second)
	if err = repository.DB(ctx).Where("menu_id>=7200 AND menu_id<7300").Find(&menus).Error; err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	menuIDs := []int64{6000, 6100, 7100}
	for _, m := range menus {
		menuIDs = append(menuIDs, m.MenuID)
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "商品验收操作员", "roleKey": "item_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menuIDs}))
	r := s.req("GET", "/system/role/list?roleKey=item_operator", a.owner, nil)
	s.ok(r)
	roleID := r["rows"].([]any)[0].(map[string]any)["roleId"]
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "itememployee", "nickName": "商品验收员工", "password": operationsPassword, "status": "0", "roleIds": []any{roleID}}))
	r = s.req("GET", "/system/user/list?userName=itememployee", a.owner, nil)
	s.ok(r)
	employeeID := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
	employee := s.loginAs(a.code, "itememployee", operationsPassword)
	grant := func(ids []int64) {
		t.Helper()
		path := fmt.Sprintf("/erp/shop/access/%d", employeeID)
		r := s.req("GET", path, a.owner, nil)
		s.ok(r)
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": r["data"].(map[string]any)["version"], "shopIds": ids}))
	}
	t.Run("text_identifiers_nullable_external_ids_and_unique_scope", func(t *testing.T) {
		d := detail(a.owner, itemA)
		if d["shopItemCode"] != "000001" || d["externalProductId"] != "00001" || d["externalVariantId"] != nil || d["mappingId"] != nil {
			t.Fatal("text/null contract", d)
		}
		deny(s.req("POST", "/erp/shopItem", a.owner, body(shopA, "000001")))
		v := body(shopA, "VARIANT")
		v["externalVariantId"] = "00002"
		s.ok(s.req("POST", "/erp/shopItem", a.owner, v))
		v["shopItemCode"] = "OTHER"
		deny(s.req("POST", "/erp/shopItem", a.owner, v))
		v["shopId"] = shopB
		s.ok(s.req("POST", "/erp/shopItem", a.owner, v))
		for key, values := range map[string][]any{"shopItemCode": {" 001", "001 ", 1}, "externalProductId": {" 001", 1}, "externalVariantId": {"001 ", 1}, "status": {"2", nil}, "shopId": {0, -1, 1.5}, "title": {" "}} {
			for _, value := range values {
				v := body(shopA, "INVALID")
				v[key] = value
				deny(s.req("POST", "/erp/shopItem", a.owner, v))
			}
		}
	})
	t.Run("enterprise_platform_and_live_shop_scope", func(t *testing.T) {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "itemnone", "nickName": "无功能权限", "password": operationsPassword, "status": "0", "roleIds": []any{}}))
		none := s.loginAs(a.code, "itemnone", operationsPassword)
		deny(s.req("GET", "/erp/shopItem/list", none, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/shopItem/%d", itemA), none, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", itemA), none, map[string]any{"version": 1, "skuId": skuA}))
		deny(s.req("GET", "/erp/shopItem/list", platform, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/shopItem/%d", foreignItem), a.owner, nil))
		deny(s.req("POST", "/erp/shopItem", a.owner, body(foreignShop, "FORGED")))
		r := s.req("GET", "/erp/shopItem/list", employee, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("employee default not empty")
		}
		deny(s.req("POST", "/erp/shopItem", employee, body(shopA, "BEFORE")))
		grant([]int64{shopA})
		detail(employee, itemA)
		deny(s.req("GET", fmt.Sprintf("/erp/shopItem/%d", itemB), employee, nil))
		create(employee, shopA, "EMPLOYEE")
		deny(s.req("POST", "/erp/shopItem", employee, body(shopB, "FORGED")))
		v := body(shopB, "000001")
		v["version"] = 1
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d", itemA), employee, v))
		grant([]int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/shopItem/%d", itemA), employee, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", itemA), employee, map[string]any{"version": 1, "skuId": skuA}))
		s.ok(s.req("GET", "/getInfo", employee, nil))
		grant([]int64{shopA})
	})
	t.Run("server_owned_fields_and_no_item_delete_route", func(t *testing.T) {
		v := body(shopA, "SERVER-CONTROL")
		v["tenantId"] = b.tenant.TenantID
		v["itemId"] = foreignItem
		v["createBy"] = "forged"
		v["version"] = 99
		v["skuId"] = foreignSKU
		r := s.req("POST", "/erp/shopItem", a.owner, v)
		s.ok(r)
		d := r["data"].(map[string]any)
		if d["tenantId"] != nil || d["itemId"] == float64(foreignItem) || d["createBy"] != "owner" || d["version"] != float64(1) {
			t.Fatal("client controlled server fields")
		}
		id := int64(d["itemId"].(float64))
		if detail(a.owner, id)["mappingId"] != nil {
			t.Fatal("create bypassed mapping action")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("%s/erp/shopItem/%d", s.base, id), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+a.owner)
		res, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatal("physical item delete exposed")
		}
		detail(a.owner, id)
	})
	t.Run("item_edit_CAS_and_immutable_shop", func(t *testing.T) {
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			v := body(shopA, "000001")
			v["version"] = 1
			v["title"] = fmt.Sprintf("winner-%d", i)
			return "PUT", fmt.Sprintf("/erp/shopItem/%d", itemA), a.owner, v
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
		if wins != 1 || detail(a.owner, itemA)["version"] != float64(2) {
			t.Fatal("one edit CAS winner required")
		}
		v := body(shopB, "000001")
		v["version"] = 2
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d", itemA), a.owner, v))
		for _, version := range []any{nil, 0, 1.5, "2"} {
			v := body(shopA, "000001")
			v["version"] = version
			deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d", itemA), a.owner, v))
		}
	})
	t.Run("mapping_rebind_cancel_retains_row_and_versions", func(t *testing.T) {
		id := create(a.owner, shopA, "MAPPING")
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 1, "skuId": foreignSKU}))
		one := mapping(a.owner, id, skuA)
		if one["version"] != float64(2) || one["mappingVersion"] != float64(1) {
			t.Fatal("initial versions")
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 1, "skuId": skuB}))
		two := mapping(a.owner, id, skuB)
		three := unmap(a.owner, id)
		four := mapping(a.owner, id, skuA)
		for _, v := range []map[string]any{two, three, four} {
			if v["mappingId"] != one["mappingId"] {
				t.Fatal("mapping row replaced")
			}
		}
		if four["version"] != float64(5) || four["mappingVersion"] != float64(4) {
			t.Fatal("mapping ABA version reused")
		}
		for _, suffix := range []string{"", "?version=0", "?version=1.5", "?version=1"} {
			deny(s.req("DELETE", fmt.Sprintf("/erp/shopItem/%d/mapping%s", id, suffix), a.owner, nil))
		}
		mapping(a.owner, itemB, skuA) // multiple shops/variants may share an internal SKU.
		v := body(shopA, "CHANGED")
		v["version"] = 5
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d", id), a.owner, v))
		v = body(shopA, "MAPPING")
		v["version"] = 5
		v["title"] = "标题可变"
		v["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d", id), a.owner, v))
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 6, "skuId": skuB}))
		unmap(a.owner, id)
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", skuA), a.owner, nil))
	})
	t.Run("concurrent_mapping_and_edit_share_the_item_version", func(t *testing.T) {
		id := create(a.owner, shopA, "RACE")
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				return "PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 1, "skuId": skuA}
			}
			v := body(shopA, "RACE")
			v["version"] = 1
			v["title"] = "new"
			return "PUT", fmt.Sprintf("/erp/shopItem/%d", id), employee, v
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
		if wins != 1 || detail(a.owner, id)["version"] != float64(2) {
			t.Fatal("shared edit/mapping CAS failed")
		}
	})
	t.Run("mapping_audit_failure_rolls_back_item_and_mapping", func(t *testing.T) {
		id := create(a.owner, shopA, "FAULT")
		before := detail(a.owner, id)
		ctx, cancel := context.WithTimeout(ctxFor(), 10*time.Second)
		defer cancel()
		cb := repository.DB(ctx).Callback().Create()
		failure := errors.New("injected mapping audit fault")
		if err := cb.Before("gorm:create").Register("mapping-test:audit", func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err := repository.ChangeERPShopItemMapping(ctx, id, 1, skuA, "", "fault-test", false)
		cb.Remove("mapping-test:audit")
		if !errors.Is(err, failure) {
			t.Fatal("fault not reached", err)
		}
		after := detail(a.owner, id)
		if after["version"] != before["version"] || after["mappingId"] != nil {
			t.Fatal("failed audit partially committed")
		}
		mapping(a.owner, id, skuA)
		before = detail(a.owner, id)
		if err = cb.Before("gorm:create").Register("mapping-test:audit", func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err = repository.ChangeERPShopItemMapping(ctx, id, 2, 0, "", "fault-test", true)
		cb.Remove("mapping-test:audit")
		if !errors.Is(err, failure) {
			t.Fatal("cancel fault not reached", err)
		}
		after = detail(a.owner, id)
		if after["version"] != before["version"] || after["mappingStatus"] != "0" || after["mappingVersion"] != before["mappingVersion"] {
			t.Fatal("failed cancel partially committed")
		}
	})
	t.Run("stopped_parent_and_SKU_rules", func(t *testing.T) {
		id := create(a.owner, shopA, "STOP")
		mapping(a.owner, id, skuB)
		v := s.req("GET", fmt.Sprintf("/erp/sku/%d", skuB), a.owner, nil)
		s.ok(v)
		input := v["data"].(map[string]any)
		input["status"] = "1"
		s.ok(s.req("PUT", "/erp/sku", a.owner, input))
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 2, "skuId": skuB}))
		d := s.req("GET", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, nil)
		s.ok(d)
		shopInput := d["data"].(map[string]any)
		shopInput["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, shopInput))
		deny(s.req("POST", "/erp/shopItem", a.owner, body(shopA, "STOPPED")))
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 2, "skuId": skuA}))
		detail(a.owner, id)
		unmap(a.owner, id)
		shopInput["version"] = shopInput["version"].(float64) + 1
		shopInput["status"] = "0"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, shopInput))
		input["status"] = "0"
		s.ok(s.req("PUT", "/erp/sku", a.owner, input))
	})
	t.Run("stable_bounded_joined_pages_and_mapping_filters", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctxFor(), 10*time.Second)
		defer cancel()
		items := make([]model.ErpShopItem, 105)
		for i := range items {
			items[i] = model.ErpShopItem{ShopID: shopA, ShopItemCode: fmt.Sprintf("PAGE-%03d", i), Title: "分页并列", Status: "0", Version: 1}
		}
		if err := repository.DB(ctx).Create(&items).Error; err != nil {
			t.Fatal(err)
		}
		seen := map[float64]bool{}
		total := 0.0
		for page := 1; page <= 2; page++ {
			r := s.req("GET", fmt.Sprintf("/erp/shopItem/list?pageNum=%d&pageSize=999&orderByColumn=status&shopId=%d", page, shopA), a.owner, nil)
			s.ok(r)
			total = r["total"].(float64)
			rows := r["rows"].([]any)
			if len(rows) > 100 {
				t.Fatal("page cap")
			}
			for _, row := range rows {
				id := row.(map[string]any)["itemId"].(float64)
				if seen[id] {
					t.Fatal("duplicate joined/page row")
				}
				seen[id] = true
			}
		}
		if len(seen) != int(total) {
			t.Fatal("missing joined/page row")
		}
		for _, state := range []string{"MAPPED", "UNMAPPED"} {
			r := s.req("GET", "/erp/shopItem/list?mappingState="+state, a.owner, nil)
			s.ok(r)
			for _, row := range r["rows"].([]any) {
				active := row.(map[string]any)["mappingStatus"] == "0"
				if active != (state == "MAPPED") {
					t.Fatal("mapping filter")
				}
			}
		}
	})
	t.Run("open_order_reference_blocks_remap_terminal_history_stays_frozen", func(t *testing.T) {
		id := create(a.owner, shopA, "REFERENCE")
		m := mapping(a.owner, id, skuA)
		ctx, cancel := context.WithTimeout(ctxFor(), 10*time.Second)
		defer cancel()
		order := mappingOrderFixture{ShopID: shopA, OrderNo: "REF-ONLY", SourceType: "MANUAL", SourceOrderNo: "REF-ONLY", CreateRequestKey: "REF-ONLY", CreateRequestHash: strings.Repeat("a", 64), Status: "READY_TO_SHIP", ReceiverName: "synthetic", ReceiverCountry: "CN", ReceiverAddress: "synthetic", Currency: "CNY", OrderedAt: types.Now()}
		if err := repository.DB(ctx).Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Delete(&order).Error; err != nil {
				t.Error("order fixture cleanup", err)
			}
		}()
		line := mappingLineFixture{ShopID: shopA, OrderID: order.OrderID, SourceLineNo: "0001", ShopItemCode: "REFERENCE", SourceTitle: "synthetic", Quantity: 1, ShopItemID: id, MappingID: int64(m["mappingId"].(float64)), MappingVersion: 1, SkuID: skuA, SkuCode: "MAP-A", ProductName: "snapshot", Unit: "件"}
		if err := repository.DB(ctx).Create(&line).Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Delete(&line).Error; err != nil {
				t.Error("line fixture cleanup", err)
			}
		}()
		deny(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", id), a.owner, map[string]any{"version": 2, "skuId": skuB}))
		deny(s.req("DELETE", fmt.Sprintf("/erp/shopItem/%d/mapping?version=2", id), a.owner, nil))
		if err := repository.DB(ctx).Model(&order).Update("status", "SHIPPED").Error; err != nil {
			t.Fatal(err)
		}
		mapping(a.owner, id, skuB)
		var historical mappingLineFixture
		if err := repository.DB(ctx).Where("line_id=?", line.LineID).Take(&historical).Error; err != nil || historical.SkuID != skuA || historical.MappingVersion != 1 || historical.Unit != "件" {
			t.Fatal("historical snapshot rewritten", err)
		}
		var logs []model.SysOperLog
		if err := repository.DB(ctx).Where("method='erp.shopItem.mapping'").Find(&logs).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, log := range logs {
			var p map[string]any
			if json.Unmarshal([]byte(log.OperParam), &p) != nil {
				t.Fatal("invalid audit")
			}
			if p["itemId"] == float64(id) && p["previousSkuId"] == float64(skuA) && p["skuId"] == float64(skuB) {
				found = true
			}
		}
		if !found {
			t.Fatal("remap history missing from atomic audit")
		}
	})
}
