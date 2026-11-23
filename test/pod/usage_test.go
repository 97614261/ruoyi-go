package podtest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
)

func exercisePODUsage(t *testing.T, ctx context.Context) {
	var users []model.SysUser
	var spaces []int64
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupCreatorFixtures(t, cleanup, spaces)
		for _, table := range []string{"pod_provider_call", "pod_node_run", "pod_run", "pod_workspace_member", "pod_workspace"} {
			if len(spaces) > 0 {
				if err := repository.DB(cleanup).Table(table).Where("workspace_id IN ?", spaces).Delete(map[string]any{}).Error; err != nil {
					t.Error("usage fixture cleanup failed", table)
				}
			}
		}
		for _, u := range users {
			if err := repository.DB(cleanup).Delete(&model.SysUser{}, u.UserID).Error; err != nil {
				t.Error("usage user cleanup failed")
			}
		}
	})
	for i := 0; i < 2; i++ {
		u := model.SysUser{UserName: "usage_" + uuid.NewString()[:8], NickName: "Usage fixture", Status: "0", DelFlag: "0"}
		if err := repository.DB(ctx).Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		w, err := service.InitPODWorkspace(ctx, &u)
		if err != nil {
			t.Fatal(err)
		}
		spaces = append(spaces, w.WorkspaceID)
	}
	svc := service.PODRunService{Limits: model.PODExecutionLimits{Mode: "qwen"}, BudgetFen: 500}
	t.Run("empty_is_zero_and_never_null", func(t *testing.T) {
		v, err := svc.Usage(ctx, &users[0], page.Query{})
		if err != nil || v == nil {
			t.Fatal("usage failed", err)
		}
		if v.Rows == nil || len(v.Rows) != 0 || v.Summary != (model.PODUsageSummary{}) || v.BudgetScope != "instanceCumulative" || v.BudgetLimitMicros != 5000000 {
			t.Fatal("incorrect empty usage")
		}
	})
	now := time.Now().Truncate(time.Second)
	var rows []model.PODRun
	for i := 0; i < 104; i++ {
		rows = append(rows, model.PODRun{WorkspaceID: spaces[0], UserID: users[0].UserID, ProjectID: 1, ProjectName: "Own usage", ProjectRevision: 1, RequestID: uuid.NewString(), RequestHash: strings.Repeat("a", 64), ExecutionMode: "qwen", Mode: "workflow", State: model.PODRunSucceeded, Attempt: 2, GraphJSON: json.RawMessage(`{"schemaVersion":1,"nodes":[],"edges":[]}`), BaseOutputsJSON: json.RawMessage(`{}`), CreateTime: now, UpdateTime: now})
	}
	rows[0].State = model.PODRunFailed
	rows[1].State = model.PODRunAwaitingReconciliation
	rows[2].ExecutionMode = "simulation"
	rows[2].Spent = 999
	if err := repository.DB(ctx).Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	foreign := rows[0]
	foreign.RunID = 0
	foreign.WorkspaceID = spaces[1]
	foreign.UserID = users[1].UserID
	foreign.ProjectName = "PRIVATE_OTHER_PROJECT"
	if err := repository.DB(ctx).Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	addCall := func(run model.PODRun, charged, reserved, images int64, state string) {
		t.Helper()
		receipt, _ := json.Marshal(map[string]any{"state": "succeeded", "text": "PRIVATE_PROVIDER_TEXT", "imageUrls": []string{"https://private.invalid/secret"}, "usage": map[string]any{"imageCount": images}})
		call := model.PODProviderCall{SubmissionKey: uuid.NewString(), RunID: run.RunID, NodeID: "B", WorkspaceID: run.WorkspaceID, Model: "fixture", State: state, ChargedMicros: charged, ReservedMicros: reserved, ReceiptJSON: receipt, CreateTime: now, UpdateTime: now}
		if err := repository.DB(ctx).Create(&call).Error; err != nil {
			t.Fatal(err)
		}
	}
	addCall(rows[0], 100001, 0, 1, "confirmed")
	addCall(rows[0], 200000, 0, 2, "confirmed")
	addCall(rows[1], 0, 100000, 4, "submitted")
	addCall(rows[1], 50, 0, 0, "confirmed")
	addCall(foreign, 900000000, 700000, 80, "confirmed")
	node := model.PODNodeRun{RunID: rows[0].RunID, NodeID: "B", WorkspaceID: spaces[0], UserID: users[0].UserID, NodeType: "imageGenerate", Provider: "qwen", State: "succeeded", Attempt: 2, InputJSON: json.RawMessage(`{}`), OutputJSON: json.RawMessage(`{"usage":{"estimatedCostMicros":"1","imageCount":99}}`), CreateTime: now, UpdateTime: now}
	if err := repository.DB(ctx).Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	t.Run("all_attempts_unknown_and_generated_not_local_or_current_output", func(t *testing.T) {
		v, err := svc.Usage(ctx, &users[0], page.Query{PageNum: 104, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		want := model.PODUsageStats{ChargedMicros: 300051, ReservedMicros: 100000, ProviderCalls: 4, PendingCalls: 1, GeneratedImages: 3}
		if v.Total != 104 || v.Summary.RunCount != 104 || v.Summary.FailedRuns != 1 || v.Summary.PendingRuns != 1 || v.Summary.PODUsageStats != want {
			t.Fatalf("incorrect totals: %+v", v.Summary)
		}
		if len(v.Rows) != 1 || v.Rows[0].RunID != rows[0].RunID || v.Rows[0].ChargedMicros != 300001 || v.Rows[0].GeneratedImages != 3 {
			t.Fatal("all attempts or stable pagination lost")
		}
		encoded, _ := json.Marshal(v)
		for _, forbidden := range []string{"PRIVATE_", "receipt", "submissionKey", "requestId", "imageUrls", "secret", "workspaceId", "remainingMicros", "globalSpent"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatal("private field exposed", forbidden)
			}
		}
		if !strings.Contains(string(encoded), `"chargedMicros":"300001"`) {
			t.Fatal("money must be a decimal string")
		}
	})
	t.Run("page_cap_stable_ties_and_out_of_range_keep_totals", func(t *testing.T) {
		v, err := svc.Usage(ctx, &users[0], page.Query{PageNum: -1, PageSize: 999})
		if err != nil || v == nil || len(v.Rows) != 100 || v.Rows[0].RunID != rows[103].RunID {
			t.Fatal("page cap/order failed", err)
		}
		v, err = svc.Usage(ctx, &users[0], page.Query{PageNum: 2, PageSize: 100})
		if err != nil || len(v.Rows) != 4 || v.Rows[0].RunID != rows[3].RunID {
			t.Fatal("second page overlaps", err)
		}
		v, err = svc.Usage(ctx, &users[0], page.Query{PageNum: 200, PageSize: 100})
		if err != nil || v.Rows == nil || len(v.Rows) != 0 || v.Summary.ChargedMicros != 300051 {
			t.Fatal("out-of-range page changed totals", err)
		}
	})
	t.Run("other_owner_revoked_member_and_disabled_workspace", func(t *testing.T) {
		v, err := svc.Usage(ctx, &users[1], page.Query{})
		if err != nil || v.Total != 1 || v.Summary.ChargedMicros != 900000000 {
			t.Fatal("other owner's totals incorrect", err)
		}
		for _, table := range []string{"pod_workspace_member", "pod_workspace"} {
			if err := repository.DB(ctx).Table(table).Where("workspace_id=?", spaces[0]).Update("status", "1").Error; err != nil {
				t.Fatal(err)
			}
			_, denied := svc.Usage(ctx, &users[0], page.Query{})
			if err := repository.DB(ctx).Table(table).Where("workspace_id=?", spaces[0]).Update("status", "0").Error; err != nil {
				t.Fatal(err)
			}
			if denied == nil {
				t.Fatal("revoked usage access accepted", table)
			}
		}
	})
}
