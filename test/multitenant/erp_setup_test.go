package multitenant_test

import (
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"testing"
	"time"
)

// Only this suite and the shared business flow may use the isolated clean-room
// listener. Never redirect the whole legacy suite or touch the original DB.
func erpEnablementTestBase(t *testing.T, cfg *config.Config) string {
	t.Helper()
	target := os.Getenv("ERP_FRESH_BASE_URL")
	if target == "" {
		return "http://127.0.0.1:8081"
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil || target != "http://127.0.0.1:18082" || d.Addr != "127.0.0.1:14306" || cfg.Redis.Addr != "127.0.0.1:16380" {
		t.Fatal("unrecognized clean-room test environment")
	}
	return target
}

func TestERPEnablement(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("dsn")
	}
	host, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("local dedicated database only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	t.Cleanup(func() { repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { cache.Close() })
	s := &suite{t: t, root: root, base: erpEnablementTestBase(t, cfg), client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Logf("enablement fixtures tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	data := func(v map[string]any) map[string]any { s.ok(v); return v["data"].(map[string]any) }
	states := func(token string) map[string]string {
		v := data(s.req("GET", "/erp/setup?tenantId=1", token, nil))
		out := map[string]string{}
		for _, r := range v["checks"].([]any) {
			x := r.(map[string]any)
			out[x["key"].(string)] = x["state"].(string)
		}
		if len(out) != 16 {
			t.Fatal("checklist shape")
		}
		return out
	}
	deny := func(v map[string]any) {
		if v["code"] == float64(200) {
			t.Fatal("expected rejection")
		}
	}
	before := states(a.owner)
	for k, v := range before {
		if v != "MISSING" {
			t.Fatalf("new enterprise %s=%s", k, v)
		}
	}
	deny(s.req("GET", "/erp/setup", p, nil))
	deny(s.req("GET", "/erp/setup/roleTemplates", p, nil))
	s.ok(s.req("POST", "/erp/warehouse", a.owner, map[string]any{"warehouseCode": "START-W", "warehouseName": "启用验收仓", "status": "0"}))
	wr := s.req("GET", "/erp/warehouse/list?warehouseCode=START-W", a.owner, nil)
	s.ok(wr)
	wid := int64(wr["rows"].([]any)[0].(map[string]any)["warehouseId"].(float64))
	if states(a.owner)["warehouses"] != "READY" || states(b.owner)["warehouses"] != "MISSING" {
		t.Fatal("tenant isolation")
	}
	templates := s.req("GET", "/erp/setup/roleTemplates", a.owner, nil)
	s.ok(templates)
	var template map[string]any
	for _, r := range templates["data"].([]any) {
		v := r.(map[string]any)
		if v["templateKey"] == "erp_warehouse" {
			template = v
		}
	}
	if template == nil {
		t.Fatal("warehouse template missing")
	}
	for _, item := range template["permissions"].([]any) {
		perm := item.(map[string]any)["permission"].(string)
		if perm == "erp:purchase:query" || perm == "erp:payable:list" || perm == "erp:order:query" {
			t.Fatal("unsafe warehouse template")
		}
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": template["name"], "roleKey": template["templateKey"], "roleSort": 1, "status": "0", "menuCheckStrictly": false, "menuIds": template["menuIds"]}))
	rr := s.req("GET", "/system/role/list?roleKey=erp_warehouse", a.owner, nil)
	s.ok(rr)
	rid := int64(rr["rows"].([]any)[0].(map[string]any)["roleId"].(float64))
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "starter", "nickName": "启用验收员工", "password": operationsPassword, "status": "0", "roleIds": []int64{rid}}))
	ur := s.req("GET", "/system/user/list?userName=starter", a.owner, nil)
	s.ok(ur)
	uid := int64(ur["rows"].([]any)[0].(map[string]any)["userId"].(float64))
	employee := s.loginAs(a.code, "starter", operationsPassword)
	t.Cleanup(func() { s.req("POST", "/logout", employee, nil) })
	deny(s.req("GET", "/erp/setup", employee, nil))
	deny(s.req("GET", "/erp/setup/roleTemplates", employee, nil))
	deny(s.req("GET", "/erp/purchase/list", employee, nil))
	deny(s.req("GET", "/erp/finance/payables", employee, nil))
	if states(a.owner)["warehouseScopes"] != "MISSING" {
		t.Fatal("template assigned range implicitly")
	}
	accessPath := fmt.Sprintf("/erp/warehouse/access/%d", uid)
	setRange := func(ids []int64) {
		v := data(s.req("GET", accessPath, a.owner, nil))
		s.ok(s.req("PUT", accessPath, a.owner, map[string]any{"version": v["version"], "warehouseIds": ids}))
	}
	setRange([]int64{wid})
	t.Cleanup(func() { setRange([]int64{}) })
	if states(a.owner)["warehouseScopes"] != "READY" {
		t.Fatal("live range not reflected")
	}
	setRange([]int64{})
	if states(a.owner)["warehouseScopes"] != "MISSING" {
		t.Fatal("revoked range still ready")
	}
	if os.Getenv("ERP_SETUP_GUI") == "1" {
		dir := filepath.Join(root, "logs/erp-enablement-20261025")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(dir, "gui-fixture.json")
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("existing GUI receipt retained; do not reopen")
		}
		configs := s.req("GET", "/system/config/list?configKey=sys.account.captchaEnabled", a.owner, nil)
		s.ok(configs)
		var captcha map[string]any
		for _, row := range configs["rows"].([]any) {
			v := row.(map[string]any)
			if v["configKey"] == "sys.account.captchaEnabled" {
				captcha = v
			}
		}
		if captcha == nil {
			t.Fatal("fixture config missing")
		}
		original := captcha["configValue"]
		t.Cleanup(func() { captcha["configValue"] = original; s.ok(s.req("PUT", "/system/config", a.owner, captcha)) })
		captcha["configValue"] = "false"
		s.ok(s.req("PUT", "/system/config", a.owner, captcha))
		evidence := map[string]any{"tenantId": a.tenant.TenantID, "tenantCode": a.code, "ownerUserId": *a.tenant.OwnerUserID, "employeeId": uid, "roleId": rid, "originalCaptcha": original, "deadlineUtc": time.Now().Add(30 * time.Minute).UTC()}
		raw, e := json.MarshalIndent(evidence, "", "  ")
		if e != nil || os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("GUI evidence")
		}
		t.Logf("SETUP_GUI_READY tenant=%s owner=owner tenantId=%d", a.code, a.tenant.TenantID)
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
	}
	menuPath := fmt.Sprintf("/platform/tenant/%d/menus", a.tenant.TenantID)
	m := data(s.req("GET", menuPath, p, nil))
	ids := []any{}
	for _, id := range m["checkedKeys"].([]any) {
		if id != float64(9500) && id != float64(7900) {
			ids = append(ids, id)
		}
	}
	s.ok(s.req("PUT", menuPath, p, map[string]any{"version": m["version"], "confirmTenantCode": a.code, "menuIds": ids, "reason": "本轮启用验收撤回功能"}))
	deny(s.req("GET", "/erp/setup", a.owner, nil))
	// Platform feature changes deliberately invalidate the old owner session.
	// Reauthenticate before checking the newly reduced permission catalogue.
	a.owner = s.loginAs(a.code, "owner", operationsPassword)
	deny(s.req("GET", "/erp/setup", a.owner, nil))
	v := s.req("GET", "/erp/setup/roleTemplates", a.owner, nil)
	s.ok(v)
	for _, r := range v["data"].([]any) {
		for _, item := range r.(map[string]any)["permissions"].([]any) {
			if item.(map[string]any)["permission"] == "erp:purchase:receive" {
				t.Fatal("revoked grant in template")
			}
		}
	}
	dir := filepath.Join(root, "logs/erp-enablement-20261025")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	raw, e := json.MarshalIndent(map[string]any{"test": "TestERPEnablement", "tenantId": a.tenant.TenantID, "tenantCode": a.code, "ownerUserId": *a.tenant.OwnerUserID}, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(dir, "latest-fixtures.json"), raw, 0600) != nil {
		t.Fatal("receipt")
	}
}
