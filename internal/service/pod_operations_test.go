package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"testing"
	"time"
)

func TestPODOperationsRejectInvalidScopeBeforeRepository(t *testing.T) {
	ctx := context.Background()
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{"*:*:*"}}
	for _, f := range []model.PODTaskFilter{{}, {Begin: time.Now(), End: time.Now().Add(32 * 24 * time.Hour)}, {Begin: time.Now(), End: time.Now().Add(time.Hour), UserID: -1}} {
		_, _, err := PODTaskList(ctx, admin, f, page.Query{}, false)
		assertAdminCode(t, err, 400)
		_, _, err = PODAdminUsage(ctx, admin, f, page.Query{})
		assertAdminCode(t, err, 400)
	}
	svc := NewPODOfficialService(t.TempDir(), model.PODGraphLimits{}, model.PODAssetLimits{})
	if _, _, err := svc.List(ctx, nil, false, "", page.Query{}); err == nil {
		t.Fatal("anonymous official list accepted")
	}
	if _, err := svc.Get(ctx, nil, 1, 0, false); err == nil {
		t.Fatal("anonymous official detail accepted")
	}
	if _, err := svc.Use(ctx, nil, 1, model.PODTemplateUse{}); err == nil {
		t.Fatal("anonymous official use accepted")
	}
	if _, err := svc.UseAction(ctx, nil, "unknown"); err == nil {
		t.Fatal("anonymous copy lookup accepted")
	}
	readonly := &model.LoginUser{UserID: 42, User: &model.SysUser{UserID: 42}, Permissions: []string{model.PODAdminAccess, "podAdmin:task:list"}}
	_, err := PODOperation(ctx, readonly, "task.cancel", 1, model.PODOperationWrite{})
	assertAdminCode(t, err, 403)
	_, err = PODPrivateRun(ctx, readonly, 1)
	assertAdminCode(t, err, 403)
	_, err = svc.Write(ctx, readonly, 0, "official.create", model.PODOfficialWrite{})
	assertAdminCode(t, err, 403)
}
