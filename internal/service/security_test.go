package service

import (
	"context"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func TestLoginLengthValidationRunsBeforeLookup(t *testing.T) {
	for _, test := range []struct {
		username string
		password string
	}{
		{username: "a", password: "12345"},
		{username: strings.Repeat("u", 21), password: "12345"},
		{username: "valid", password: "1234"},
		{username: "valid", password: strings.Repeat("p", 21)},
	} {
		if err := loginPreCheck(context.Background(), test.username, test.password, "127.0.0.1"); err == nil {
			t.Errorf("invalid login length accepted: username=%q passwordLen=%d", test.username, len(test.password))
		}
	}
}

func TestPasswordLengthCountsUnicodeCharacters(t *testing.T) {
	if !validPasswordLength("密码安全测试甲") {
		t.Fatal("five-or-more non-ASCII characters should not be rejected by byte length")
	}
	if validPasswordLength("密码四字") {
		t.Fatal("four Unicode characters must be rejected")
	}
}

func TestUserStatusIsStrict(t *testing.T) {
	for _, status := range []string{model.StatusNormal, model.StatusDisable} {
		if err := checkUserStatus(status); err != nil {
			t.Fatalf("valid status %q rejected: %v", status, err)
		}
	}
	for _, status := range []string{"", "2", "normal"} {
		if err := checkUserStatus(status); err == nil {
			t.Fatalf("invalid status %q accepted", status)
		}
	}
}

func TestReservedAdminRoleCannotBeAssignedToOrdinaryUser(t *testing.T) {
	if err := checkReservedAdminRole(2, []int64{model.AdminRoleID}); err == nil {
		t.Fatal("ordinary user accepted reserved admin role")
	}
	if err := checkReservedAdminRole(model.AdminUserID, []int64{model.AdminRoleID}); err != nil {
		t.Fatalf("admin user should retain its reserved role: %v", err)
	}
}

func TestIPMatchesFilter(t *testing.T) {
	filter := "10.0.0.1;192.168.1.*;172.16.0.10-172.16.0.20;2001:db8::/32"
	for _, ip := range []string{"10.0.0.1", "192.168.1.99", "172.16.0.15", "2001:db8::1"} {
		if !ipMatchesFilter(filter, ip) {
			t.Errorf("%s should match %q", ip, filter)
		}
	}
	for _, ip := range []string{"10.0.0.2", "192.168.2.1", "172.16.0.21", "invalid"} {
		if ipMatchesFilter(filter, ip) {
			t.Errorf("%s should not match %q", ip, filter)
		}
	}
}

func TestNormalizeRelationIDs(t *testing.T) {
	got, err := normalizeRelationIDs([]int64{3, 1, 3, 2}, "测试")
	if err != nil || len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("unexpected normalized IDs: %v err=%v", got, err)
	}
	if _, err := normalizeRelationIDs([]int64{0}, "测试"); err == nil {
		t.Fatal("zero relation ID must be rejected")
	}
	if _, err := normalizeRelationIDs(make([]int64, maxRelationIDs+1), "测试"); err == nil {
		t.Fatal("oversized relation list must be rejected")
	}
}

func TestMenuOutputEscapesStoredMarkup(t *testing.T) {
	order := 1
	menu := &model.SysMenu{
		MenuID: 2, MenuName: `<img src=x onerror=alert(1)>`, Path: `safe<img>`,
		ParentID: MenuRootID, MenuType: model.MenuTypeDir, IsFrame: model.MenuIsFrameNo,
		OrderNum: &order,
	}
	tree := BuildMenuTreeSelect([]model.SysMenu{*menu})
	if len(tree) != 1 || strings.Contains(tree[0].Label, "<img") {
		t.Fatalf("tree label was not escaped: %#v", tree)
	}
	routes := BuildRouters([]*model.SysMenu{menu})
	if len(routes) != 1 || strings.Contains(routes[0].Path, "<") ||
		routes[0].Meta == nil || strings.Contains(routes[0].Meta.Title, "<img") {
		t.Fatalf("router output was not escaped: %#v", routes)
	}
}
