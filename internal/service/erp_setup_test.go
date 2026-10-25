package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestERPSetupRejectsMissingOrForeignIdentityBeforeDB(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 123, false), 456)
	for _, u := range []*model.LoginUser{nil, {}, {User: &model.SysUser{TenantRecord: model.TenantRecord{TenantID: 999}, UserID: 456, UserType: "01"}}, {User: &model.SysUser{TenantRecord: model.TenantRecord{TenantID: 123}, UserID: 456, UserType: "00"}}} {
		if _, e := ERPSetup(ctx, u); e == nil {
			t.Fatal("identity accepted")
		}
		if _, e := ERPRoleTemplates(ctx, u); e == nil {
			t.Fatal("template identity accepted")
		}
	}
}
func TestERPRoleTemplateUsesOnlyOpenedMenusAndExactAncestors(t *testing.T) {
	p := func(v string) *string { return &v }
	menus := []model.SysMenu{{MenuID: 6000, Status: "0"}, {MenuID: 7900, ParentID: 6000, Status: "0", MenuName: "无价收货", Perms: p("erp:purchase:receive")}, {MenuID: 7702, ParentID: 6000, Status: "0", Perms: p("erp:purchase:query")}, {MenuID: 8400, ParentID: 6000, Status: "1", Perms: p("erp:payable:list")}}
	templates := buildERPRoleTemplates(menus)
	if len(templates) != 5 {
		t.Fatal("template set changed")
	}
	for _, r := range templates {
		if r.TemplateKey != "erp_warehouse" {
			continue
		}
		if len(r.MenuIDs) != 2 || r.MenuIDs[0] != 6000 || r.MenuIDs[1] != 7900 {
			t.Fatalf("unexpected sibling/closed grant: %v", r.MenuIDs)
		}
		if len(r.Permissions) != 1 || r.Permissions[0].Permission != "erp:purchase:receive" {
			t.Fatal("warehouse template leaks prices")
		}
		if len(r.UnavailablePermissions) == 0 {
			t.Fatal("partial template not disclosed")
		}
	}
	for _, s := range erpRoleTemplateSpecs() {
		seen := map[string]bool{}
		for _, permission := range s.permissions {
			if !strings.HasPrefix(permission, "erp:") || strings.HasSuffix(permission, ":grant") || permission == "erp:setup:list" || seen[permission] {
				t.Fatalf("unsafe template %s: %s", s.key, permission)
			}
			seen[permission] = true
			if s.key == "erp_warehouse" && (strings.Contains(permission, "finance") || strings.HasPrefix(permission, "erp:payable:") || permission == "erp:purchase:query" || permission == "erp:order:query") {
				t.Fatal("warehouse permission exposure")
			}
		}
	}
}
