package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
)

func TestTenantMenuNormalizationClosesAncestorsWithoutGrantingSiblings(t *testing.T) {
	catalog := []model.SysMenu{{MenuID: 1}, {MenuID: 2, ParentID: 1}, {MenuID: 3, ParentID: 2}, {MenuID: 4, ParentID: 2}}
	got, err := normalizeTenantMenus([]int64{3, 3}, catalog)
	if err != nil || !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("bad closure: %v %v", got, err)
	}
	got, err = normalizeTenantMenus([]int64{}, catalog)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatal("explicit empty grant must be supported")
	}
	for _, ids := range [][]int64{{0}, {-1}, {5000}, make([]int64, maxTenantMenuNodes+1)} {
		if _, err := normalizeTenantMenus(ids, catalog); err == nil {
			t.Fatal("accepted invalid, forbidden or oversized selection")
		}
	}
}

func TestTenantMenuCatalogDropsOrphanCyclesAndTooDeepNodes(t *testing.T) {
	catalog := []model.SysMenu{{MenuID: 1}, {MenuID: 2, ParentID: 1}, {MenuID: 3, ParentID: 99}, {MenuID: 4, ParentID: 5}, {MenuID: 5, ParentID: 4}}
	if got := reachableCustomerMenus(catalog); len(got) != 2 {
		t.Fatalf("bad catalog: %+v", got)
	}
	if _, err := normalizeTenantMenus([]int64{4}, catalog); err == nil {
		t.Fatal("accepted a menu cycle")
	}
	chain := []model.SysMenu{{MenuID: 100}}
	for id := int64(101); id <= 165; id++ {
		chain = append(chain, model.SysMenu{MenuID: id, ParentID: id - 1})
	}
	if len(reachableCustomerMenus(chain)) != 64 {
		t.Fatal("catalog did not enforce its hierarchy depth bound")
	}
	if _, err := normalizeTenantMenus([]int64{165}, chain); err == nil {
		t.Fatal("accepted an excessively deep hierarchy")
	}
}

func TestTenantAdminActionNeedsBoundPlatformIdentity(t *testing.T) {
	platform := &model.SysUser{TenantRecord: model.TenantRecord{TenantID: 1}, UserID: 7, UserType: "02", UserName: "admin"}
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 10000, false), tenantctx.With(context.Background(), 99, true)} {
		if _, err := platformActor(ctx, platform, "127.0.0.1"); err == nil {
			t.Fatal("accepted unbound or foreign workspace")
		}
	}
	if _, err := platformActor(tenantctx.With(context.Background(), 1, true), platform, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	platform.UserType = "01"
	if _, err := platformActor(tenantctx.With(context.Background(), 1, true), platform, ""); err == nil {
		t.Fatal("tenant admin became platform admin")
	}
}

func TestOwnerPasswordResetValidation(t *testing.T) {
	body := TenantOwnerPasswordInput{Version: 1, OwnerUserID: 1, ConfirmTenantCode: "demo", NewPassword: "重置测试密码八字", ConfirmPassword: "重置测试密码八字", OperatorPassword: "platform-proof", Reason: "遗失密码"}
	if err := validateTenantOwnerPassword(body); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*TenantOwnerPasswordInput){
		func(b *TenantOwnerPasswordInput) { b.OperatorPassword = "" },
		func(b *TenantOwnerPasswordInput) { b.ConfirmPassword = "mismatch" },
		func(b *TenantOwnerPasswordInput) { b.NewPassword = "short" },
		func(b *TenantOwnerPasswordInput) { b.NewPassword = strings.Repeat("中", 21) },
		func(b *TenantOwnerPasswordInput) { b.Version = 0 },
		func(b *TenantOwnerPasswordInput) { b.OwnerUserID = 0 },
		func(b *TenantOwnerPasswordInput) { b.Reason = " " },
	} {
		copy := body
		change(&copy)
		if validateTenantOwnerPassword(copy) == nil {
			t.Fatal("accepted incomplete reset confirmation")
		}
	}
}
