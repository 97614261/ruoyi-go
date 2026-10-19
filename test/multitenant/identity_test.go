package multitenant_test

import (
	"context"
	"testing"
	"time"

	"ruoyi-go/internal/repository"
)

func runIdentityChecks(t *testing.T, parent *suite, a, b, p string) {
	t.Run("workspace_identity_and_platform_menu", func(t *testing.T) {
		s := *parent
		s.t = t
		for _, tc := range []struct {
			token, code, name, role string
			platform                bool
		}{
			{p, "platform", "平台管理", "平台超级管理员", true},
			{a, "demo", "示例企业 A", "企业管理员", false},
			{b, "demo-b", "示例企业 B", "企业管理员", false},
		} {
			info := s.req("GET", "/getInfo", tc.token, nil)
			s.ok(info)
			identity, ok := info["identity"].(map[string]any)
			if !ok || identity["tenantCode"] != tc.code || identity["tenantName"] != tc.name || identity["userName"] != "admin" || identity["platformAdmin"] != tc.platform {
				t.Fatalf("wrong workspace identity for %s", tc.code)
			}
			names, ok := identity["roleNames"].([]any)
			if !ok || len(names) != 1 || names[0] != tc.role {
				t.Fatalf("wrong role names for %s", tc.code)
			}
			routes := s.req("GET", "/getRouters", tc.token, nil)
			s.ok(routes)
			found := false
			for _, item := range routes["data"].([]any) {
				root := item.(map[string]any)
				if root["path"] != "/platform" {
					continue
				}
				for _, child := range root["children"].([]any) {
					menu := child.(map[string]any)
					if menu["path"] == "tenant" && menu["component"] == "platform/tenant/index" {
						found = true
					}
				}
			}
			if found != tc.platform {
				t.Fatalf("platform menu visibility incorrect for %s", tc.code)
			}
			if !tc.platform {
				for _, method := range []string{"POST", "PUT"} {
					if v := s.req(method, "/platform/tenant", tc.token, map[string]any{}); v["code"] != float64(403) {
						t.Fatal("customer reached platform mutation")
					}
				}
			}
		}
	})
	t.Run("platform_menu_bootstrap_is_repeatable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for i := 0; i < 2; i++ {
			if err := repository.EnsurePlatformConsole(ctx); err != nil {
				t.Fatal(err)
			}
		}
		s := *parent
		s.t = t
		s.ok(s.req("GET", "/platform/tenant/list", p, nil))
	})
}
