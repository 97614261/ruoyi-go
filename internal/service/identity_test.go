package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func TestIdentityUsesServerIdentityNotRoleNameOrFixedID(t *testing.T) {
	tenant := &model.SysTenant{TenantID: 10000, TenantCode: "demo", TenantName: "示例企业 A"}
	for _, tc := range []struct {
		kind            string
		roles           []string
		platform, owner bool
	}{
		{"02", []string{"平台超级管理员"}, true, false},
		{"01", []string{"企业管理员"}, false, true},
		{"00", []string{"平台超级管理员", "财务"}, false, false},
	} {
		user := &model.SysUser{UserID: 1, UserType: tc.kind, UserName: "admin", Password: "must-not-leak", Roles: []model.SysRole{
			{RoleName: "平台超级管理员", Status: "0"}, {RoleName: "财务", Status: "0"},
			{RoleName: "财务", Status: "0"}, {RoleName: "已停用", Status: "1"}, {RoleName: "已删除", Status: "0", DelFlag: "2"},
		}}
		got := buildIdentity(tenant, user)
		if got.PlatformAdmin != tc.platform || got.TenantAdmin != tc.owner || !reflect.DeepEqual(got.RoleNames, tc.roles) {
			t.Fatalf("kind=%s unexpected identity: %+v", tc.kind, got)
		}
		b, _ := json.Marshal(got)
		if strings.Contains(string(b), "password") || strings.Contains(string(b), user.Password) {
			t.Fatal("identity leaked account credentials")
		}
	}
}

func TestCurrentIdentityRejectsUnboundWorkspace(t *testing.T) {
	if _, err := CurrentIdentity(context.Background(), &model.SysUser{TenantRecord: model.TenantRecord{TenantID: 10000}}); err == nil {
		t.Fatal("identity lookup accepted an unbound workspace")
	}
}
