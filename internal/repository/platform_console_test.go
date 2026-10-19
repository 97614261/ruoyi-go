package repository

import (
	"strings"
	"testing"
)

func TestPlatformConsoleMenuHierarchy(t *testing.T) {
	menus := platformConsoleMenus()
	if len(menus) != 9 || menus[0].ParentID != 0 || menus[0].Path != "platform" || menus[1].ParentID != menus[0].MenuID || menus[1].Path != "tenant" {
		t.Fatal("tenant management must be nested under the platform root")
	}
	seen := map[int64]bool{}
	for _, menu := range menus {
		if seen[menu.MenuID] || menu.Perms == nil || !strings.HasPrefix(*menu.Perms, "platform:") {
			t.Fatal("platform menu must have distinct IDs and platform-only permissions")
		}
		seen[menu.MenuID] = true
	}
	if !strings.Contains(customerMenuCondition("m"), "'platform'") {
		t.Fatal("platform navigation root must be excluded from customer onboarding")
	}
}
