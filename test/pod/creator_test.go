package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

// Called only by suites which already remove their exact, newly-created spaces.
func cleanupCreatorFixtures(t *testing.T, ctx context.Context, spaces []int64) {
	t.Helper()
	if len(spaces) == 0 {
		return
	}
	var users []int64
	if err := repository.DB(ctx).Table("pod_creator_policy").Where("workspace_id IN ?", spaces).Pluck("user_id", &users).Error; err != nil {
		t.Error(err)
		return
	}
	if len(users) == 0 {
		return
	}
	for _, table := range []string{"pod_user_model_grant", "pod_points_event", "pod_points_account", "pod_user_daily_usage", "pod_storage_reservation", "pod_storage_account", "pod_creator_policy"} {
		if err := repository.DB(ctx).Table(table).Where("user_id IN ?", users).Delete(map[string]any{}).Error; err != nil {
			t.Error(table, err)
		}
	}
}

func exercisePODCreators(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string) {
	admin := tokens["admin"]
	transport := http.DefaultTransport
	http.DefaultTransport = podQwenTransport(func(*http.Request) (*http.Response, error) {
		t.Error("creator administration must not call a provider")
		return nil, errors.New("network forbidden")
	})
	defer func() { http.DefaultTransport = transport }()
	check := func(p map[string]json.RawMessage, want int) {
		t.Helper()
		if string(p["code"]) != strconv.Itoa(want) {
			t.Fatalf("creator code %s want %d; %s", p["code"], want, p["msg"])
		}
	}
	decode := func(p map[string]json.RawMessage, v any) {
		t.Helper()
		if json.Unmarshal(p["data"], v) != nil {
			t.Fatal("creator DTO invalid")
		}
	}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	write := func(method, path string, r model.PODCreatorWrite, want int) model.PODCatalogResult {
		t.Helper()
		if r.RequestID == "" {
			r.RequestID = uuid.NewString()
		}
		if r.Reason == "" {
			r.Reason = "本地创作者专项验收"
		}
		p := call(method, "/pod-admin/"+path, admin, r)
		check(p, want)
		var out model.PODCatalogResult
		if want == 200 {
			decode(p, &out)
		}
		return out
	}
	limits := service.PODExecutionLimits(cfg.POD.Execution)
	policy := model.PODCreatorPolicyInput{Status: "active", MaxConcurrentNodes: 1, MaxPendingRuns: min(10, limits.MaxUserPending), MaxDailySubmissions: 1000, MaxStorageBytes: "1073741824"}
	password := "Cr-" + uuid.NewString()[:15]
	request := model.PODCreatorWrite{RequestID: uuid.NewString(), Reason: "本地新用户验收", UserName: "creator_" + uuid.NewString()[:8], NickName: "权益验收", InitialPassword: password, Policy: &policy}
	for _, path := range []string{"creators", "creator-limits", "creator-model-options"} {
		check(call("GET", "/pod-admin/"+path, "", nil), 401)
		check(call("GET", "/pod-admin/"+path, tokens["pod_creator"], nil), 403)
	}
	check(call("POST", "/pod-admin/creators", tokens["pod_creator"], request), 403)
	created := write("POST", "creators", request, 200)
	uid := id(created.ResourceID)
	base := "creators/" + uid
	get := func() model.PODCreatorDetail {
		t.Helper()
		p := call("GET", "/pod-admin/"+base, admin, nil)
		check(p, 200)
		var d model.PODCreatorDetail
		decode(p, &d)
		return d
	}
	account := func() model.PODPointsAccount {
		t.Helper()
		p := call("GET", "/pod-admin/"+base+"/points", admin, nil)
		check(p, 200)
		var a model.PODPointsAccount
		decode(p, &a)
		return a
	}
	if replay := write("POST", "creators", request, 200); replay != created {
		t.Fatal("creator replay duplicated account")
	}
	request.InitialPassword += "x"
	write("POST", "creators", request, 409)
	request.InitialPassword = password
	var user model.SysUser
	if err := repository.DB(ctx).Where("user_id=?", created.ResourceID).Take(&user).Error; err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		t.Fatal("creator password hash mismatch")
	}
	t.Cleanup(func() {
		p := policy
		p.Status = "suspended"
		d, err := repository.PODCreator(ctx, created.ResourceID)
		if err != nil {
			t.Error(err)
			return
		}
		_, err = service.PODCreatorWrite(ctx, &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{"*:*:*"}}, "creator.policy", created.ResourceID, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(d.Creator.Revision), Reason: "本轮专用测试账号停用保留", Policy: &p}, limits)
		if err != nil {
			t.Error("suspend creator fixture", err)
		}
	})
	d := get()
	a := account()
	if d.ModelIDs == nil || len(d.ModelIDs) != 0 || a.Available != 0 || a.Held != 0 || a.Spent != 0 {
		t.Fatal("new account granted privileges or balance")
	}
	captchaID := "creator-" + uuid.NewString()
	if err := redisx.C().Set(ctx, redisx.CaptchaKey(captchaID), "2468", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	login := call("POST", "/login", "", model.LoginBody{Username: request.UserName, Password: password, UUID: captchaID, Code: "2468"})
	check(login, 200)
	var token string
	json.Unmarshal(login["token"], &token)
	if token == "" {
		t.Fatal("created user cannot log in")
	}
	defer call("POST", "/logout", token, nil)
	check(call("GET", "/pod-admin/creators", token, nil), 403)
	for _, path := range []string{"/pod/eligibility", "/pod/points"} {
		p := call("GET", path+"?userId=1&workspaceId=1", token, nil)
		check(p, 200)
		if bytes.Contains(p["data"], []byte(password)) {
			t.Fatal("self response leaked password")
		}
	}

	t.Run("point_idempotency_revision_and_concurrency", func(t *testing.T) {
		r := model.PODCreatorWrite{RequestID: uuid.NewString(), Reason: "grant local points", AccountRevision: "1", Kind: "grant", Amount: "100"}
		got := write("POST", base+"/point-adjustments", r, 200)
		write("POST", base+"/point-adjustments", r, 200)
		r.Amount = "101"
		write("POST", base+"/point-adjustments", r, 409)
		if a := account(); a.Available != 100 || a.GrantedTotal != 100 || a.Revision != got.Revision {
			t.Fatal("double grant")
		}
		a := account()
		r = model.PODCreatorWrite{Reason: "concurrent local deductions", AccountRevision: id(a.Revision), Kind: "deduct", Amount: "80"}
		actor := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{"*:*:*"}}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				copy := r
				copy.RequestID = uuid.NewString()
				_, err := service.PODCreatorWrite(ctx, actor, "points.adjust", created.ResourceID, copy, limits)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if e := errs.As(err); e != nil && e.Code == 409 {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal("concurrent point mutation lost revision", success, conflict)
		}
		a = account()
		if a.Available != 20 || a.GrantedTotal-a.DeductedTotal != a.Available+a.Held+a.Spent {
			t.Fatal("point invariant failed")
		}
		write("POST", base+"/point-adjustments", model.PODCreatorWrite{AccountRevision: id(a.Revision), Kind: "deduct", Amount: "21"}, 409)
		p := call("GET", "/pod-admin/"+base+"/point-events?pageNum=99&pageSize=1", admin, nil)
		check(p, 200)
		if string(p["total"]) != "2" || string(p["rows"]) != "[]" {
			t.Fatal("point pagination loses totals")
		}
	})
	t.Run("model_scope_and_audit", func(t *testing.T) {
		p := call("POST", "/pod-admin/providers", admin, model.PODCatalogWrite{RequestID: uuid.NewString(), Reason: "creator model fixture", Code: "creator-" + uuid.NewString()[:8], Name: "权益测试供应商", AdapterKind: "qwen-v1", Region: "cn-beijing", Endpoint: "https://dashscope.aliyuncs.com"})
		check(p, 200)
		var provider model.PODCatalogResult
		decode(p, &provider)
		p = call("POST", "/pod-admin/models", admin, model.PODCatalogWrite{RequestID: uuid.NewString(), Reason: "creator model fixture", Code: "creator-" + uuid.NewString()[:8], Name: "权益测试文字模型", ProviderID: id(provider.ResourceID), Kind: "text"})
		check(p, 200)
		var m model.PODCatalogResult
		decode(p, &m)
		mids := []string{id(m.ResourceID)}
		d := get()
		write("PUT", base+"/models", model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &mids}, 200)
		d = get()
		if len(d.ModelIDs) != 1 || d.ModelIDs[0] != mids[0] {
			t.Fatal("model grant missing")
		}
		dup := []string{mids[0], mids[0]}
		write("PUT", base+"/models", model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &dup}, 400)
		missing := []string{"9223372036854775807"}
		write("PUT", base+"/models", model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &missing}, 400)
		empty := []string{}
		write("PUT", base+"/models", model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &empty}, 200)
		d = get()
		if len(d.ModelIDs) != 0 {
			t.Fatal("empty grant did not revoke")
		}
		var audit model.PODAdminAudit
		if err := repository.DB(ctx).Where("owner_user_id=? AND action_type='creator.models'", created.ResourceID).Order("audit_id DESC").Take(&audit).Error; err != nil {
			t.Fatal(err)
		}
		var snap model.PODAdminAuditSnapshot
		if json.Unmarshal(audit.BeforeJSON, &snap) != nil || snap.Creator == nil || len(snap.Creator.ModelIDs) != 1 {
			t.Fatal("model before snapshot missing")
		}
	})
	t.Run("expiry_suspension_and_daily_submission", func(t *testing.T) {
		p, err := service.CreatePODProject(ctx, &user, model.PODProjectCreate{Name: "资格专项画布"})
		if err != nil {
			t.Fatal(err)
		}
		graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"fixture"}`)}, {ID: "B", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{}`)}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
		p, err = service.EditPODProject(ctx, &user, p.ProjectID, model.PODProjectUpdate{Revision: id(p.Revision), Graph: &graph}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		d := get()
		small := policy
		small.MaxDailySubmissions = 1
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &small}, 200)
		r := model.PODRunCreate{ExecutionMode: "simulation", RequestID: uuid.NewString(), ProjectID: id(p.ProjectID), Revision: id(p.Revision), Mode: "workflow", TestScenario: "success"}
		svc := service.PODRunService{Limits: limits}
		run, err := svc.Create(ctx, &user, r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Create(ctx, &user, r); err != nil {
			t.Fatal("idempotent request charged daily twice", err)
		}
		if _, err = svc.Change(ctx, &user, run.RunID, ""); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if _, err = service.ExecutePODStep(ctx, run.RunID, limits, executor.Simulation{}); err != nil {
				t.Fatal(err)
			}
		}
		r.RequestID = uuid.NewString()
		if _, err = svc.Create(ctx, &user, r); err == nil {
			t.Fatal("cancellation returned daily quota")
		}
		if d = get(); d.DailySubmissions != 1 {
			t.Fatal("wrong daily count")
		}
		until := "2000-01-01 00:00:00"
		expired := policy
		expired.ValidUntil = &until
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &expired}, 200)
		check(call("GET", "/pod/projects", token, nil), 200)
		check(call("POST", "/pod/projects", token, model.PODProjectCreate{Name: "denied"}), 403)
		if _, err = service.CreatePODProject(ctx, &user, model.PODProjectCreate{Name: "txn denied"}); err == nil {
			t.Fatal("transaction ignored expiry")
		}
		check(call("POST", "/pod/workspace", token, map[string]any{}), 403)
		d = get()
		suspended := policy
		suspended.Status = "suspended"
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &suspended}, 200)
		check(call("GET", "/pod/projects", token, nil), 403)
		check(call("GET", "/pod/eligibility", token, nil), 200)
		check(call("GET", "/pod/points", token, nil), 200)
		check(call("POST", "/pod/workspace", token, map[string]any{}), 403)
		d = get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &policy}, 200)
	})
	t.Run("worker_checks_policy_but_keeps_submitted_reconciliation", func(t *testing.T) {
		d := get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &policy}, 200)
		p, err := service.CreatePODProject(ctx, &user, model.PODProjectCreate{Name: "资格 Worker 专项"})
		if err != nil {
			t.Fatal(err)
		}
		graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"fixture"}`)}, {ID: "B", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{}`)}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
		p, err = service.EditPODProject(ctx, &user, p.ProjectID, model.PODProjectUpdate{Revision: id(p.Revision), Graph: &graph}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		q := limits
		q.Mode = "qwen"
		svc := service.PODRunService{Limits: q}
		run, err := svc.Create(ctx, &user, model.PODRunCreate{ExecutionMode: "qwen", RequestID: uuid.NewString(), ProjectID: id(p.ProjectID), Revision: id(p.Revision), Mode: "workflow"})
		if err != nil {
			t.Fatal(err)
		}
		a, err := repository.ClaimPODNode(ctx, run.RunID, q, service.PODNodeLease)
		if err != nil || a == nil {
			t.Fatal("input claim unavailable", err)
		}
		if err = repository.FinishPODNode(ctx, a, "succeeded", model.PODRunOutput{Text: "fixture"}, "", ""); err != nil {
			t.Fatal(err)
		}
		b, err := repository.ClaimPODNode(ctx, run.RunID, q, service.PODNodeLease)
		if err != nil || b == nil {
			t.Fatal("provider claim unavailable", err)
		}
		suspended := policy
		suspended.Status = "suspended"
		d = get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &suspended}, 200)
		if _, fresh, err := repository.BeginPODProviderCall(ctx, b, 7, 1000000000000); err == nil || fresh {
			t.Fatal("stale claim bypassed suspension")
		}
		d = get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &policy}, 200)
		if _, fresh, err := repository.BeginPODProviderCall(ctx, b, 7, 1000000000000); err != nil || !fresh {
			t.Fatal("fixture intent unavailable", err)
		}
		defer repository.DB(ctx).Where("submission_key=? AND run_id=?", b.Node.SubmissionKey, run.RunID).Delete(&model.PODProviderCall{})
		if err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id=?", run.RunID, b.Node.NodeID).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		d = get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &suspended}, 200)
		reconciled, err := repository.ClaimPODNode(ctx, run.RunID, q, service.PODNodeLease)
		if err != nil || reconciled == nil || !reconciled.Reconcile || reconciled.Node.SubmissionKey != b.Node.SubmissionKey {
			t.Fatal("suspension lost submitted reconciliation", err)
		}
		if err = repository.FinishPODNode(ctx, reconciled, "failed", model.PODRunOutput{}, "fixtureOnly", "本地无网络夹具结束"); err != nil {
			t.Fatal(err)
		}
		d = get()
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &policy}, 200)
	})
	t.Run("concurrent_storage_and_unknown_held", func(t *testing.T) {
		d := get()
		small := policy
		small.MaxStorageBytes = "150"
		write("PUT", base+"/policy", model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &small}, 200)
		keys := []string{uuid.NewString(), uuid.NewString()}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, key := range keys {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				results <- repository.PODReserveStorage(ctx, created.ResourceID, k, 100, false)
			}(key)
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if e := errs.As(err); e == nil || e.Code != 409 {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatal("concurrent storage exceeded cap")
		}
		d = get()
		if d.Storage.HeldBytes != 100 {
			t.Fatal("storage reservation lost")
		}
		var held model.PODStorageReservation
		if err := repository.DB(ctx).Where("user_id=? AND state='held'", created.ResourceID).Take(&held).Error; err != nil {
			t.Fatal(err)
		}
		if err := repository.DB(ctx).Table("pod_storage_reservation").Where("reservation_id=?", held.ReservationID).Update("lease_until", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
		if err := repository.PODReserveStorage(ctx, created.ResourceID, held.OperationKey, 100, false); err == nil {
			t.Fatal("expired uncertain reservation reused")
		}
		d = get()
		if d.Storage.HeldBytes != 100 {
			t.Fatal("time released uncertain storage")
		}
		// This test never wrote files, so exact release is known to be safe.
		if err := repository.PODReleaseStorage(ctx, created.ResourceID, held.OperationKey); err != nil {
			t.Fatal(err)
		}
		if err := repository.PODReleaseStorage(ctx, created.ResourceID, held.OperationKey); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("transaction_rechecks_admin_and_no_password_in_audit", func(t *testing.T) {
		actor := model.SysUser{UserName: "cractor_" + uuid.NewString()[:8], NickName: "撤权验收", Status: "0", DelFlag: "0", CreateTime: types.Now()}
		if err := repository.DB(ctx).Create(&actor).Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Delete(&actor)
		d := get()
		_, err := service.PODCreatorWrite(ctx, &model.LoginUser{UserID: actor.UserID, User: &actor, Permissions: []string{model.PODAdminAccess, "podAdmin:creator:edit"}}, "creator.policy", created.ResourceID, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(d.Creator.Revision), Reason: "stale permissions", Policy: &policy}, limits)
		if e := errs.As(err); e == nil || e.Code != 403 {
			t.Fatal("stale admin permissions accepted", err)
		}
		var audits []model.PODAdminAudit
		if err := repository.DB(ctx).Where("owner_user_id=?", created.ResourceID).Find(&audits).Error; err != nil {
			t.Fatal(err)
		}
		for _, a := range audits {
			b, _ := json.Marshal(a)
			if bytes.Contains(b, []byte(password)) || bytes.Contains(b, []byte(user.Password)) || strings.Contains(string(b), "initialPassword") {
				t.Fatal("password leaked to audit")
			}
		}
	})
}
