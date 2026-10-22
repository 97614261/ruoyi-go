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
	"reflect"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"testing"
	"time"
)

type shopReferenceFixture struct {
	TenantID     int64  `gorm:"column:tenant_id"`
	ItemID       int64  `gorm:"column:item_id;primaryKey;autoIncrement"`
	ShopID       int64  `gorm:"column:shop_id"`
	ShopItemCode string `gorm:"column:shop_item_code"`
	Title        string `gorm:"column:title"`
}

func (shopReferenceFixture) TableName() string { return "erp_shop_item" }

// Real local HTTP/DB acceptance. New customers only; retained and disabled by
// the existing fixture lifecycle. No demo resets, migration or cache clearing.
func TestERPShops(t *testing.T) {
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
		t.Fatal("invalid local config")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || dsn.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("refusing original/remote DB")
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
	body := func(code string) map[string]any {
		return map[string]any{"shopCode": code, "shopName": "店铺验收", "channelCode": "MANUAL", "siteCode": "cn", "currency": "cny", "status": "0", "remark": ""}
	}
	create := func(token, code string) int64 {
		t.Helper()
		r := s.req("POST", "/erp/shop", token, body(code))
		s.ok(r)
		return int64(r["data"].(map[string]any)["shopId"].(float64))
	}
	detail := func(token string, id int64) map[string]any {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/shop/%d", id), token, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	shopA, shopB, foreign := create(a.owner, "SAME"), create(a.owner, "SECOND"), create(b.owner, "SAME")
	ctxFor := func(f operationsFixture) context.Context {
		return erpctx.WithUser(tenantctx.With(context.Background(), f.tenant.TenantID, false), *f.tenant.OwnerUserID)
	}
	ctx, cancel := context.WithTimeout(ctxFor(a), 30*time.Second)
	defer cancel()
	var menus []model.SysMenu
	if err = repository.DB(tenantctx.System(ctx)).Where("menu_id>=7000 AND menu_id<7200").Order("menu_id").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	menuIDs := []int64{6000}
	for _, m := range menus {
		menuIDs = append(menuIDs, m.MenuID)
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "店铺验收操作员", "roleKey": "shop_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menuIDs, "menuCheckStrictly": true, "deptCheckStrictly": true}))
	role := s.req("GET", "/system/role/list?roleKey=shop_operator", a.owner, nil)
	s.ok(role)
	roleID := role["rows"].([]any)[0].(map[string]any)["roleId"]
	user := func(name string, roles []any) (int64, string) {
		t.Helper()
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": roles}))
		r := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(r)
		return int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64)), s.loginAs(a.code, name, operationsPassword)
	}
	aliceID, alice := user("shopalice", []any{roleID})
	bobID, bob := user("shopbob", []any{roleID})
	_, noFunction := user("shopnone", []any{})
	access := func(userID int64) map[string]any {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/shop/access/%d", userID), a.owner, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	grant := func(userID int64, ids []int64) {
		t.Helper()
		v := access(userID)
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/access/%d", userID), a.owner, map[string]any{"version": v["version"], "shopIds": ids}))
	}

	t.Run("directory_is_read_only_and_not_a_connected_platform", func(t *testing.T) {
		r := s.req("GET", "/erp/channel/list?pageSize=1", a.owner, nil)
		s.ok(r)
		if r["total"] != float64(6) || len(r["rows"].([]any)) != 1 {
			t.Fatal("directory pagination")
		}
		c := r["rows"].([]any)[0].(map[string]any)
		if c["supportsApi"] != false || len(c["sources"].([]any)) != 2 {
			t.Fatal("false API integration claim")
		}
		deny(s.req("GET", "/erp/channel/list", platform, nil))
		deny(s.req("GET", "/erp/channel/list", noFunction, nil))
	})
	t.Run("tenant_actor_and_function_boundaries", func(t *testing.T) {
		if detail(a.owner, shopA)["siteCode"] != "CN" {
			t.Fatal("normalization")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/shop/%d", foreign), a.owner, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/shop/%d", foreign), a.owner, body("FOREIGN")))
		deny(s.req("GET", "/erp/shop/list", platform, nil))
		deny(s.req("POST", "/erp/shop", platform, body("NO")))
		deny(s.req("GET", "/erp/shop/list", noFunction, nil))
		r := s.req("GET", "/erp/shop/list", alice, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("employee default scope must be empty")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/shop/%d", shopA), alice, nil))
		deny(s.req("POST", "/erp/shop", alice, body("EMPLOYEE")))
		edit := body("SAME")
		edit["version"] = 1
		deny(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), alice, edit))
		deny(s.req("GET", fmt.Sprintf("/erp/shop/access/%d", bobID), alice, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/shop/access/%d", bobID), alice, map[string]any{"version": 1, "shopIds": []int64{shopA}}))
		payload := body("SERVER-CONTROL")
		payload["tenantId"] = b.tenant.TenantID
		payload["shopId"] = foreign
		payload["createBy"] = "forged"
		payload["version"] = 99
		r = s.req("POST", "/erp/shop", a.owner, payload)
		s.ok(r)
		d := r["data"].(map[string]any)
		if d["tenantId"] != nil || d["createBy"] != "owner" || d["version"] != float64(1) || d["shopId"] == float64(foreign) {
			t.Fatal("client controlled server fields")
		}
	})
	t.Run("validation_and_duplicate_codes", func(t *testing.T) {
		for key, values := range map[string][]any{"shopCode": {" ", "中", "a/b"}, "shopName": {" "}, "channelCode": {"UNKNOWN"}, "siteCode": {"US/CA"}, "currency": {"C1Y"}, "status": {"", nil, "2"}} {
			for _, value := range values {
				v := body("INVALID")
				v[key] = value
				deny(s.req("POST", "/erp/shop", a.owner, v))
			}
		}
		deny(s.req("POST", "/erp/shop", a.owner, body("SAME")))
		for _, id := range []string{"0", "-1", "1.1", "bad"} {
			deny(s.req("GET", "/erp/shop/"+id, a.owner, nil))
		}
		for _, version := range []any{nil, 0, -1, 1.5, "1"} {
			v := body("SAME")
			v["version"] = version
			deny(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, v))
		}
	})
	t.Run("edit_CAS_enable_disable_and_no_physical_delete", func(t *testing.T) {
		v := body("SAME")
		v["version"] = 1
		v["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, v))
		d := detail(a.owner, shopA)
		if d["version"] != float64(2) || d["status"] != "1" {
			t.Fatal("disable or version failed")
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, v))
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			input := body("SAME")
			input["version"] = 2
			input["shopName"] = fmt.Sprintf("CAS-%d", i)
			return "PUT", fmt.Sprintf("/erp/shop/%d", shopA), a.owner, input
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
		if wins != 1 || detail(a.owner, shopA)["version"] != float64(3) {
			t.Fatal("edit CAS must have one winner")
		}
		// Route absence is checked with HTTP because the framework's 404 is not JSON.
		req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/erp/shop/%d", s.base, shopA), nil)
		req.Header.Set("Authorization", "Bearer "+a.owner)
		res, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 404 {
			t.Fatal("physical delete route exposed")
		}
		detail(a.owner, shopA)
	})
	t.Run("explicit_access_set_invalid_target_and_stale_version", func(t *testing.T) {
		path := fmt.Sprintf("/erp/shop/access/%d", aliceID)
		initial := access(aliceID)
		if initial["version"] != float64(1) || len(initial["shopIds"].([]any)) != 0 {
			t.Fatal("initial policy")
		}
		for _, payload := range []any{map[string]any{"version": 1}, map[string]any{"version": 1, "shopIds": nil}, map[string]any{"version": 1, "shopIds": []int64{shopA, foreign}}, map[string]any{"version": 1, "shopIds": []int64{shopA, shopA}}, map[string]any{"version": 1, "shopIds": []int64{0}}, map[string]any{"version": 1, "shopIds": make([]int64, 201)}} {
			deny(s.req("PUT", path, a.owner, payload))
		}
		if access(aliceID)["version"] != float64(1) {
			t.Fatal("invalid write advanced policy")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/shop/access/%d", *b.tenant.OwnerUserID), a.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/shop/access/%d", *a.tenant.OwnerUserID), a.owner, nil))
		grant(aliceID, []int64{shopA})
		s.ok(s.req("GET", fmt.Sprintf("/erp/shop/%d", shopA), alice, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/shop/%d", shopB), alice, nil))
		deny(s.req("PUT", path, a.owner, map[string]any{"version": 1, "shopIds": []int64{shopB}}))
		grant(aliceID, []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/shop/%d", shopA), alice, nil))
		s.ok(s.req("GET", "/getInfo", alice, nil))
		r := s.req("GET", "/erp/shop/list", alice, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("old session retained revoked scope")
		}
	})
	t.Run("concurrent_grant_one_winner_and_atomic_audit_failure", func(t *testing.T) {
		grant(bobID, []int64{shopA})
		before := access(bobID)
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/shop/access/%d", bobID), a.owner, map[string]any{"version": before["version"], "shopIds": []int64{[]int64{shopA, shopB}[i]}}
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
			t.Fatal("grant CAS must have one winner")
		}
		before = access(bobID)
		ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
		defer cancel()
		cb := repository.DB(ctx).Callback().Create()
		failure := errors.New("injected shop audit failure")
		if err := cb.Before("gorm:create").Register("shop-test:audit-failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		err := repository.ReplaceERPShopAccess(ctx, bobID, int64(before["version"].(float64)), []int64{shopB}, "fault-test")
		cb.Remove("shop-test:audit-failure")
		if !errors.Is(err, failure) {
			t.Fatal("audit fault not reached", err)
		}
		after := access(bobID)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("failed audit partially committed access or version")
		}
		grant(bobID, []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/shop/%d", shopA), bob, nil))
	})
	t.Run("bounded_stable_pages_and_full_200_shop_audit", func(t *testing.T) {
		// Bulk fixtures only within this new customer; normal create is tested above.
		rows := make([]model.ErpShop, 200)
		for i := range rows {
			rows[i] = model.ErpShop{ShopCode: fmt.Sprintf("BULK-%03d", i), ShopName: "分页并列", ChannelCode: "MANUAL", SiteCode: "CN", Currency: "CNY", Status: "0", Version: 1}
		}
		ctx, cancel := context.WithTimeout(ctxFor(a), 15*time.Second)
		defer cancel()
		if err := repository.DB(ctx).Create(&rows).Error; err != nil {
			t.Fatal(err)
		}
		seen := map[float64]bool{}
		total := 0.0
		for page := 1; page <= 3; page++ {
			r := s.req("GET", fmt.Sprintf("/erp/shop/list?pageNum=%d&pageSize=9999&orderByColumn=status&isAsc=asc", page), a.owner, nil)
			s.ok(r)
			total = r["total"].(float64)
			list := r["rows"].([]any)
			if len(list) > 100 {
				t.Fatal("page cap failed")
			}
			for _, row := range list {
				id := row.(map[string]any)["shopId"].(float64)
				if seen[id] {
					t.Fatal("stable pages repeated row")
				}
				seen[id] = true
			}
		}
		if len(seen) != int(total) {
			t.Fatal("stable pages lost row")
		}
		r := s.req("GET", "/erp/shop/list?orderByColumn=shop_id%20DESC;DROP%20TABLE%20erp_shop&tenantId="+fmt.Sprint(b.tenant.TenantID), a.owner, nil)
		s.ok(r)
		if r["total"] != total {
			t.Fatal("client tenant/sort bypass")
		}
		ids := make([]int64, len(rows))
		for i, row := range rows {
			ids[i] = row.ShopId
		}
		before := access(aliceID)
		grant(aliceID, ids)
		after := access(aliceID)
		if len(after["shopIds"].([]any)) != 200 {
			t.Fatal("200-shop grant incomplete")
		}
		var logs []model.SysOperLog
		if err := repository.DB(ctx).Where("method='erp.shop.access'").Order("oper_id").Find(&logs).Error; err != nil {
			t.Fatal(err)
		}
		parts, count := 0, 0
		for _, log := range logs {
			var payload map[string]any
			if json.Unmarshal([]byte(log.OperParam), &payload) != nil {
				t.Fatal("invalid audit JSON")
			}
			if payload["userId"] == float64(aliceID) && payload["previousVersion"] == before["version"] {
				parts++
				count += len(payload["shopIds"].([]any))
			}
		}
		if parts != 4 || count != 200 {
			t.Fatal("audit truncated complete set", parts, count)
		}
		grant(aliceID, []int64{})
	})
	t.Run("referenced_identity_is_immutable_but_status_and_name_remain_editable", func(t *testing.T) {
		// Temporary reference fixture, scoped to this new customer only. No stage-4
		// routes/services are implemented or made available by this test.
		ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
		defer cancel()
		fixture := shopReferenceFixture{ShopID: shopB, ShopItemCode: "REF-ONLY", Title: "reference test"}
		if err := repository.DB(ctx).Create(&fixture).Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Where("shop_id=? AND shop_item_code=?", shopB, "REF-ONLY").Delete(&fixture).Error; err != nil {
				t.Error("reference cleanup failed", err)
			}
		}()
		before := detail(a.owner, shopB)
		for key, value := range map[string]any{"shopCode": "NEW-CODE", "channelCode": "AMAZON", "siteCode": "US", "currency": "USD"} {
			v := body("SECOND")
			v["version"] = before["version"]
			v[key] = value
			deny(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopB), a.owner, v))
		}
		v := body("SECOND")
		v["version"] = before["version"]
		v["shopName"] = "已引用仍可改名称"
		v["status"] = "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shopB), a.owner, v))
		if detail(a.owner, shopB)["status"] != "1" {
			t.Fatal("referenced shop could not be disabled")
		}
	})
}
