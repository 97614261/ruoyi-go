package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func TestPODAdminAuthorizationRejectsBeforeRepository(t *testing.T) {
	ctx := context.Background()
	creator := &model.LoginUser{UserID: 42, User: &model.SysUser{UserID: 42}, Permissions: []string{model.PODStudioAccess}}
	for _, user := range []*model.LoginUser{nil, creator} {
		want := 403
		if user == nil {
			want = 401
		}
		_, err := PODAdminBootstrap(ctx, user)
		assertAdminCode(t, err, want)
		_, err = PODAdminOverview(ctx, user, "", "")
		assertAdminCode(t, err, want)
		_, _, err = PODAdminAuditList(ctx, user, model.PODAdminAuditFilter{}, page.Query{})
		assertAdminCode(t, err, want)
		_, err = PODAdminAuditGet(ctx, user, 1)
		assertAdminCode(t, err, want)
	}
	limited := &model.LoginUser{UserID: 43, User: &model.SysUser{UserID: 43}, Permissions: []string{model.PODAdminAccess}}
	if err := requirePODAdmin(limited, model.PODAdminAccess); err != nil {
		t.Fatal(err)
	}
	_, err := PODAdminOverview(ctx, limited, "", "")
	assertAdminCode(t, err, 403)
	root := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{model.AllPermission}}
	if err := requirePODAdmin(root, model.PODAdminAuditQuery); err != nil {
		t.Fatal(err)
	}
}

func assertAdminCode(t *testing.T, err error, want int) {
	t.Helper()
	business := errs.As(err)
	if business == nil || business.Code != want {
		t.Fatalf("expected business error %d", want)
	}
}

func TestPODAdminDateRange(t *testing.T) {
	now := time.Date(2026, 11, 13, 3, 0, 0, 0, time.UTC)
	b, e, err := PODAdminDateRange("", "", now)
	if err != nil || b.Format("2006-01-02") != "2026-11-07" || e.Format("2006-01-02") != "2026-11-14" || b.Location() != types.Location {
		t.Fatal("default dates must cover seven Shanghai business days")
	}
	for _, pair := range [][2]string{{"2026-10-13", "2026-11-13"}, {"2026-11-13", "2026-11-12"}, {"2026-11-13", "2026-11-14"}, {"2026-2-1", "2026-11-13"}, {"bad", ""}, {"", "2026-13-01"}} {
		_, _, err := PODAdminDateRange(pair[0], pair[1], now)
		assertAdminCode(t, err, 400)
	}
	_, _, err = PODAdminDateRange("2026-10-14", "2026-11-13", now)
	if err != nil {
		t.Fatal("31 days must be accepted")
	}
}

func TestPODAdminAuditDTOWhitelistsSnapshots(t *testing.T) {
	row := model.PODAdminAudit{AuditID: 9007199254740993, BeforeJSON: json.RawMessage(`{"password":"do-not-serialize","graph":{"prompt":"private"}}`), AfterJSON: json.RawMessage(`{"schemaVersion":"008","secret":"do-not-serialize","receipt":{"private":"content"}}`), Remark: "internal-only"}
	data, err := json.Marshal(podAdminAuditView(row, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "do-not-serialize", "graph", "secret", "receipt", "internal-only"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("audit DTO leaked fields outside allowlist")
		}
	}
	if !strings.Contains(string(data), `"auditId":"9007199254740993"`) {
		t.Fatal("ID lost precision")
	}
	data, _ = json.Marshal(podAdminAuditView(row, false))
	if strings.Contains(string(data), "before") || strings.Contains(string(data), "after") {
		t.Fatal("list must omit snapshots")
	}
}
