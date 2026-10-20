package multitenant_test

import (
	"context"
	"fmt"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/tenantctx"
	"testing"
	"time"
)

func runAdvanced(t *testing.T, parent *suite, a, b, p string, aid, bid int64) {
	run := func(name string, fn func(*testing.T, *suite)) {
		t.Run(name, func(t *testing.T) { copy := *parent; copy.t = t; fn(t, &copy) })
	}
	findUser := func(s *suite, token, name string) int64 {
		v := s.req("GET", "/system/user/list?userName="+name, token, nil)
		s.ok(v)
		rows := v["rows"].([]any)
		if len(rows) != 1 {
			s.t.Fatalf("expected one user, got %d", len(rows))
		}
		return int64(rows[0].(map[string]any)["userId"].(float64))
	}
	run("same_username_and_privilege_injection", func(t *testing.T, s *suite) {
		name := fmt.Sprintf("mt_%d", time.Now().UnixNano()%1000000000000)
		for _, token := range []string{a, b} {
			s.ok(s.req("POST", "/system/user", token, map[string]any{"userName": name, "nickName": "acceptance", "password": "CheckOnly_782!", "status": "0", "userType": "02", "tenantId": 999999}))
			id := findUser(s, token, name)
			t.Cleanup(func() { s.ok(s.req("DELETE", fmt.Sprintf("/system/user/%d", id), token, nil)) })
			v := s.req("GET", fmt.Sprintf("/system/user/%d", id), token, nil)
			s.ok(v)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			tenant, e := repository.ResolveTenant(ctx, s.customers[0].code)
			if token == b {
				tenant, e = repository.ResolveTenant(ctx, s.customers[1].code)
			}
			if e != nil {
				t.Fatal(e)
			}
			stored, e := repository.SelectUserByID(tenantctx.With(ctx, tenant.TenantID, false), id)
			cancel()
			if e != nil || stored == nil || stored.UserType != "00" {
				t.Fatal("client escalated account kind", e)
			}
		}
	})
	run("soft_deleted_username_can_be_reused", func(t *testing.T, s *suite) {
		name := fmt.Sprintf("reuse_%d", time.Now().UnixNano()%1000000000000)
		for i := 0; i < 3; i++ {
			s.ok(s.req("POST", "/system/user", a, map[string]any{"userName": name, "nickName": fmt.Sprintf("reuse-%d", i), "password": "CheckOnly_782!", "status": "0"}))
			id := findUser(s, a, name)
			s.ok(s.req("DELETE", fmt.Sprintf("/system/user/%d", id), a, nil))
		}
	})
	run("foreign_mutation_cannot_revoke_other_tenant_session", func(t *testing.T, s *suite) {
		for _, change := range []struct {
			method, path string
			body         any
		}{
			{"PUT", "/system/user/changeStatus", map[string]any{"userId": bid, "status": "1"}},
			{"PUT", "/system/user/resetPwd", map[string]any{"userId": bid, "password": "NeverApplied_782"}},
			{"DELETE", fmt.Sprintf("/system/user/%d", bid), nil},
		} {
			v := s.req(change.method, change.path, a, change.body)
			if v["code"] == float64(200) {
				t.Fatal("foreign mutation accepted", change.path)
			}
			s.ok(s.req("GET", "/getInfo", b, nil))
		}
	})
	run("foreign_role_and_department_assignment_denied", func(t *testing.T, s *suite) {
		info := s.req("GET", "/getInfo", b, nil)
		s.ok(info)
		u := info["user"].(map[string]any)
		roleID := u["roles"].([]any)[0].(map[string]any)["roleId"]
		for _, extra := range []map[string]any{{"roleIds": []any{roleID}}, {"deptId": u["deptId"]}} {
			body := map[string]any{"userName": fmt.Sprintf("bad_%d", time.Now().UnixNano()%1000000000000), "password": "CheckOnly_782!", "status": "0"}
			for k, v := range extra {
				body[k] = v
			}
			if s.req("POST", "/system/user", a, body)["code"] == float64(200) {
				t.Fatal("foreign association accepted")
			}
		}
		ctx, cancel := context.WithTimeout(tenantctx.With(context.Background(), s.customers[0].tenant.TenantID, false), 5*time.Second)
		defer cancel()
		if err := repository.InsertUserRole(ctx, int64(roleID.(float64)), []int64{aid}); err == nil {
			t.Fatal("database allowed foreign-role relationship")
		}
	})
	run("configuration_cache_isolation", func(t *testing.T, s *suite) {
		key := fmt.Sprintf("acceptance.%d", time.Now().UnixNano())
		for _, item := range []struct{ token, value string }{{a, "tenant-a"}, {b, "tenant-b"}} {
			s.ok(s.req("POST", "/system/config", item.token, map[string]any{"configName": "acceptance", "configKey": key, "configValue": item.value, "configType": "N"}))
			v := s.req("GET", "/system/config/list?configKey="+key, item.token, nil)
			s.ok(v)
			id := int64(v["rows"].([]any)[0].(map[string]any)["configId"].(float64))
			t.Cleanup(func() { s.ok(s.req("DELETE", fmt.Sprintf("/system/config/%d", id), item.token, nil)) })
		}
		for i := 0; i < 2; i++ {
			for _, item := range []struct{ token, value string }{{a, "tenant-a"}, {b, "tenant-b"}} {
				v := s.req("GET", "/system/config/configKey/"+key, item.token, nil)
				s.ok(v)
				if v["msg"] != item.value {
					t.Fatalf("cache leaked: wanted %s got %v", item.value, v["msg"])
				}
			}
		}
	})
	run("foreign_owner_transfer_rejected", func(t *testing.T, s *suite) {
		v := s.req("GET", fmt.Sprintf("/platform/tenant/%d", s.customers[1].tenant.TenantID), p, nil)
		s.ok(v)
		row := v["data"].(map[string]any)
		row["ownerUserId"] = aid
		if s.req("PUT", "/platform/tenant", p, row)["code"] == float64(200) {
			t.Fatal("cross-tenant owner transfer accepted")
		}
		s.ok(s.req("GET", "/getInfo", b, nil))
	})
	run("owner_transfer_and_session_invalidation", func(t *testing.T, s *suite) {
		name := fmt.Sprintf("owner_%d", time.Now().UnixNano()%1000000000000)
		s.ok(s.req("POST", "/system/user", a, map[string]any{"userName": name, "password": "CheckOnly_782!", "status": "0"}))
		newOwner := findUser(s, a, name)
		v := s.req("GET", fmt.Sprintf("/platform/tenant/%d", s.customers[0].tenant.TenantID), p, nil)
		s.ok(v)
		target := v["data"].(map[string]any)
		defer func() {
			target["ownerUserId"] = aid
			s.ok(s.updateTenant(p, target))
			fresh := s.login(s.customers[0].code)
			s.ok(s.req("DELETE", fmt.Sprintf("/system/user/%d", newOwner), fresh, nil))
		}()
		target["ownerUserId"] = newOwner
		s.ok(s.updateTenant(p, target))
		if s.req("GET", "/getInfo", a, nil)["code"] == float64(200) {
			t.Fatal("old owner session still active")
		}
		ownerToken := s.loginAs(s.customers[0].code, name, "CheckOnly_782!")
		info := s.req("GET", "/getInfo", ownerToken, nil)
		s.ok(info)
		if info["user"].(map[string]any)["admin"] != true {
			t.Fatal("new owner lacks admin identity")
		}
		if s.req("GET", "/platform/tenant/list", ownerToken, nil)["code"] != float64(403) {
			t.Fatal("tenant owner became platform admin")
		}
	})
	run("tenant_creation_is_complete_and_disabled_fixture_retained", func(t *testing.T, s *suite) {
		code := fmt.Sprintf("check-%d", time.Now().UnixNano()%1000000000000)
		s.ok(s.req("POST", "/platform/tenant", p, map[string]any{"tenantCode": code, "tenantName": "自动化验收企业（结束后停用）", "userName": "admin", "password": "CheckOnly_782!", "status": "0"}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tenant, e := repository.ResolveTenant(ctx, code)
		if e != nil || tenant == nil || tenant.OwnerUserID == nil || tenant.Status != "0" {
			t.Fatal("tenant published before complete initialization", e)
		}
		defer func() {
			s.ok(s.updateTenant(p, map[string]any{"tenantId": tenant.TenantID, "tenantCode": code, "tenantName": tenant.TenantName, "status": "1"}))
		}()
		fresh := s.loginAs(code, "admin", "CheckOnly_782!")
		for _, path := range []string{"/getInfo", "/getRouters", "/system/config/list", "/system/dict/type/list", "/system/dept/list", "/system/role/list"} {
			s.ok(s.req("GET", path, fresh, nil))
		}
	})
	run("tenant_suspend_and_old_session_invalidation", func(t *testing.T, s *suite) {
		v := s.req("GET", fmt.Sprintf("/platform/tenant/%d", s.customers[1].tenant.TenantID), p, nil)
		s.ok(v)
		target := v["data"].(map[string]any)
		defer func() {
			target["status"] = "0"
			s.ok(s.updateTenant(p, target))
			_ = s.login(s.customers[1].code)
		}()
		target["status"] = "1"
		s.ok(s.updateTenant(p, target))
		if s.req("GET", "/getInfo", b, nil)["code"] == float64(200) {
			t.Fatal("suspended tenant session accepted")
		}
		if s.req("POST", "/login", "", map[string]any{"tenantCode": s.customers[1].code, "username": "admin", "password": s.passwords[s.customers[1].code]})["code"] == float64(200) {
			t.Fatal("suspended tenant login accepted")
		}
		target["status"] = "0"
		s.ok(s.updateTenant(p, target))
		if s.req("GET", "/getInfo", b, nil)["code"] == float64(200) {
			t.Fatal("old session resurrected after tenant reactivation")
		}
	})
}
