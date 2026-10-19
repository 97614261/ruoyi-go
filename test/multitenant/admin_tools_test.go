package multitenant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/tenantctx"
)

func runTenantAdminTools(t *testing.T, parent *suite, other, platform string) {
	t.Run("tenant_admin_tools", func(t *testing.T) {
		s := *parent
		s.t = t
		code := fmt.Sprintf("tools-%d", time.Now().UnixNano()%1000000000000)
		const originalPassword, newPassword, memberPassword = "Tools_Owner_782!", "Tools_New_927!", "Tools_Member_927!"
		s.ok(s.req("POST", "/platform/tenant", platform, map[string]any{"tenantCode": code, "tenantName": "企业运维验收（完成后停用）", "userName": "owner", "password": originalPassword, "status": "0"}))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tenant, err := repository.ResolveTenant(ctx, code)
		if err != nil || tenant == nil || tenant.OwnerUserID == nil {
			t.Fatal("fixture initialization failed")
		}
		t.Cleanup(func() {
			s.ok(s.updateTenant(platform, map[string]any{"tenantId": tenant.TenantID, "tenantCode": code, "tenantName": tenant.TenantName, "status": "1"}))
		})
		owner := s.loginAs(code, "owner", originalPassword)
		menuPath := fmt.Sprintf("/platform/tenant/%d/menus", tenant.TenantID)
		resetPath := fmt.Sprintf("/platform/tenant/%d/resetOwnerPwd", tenant.TenantID)
		settings := func() map[string]any {
			v := s.req("GET", menuPath, platform, nil)
			s.ok(v)
			return v["data"].(map[string]any)
		}
		initial := settings()
		var flatten func([]any, string) int64
		flatten = func(nodes []any, label string) int64 {
			for _, raw := range nodes {
				n := raw.(map[string]any)
				if n["label"] == label {
					return int64(n["id"].(float64))
				}
				if children, ok := n["children"].([]any); ok {
					if id := flatten(children, label); id > 0 {
						return id
					}
				}
			}
			return 0
		}
		postID, userID := flatten(initial["menus"].([]any), "岗位管理"), flatten(initial["menus"].([]any), "用户管理")
		if postID == 0 || userID == 0 {
			t.Fatal("customer menu catalog incomplete")
		}
		full := []int64{}
		for _, key := range initial["checkedKeys"].([]any) {
			full = append(full, int64(key.(float64)))
		}
		roleName := "工具验收角色"
		s.ok(s.req("POST", "/system/role", owner, map[string]any{"roleName": roleName, "roleKey": "tools_read", "roleSort": 1, "status": "0", "menuIds": []int64{userID}}))
		roleResult := s.req("GET", "/system/role/list?roleName="+roleName, owner, nil)
		s.ok(roleResult)
		roleID := int64(roleResult["rows"].([]any)[0].(map[string]any)["roleId"].(float64))
		s.ok(s.req("POST", "/system/user", owner, map[string]any{"userName": "member", "nickName": "验收员工", "password": memberPassword, "status": "0", "roleIds": []int64{roleID}}))
		member := s.loginAs(code, "member", memberPassword)
		s.ok(s.req("GET", "/system/user/list", member, nil))
		grant := func(ids []int64) {
			v := settings()
			s.ok(s.req("PUT", menuPath, platform, map[string]any{"version": v["version"], "menuIds": ids, "confirmTenantCode": code, "reason": "自动化验证企业功能授权"}))
			s.ok(s.req("GET", "/getInfo", other, nil))
			s.ok(s.req("GET", "/getInfo", platform, nil))
		}
		run := func(name string, fn func()) bool {
			return t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); fn() })
		}
		if !run("grant_input_and_platform_boundary", func() {
			for _, token := range []string{owner, other} {
				for _, path := range []string{menuPath, resetPath} {
					if s.req("PUT", path, token, map[string]any{})["code"] != float64(403) {
						s.t.Fatal("customer reached platform write")
					}
				}
				if s.req("GET", menuPath, token, nil)["code"] != float64(403) {
					s.t.Fatal("customer read platform menu settings")
				}
			}
			for _, ids := range [][]int64{nil, {5000}, {999999999}} {
				body := map[string]any{"version": initial["version"], "confirmTenantCode": code, "reason": "无效授权负例"}
				if ids != nil {
					body["menuIds"] = ids
				}
				if s.req("PUT", menuPath, platform, body)["code"] == float64(200) {
					s.t.Fatal("accepted missing/forbidden menu selection")
				}
			}
			if settings()["version"] != initial["version"] {
				s.t.Fatal("invalid grants changed tenant version")
			}
			if s.req("GET", "/platform/tenant/1/menus", platform, nil)["code"] == float64(200) {
				s.t.Fatal("platform treated as customer")
			}
		}) {
			return
		}
		if !run("grant_revokes_sessions_permissions_and_role_rows", func() {
			grant([]int64{postID})
			if s.req("GET", "/getInfo", owner, nil)["code"] != float64(401) {
				s.t.Fatal("old owner session survived grant")
			}
			if s.req("GET", "/getInfo", member, nil)["code"] != float64(401) {
				s.t.Fatal("old employee session survived grant")
			}
			owner = s.loginAs(code, "owner", originalPassword)
			s.ok(s.req("GET", "/system/post/list", owner, nil))
			if s.req("GET", "/system/user/list", owner, nil)["code"] != float64(403) {
				s.t.Fatal("revoked endpoint remains accessible")
			}
			remaining, err := repository.SelectMenuIDsByRoleID(tenantctx.With(ctx, tenant.TenantID, false), roleID, false)
			if err != nil || len(remaining) != 0 {
				s.t.Fatal("normal role retained revoked rows")
			}
			if s.req("PUT", menuPath, platform, map[string]any{"version": initial["version"], "menuIds": full, "confirmTenantCode": code, "reason": "过期版本负例"})["code"] == float64(200) {
				s.t.Fatal("stale grant overwrote current state")
			}
		}) {
			return
		}
		if !run("reopening_does_not_restore_normal_role_permissions", func() {
			grant(full)
			owner = s.loginAs(code, "owner", originalPassword)
			s.ok(s.req("GET", "/system/user/list", owner, nil))
			member = s.loginAs(code, "member", memberPassword)
			if s.req("GET", "/system/user/list", member, nil)["code"] != float64(403) {
				s.t.Fatal("normal role regained withdrawn privilege")
			}
		}) {
			return
		}
		if !run("explicit_empty_grant_and_recovery", func() {
			grant([]int64{})
			owner = s.loginAs(code, "owner", originalPassword)
			s.ok(s.req("GET", "/getInfo", owner, nil))
			if s.req("GET", "/system/post/list", owner, nil)["code"] != float64(403) {
				s.t.Fatal("empty grant still exposes features")
			}
			if len(settings()["checkedKeys"].([]any)) != 0 {
				s.t.Fatal("empty grant was not saved")
			}
			grant(full)
			owner = s.loginAs(code, "owner", originalPassword)
			member = s.loginAs(code, "member", memberPassword)
		}) {
			return
		}
		resetBody := func() map[string]any {
			state := settings()
			return map[string]any{"version": state["version"], "ownerUserId": *tenant.OwnerUserID, "confirmTenantCode": code, "newPassword": newPassword, "confirmPassword": newPassword, "operatorPassword": s.passwords["platform"], "reason": "自动化验证企业管理员重置"}
		}
		if !run("reset_requires_step_up_target_confirmation_and_version", func() {
			for _, change := range []func(map[string]any){
				func(v map[string]any) { delete(v, "operatorPassword") },
				func(v map[string]any) { v["operatorPassword"] = "Wrong_Operator_927!" },
				func(v map[string]any) { v["confirmTenantCode"] = "demo-b" },
				func(v map[string]any) { v["ownerUserId"] = 1 },
				func(v map[string]any) { v["version"] = 1 },
				func(v map[string]any) { v["confirmPassword"] = "different" },
			} {
				v := resetBody()
				change(v)
				if s.req("PUT", resetPath, platform, v)["code"] == float64(200) {
					s.t.Fatal("unsafe password reset accepted")
				}
			}
			if s.req("PUT", "/platform/tenant/1/resetOwnerPwd", platform, resetBody())["code"] == float64(200) {
				s.t.Fatal("customer reset changed platform owner")
			}
			s.ok(s.req("GET", "/getInfo", owner, nil))
			s.ok(s.req("GET", "/getInfo", platform, nil))
		}) {
			return
		}
		if !run("reset_invalidates_only_target_workspace_and_clears_retry_counter", func() {
			if s.loginResponse(code, "owner", "Wrong_Target_927!")["code"] == float64(200) {
				s.t.Fatal("wrong credential accepted")
			}
			v := s.req("PUT", resetPath, platform, resetBody())
			s.ok(v)
			if v["data"].(map[string]any)["retryCounterCleared"] != true {
				s.t.Fatal("reset did not clear login lock counter")
			}
			for _, token := range []string{owner, member} {
				if s.req("GET", "/getInfo", token, nil)["code"] != float64(401) {
					s.t.Fatal("target old session survived reset")
				}
			}
			s.ok(s.req("GET", "/getInfo", other, nil))
			s.ok(s.req("GET", "/getInfo", platform, nil))
			if s.loginResponse(code, "owner", originalPassword)["code"] == float64(200) {
				s.t.Fatal("old owner password survived reset")
			}
			owner = s.loginAs(code, "owner", newPassword)
			s.ok(s.req("GET", "/getInfo", owner, nil))
			member = s.loginAs(code, "member", memberPassword)
			s.ok(s.req("GET", "/getInfo", member, nil))
			if count, err := s.redis.Exists(ctx, redisx.TenantPasswordKey(tenantctx.With(ctx, tenant.TenantID, false), "owner")).Result(); err != nil || count != 0 {
				s.t.Fatal("owner login retry counter not cleared")
			}
		}) {
			return
		}
		run("sensitive_action_audit_contains_no_credentials", func() {
			for _, title := range []string{"重置企业管理员密码", "企业管理员重置请求", "企业功能授权"} {
				v := s.req("GET", "/monitor/operlog/list?title="+title+"&pageSize=100", platform, nil)
				s.ok(v)
				encoded, _ := json.Marshal(v)
				for _, secret := range []string{originalPassword, newPassword, s.passwords["platform"], "Wrong_Operator_927!"} {
					if strings.Contains(string(encoded), secret) {
						s.t.Fatal("credential leaked into audit")
					}
				}
				if title == "重置企业管理员密码" {
					found := false
					for _, raw := range v["rows"].([]any) {
						row := raw.(map[string]any)
						params, _ := row["operParam"].(string)
						if strings.Contains(params, code) && row["status"] == float64(model.OperStatusSuccess) {
							found = true
						}
					}
					if !found {
						s.t.Fatal("successful credential reset has no transactional audit")
					}
				}
			}
		})
	})
}
