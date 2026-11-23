package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"image"
	"image/png"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/storage"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func exercisePODRuns(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage) {
	if cfg.POD.Execution.Mode != "simulation" {
		t.Skip("execution simulation disabled")
	}
	limits := service.PODExecutionLimits(cfg.POD.Execution)
	assert := func(ok bool, message string) {
		t.Helper()
		if !ok {
			t.Fatal(message)
		}
	}
	code := func(p map[string]json.RawMessage) int { var n int; _ = json.Unmarshal(p["code"], &n); return n }
	expect := func(p map[string]json.RawMessage, want int) {
		t.Helper()
		if got := code(p); got != want {
			var msg string
			_ = json.Unmarshal(p["msg"], &msg)
			t.Fatalf("business code %d want %d: %s", got, want, msg)
		}
	}
	users := []model.SysUser{}
	tokens := []string{}
	workspaces := []int64{}
	projectIDs := []int64{}
	assetIDs := []int64{}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupCreatorFixtures(t, cleanup, workspaces)
		for _, token := range tokens {
			call("POST", "/logout", token, nil)
		}
		if len(workspaces) > 0 {
			for _, table := range []string{"pod_run_asset_ref", "pod_node_run", "pod_credit_event", "pod_retry_request"} {
				if err := repository.DB(cleanup).Table(table).Where("workspace_id IN ?", workspaces).Delete(map[string]any{}).Error; err != nil {
					t.Error("run fixture cleanup", table, err)
				}
			}
			if err := repository.DB(cleanup).Exec("DELETE o FROM pod_outbox o JOIN pod_run r ON r.run_id=o.run_id WHERE r.workspace_id IN ?", workspaces).Error; err != nil {
				t.Error(err)
			}
			for _, table := range []string{"pod_run", "pod_credit_account", "pod_project_asset_ref", "pod_project", "pod_workspace_member", "pod_workspace"} {
				if err := repository.DB(cleanup).Table(table).Where("workspace_id IN ?", workspaces).Delete(map[string]any{}).Error; err != nil {
					t.Error("run fixture cleanup", table, err)
				}
			}
			if len(assetIDs) > 0 {
				var files []model.PODAssetFile
				if err := repository.DB(cleanup).Table("pod_asset_file").Where("asset_id IN ?", assetIDs).Find(&files).Error; err != nil {
					t.Error(err)
				} else {
					for _, f := range files {
						if err := (storage.Local{Path: cfg.POD.StorageRoot}).Remove(cleanup, f.StorageKey); err != nil {
							t.Error(err)
						}
					}
				}
				_ = repository.DB(cleanup).Table("pod_asset_file").Where("asset_id IN ?", assetIDs).Delete(&model.PODAssetFile{}).Error
				_ = repository.DB(cleanup).Table("pod_asset").Where("asset_id IN ?", assetIDs).Delete(&model.PODAsset{}).Error
			}
		}
		for _, u := range users {
			_ = repository.DB(cleanup).Exec("DELETE FROM sys_user_role WHERE user_id=?", u.UserID).Error
			_ = repository.DB(cleanup).Table("sys_user").Where("user_id=?", u.UserID).Delete(&model.SysUser{}).Error
		}
	})
	for i := 0; i < 3; i++ {
		name := "podrun_" + uuid.NewString()[:8]
		secret := uuid.NewString()[:16]
		hash, _ := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
		u := model.SysUser{UserName: name, NickName: "执行专项测试", Password: string(hash), Status: "0", DelFlag: "0"}
		if err := repository.DB(ctx).Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		if err := repository.DB(ctx).Exec("INSERT INTO sys_user_role(user_id,role_id) SELECT ?,ur.role_id FROM sys_user_role ur JOIN sys_user u ON u.user_id=ur.user_id WHERE u.user_name='pod_creator'", u.UserID).Error; err != nil {
			t.Fatal(err)
		}
		w, err := service.InitPODWorkspace(ctx, &u)
		if err != nil {
			t.Fatal(err)
		}
		workspaces = append(workspaces, w.WorkspaceID)
		captchaID := "pod-run-" + uuid.NewString()
		if err := redisx.C().Set(ctx, redisx.CaptchaKey(captchaID), "2468", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		login := call("POST", "/login", "", model.LoginBody{Username: name, Password: secret, Code: "2468", UUID: captchaID})
		expect(login, 200)
		var token string
		_ = json.Unmarshal(login["token"], &token)
		tokens = append(tokens, token)
	}
	graph := func() model.PODGraph {
		return model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
			{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"immutable input"}`)},
			{ID: "B", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{"instruction":"test instruction"}`)},
			{ID: "C", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{}`)},
			{ID: "D", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"independent"}`)},
		}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}, {ID: "BC", Source: "B", SourcePort: "text", Target: "C", TargetPort: "text"}}}
	}
	newProject := func(index int, g model.PODGraph) model.PODProjectView {
		t.Helper()
		p := call("POST", "/pod/projects", tokens[index], map[string]any{"name": "阶段4隔离测试"})
		expect(p, 200)
		var v model.PODProjectView
		_ = json.Unmarshal(p["data"], &v)
		projectIDs = append(projectIDs, v.ProjectID)
		p = call("PUT", fmt.Sprintf("/pod/projects/%d", v.ProjectID), tokens[index], map[string]any{"revision": "1", "graph": g})
		expect(p, 200)
		_ = json.Unmarshal(p["data"], &v)
		return v
	}
	p := newProject(0, graph())
	q := newProject(1, graph())
	request := func(p model.PODProjectView, scenario string) model.PODRunCreate {
		return model.PODRunCreate{ProjectID: strconv.FormatInt(p.ProjectID, 10), Revision: strconv.FormatInt(p.Revision, 10), Mode: "workflow", RequestID: uuid.NewString(), TestScenario: scenario}
	}
	view := func(payload map[string]json.RawMessage) model.PODRunView {
		t.Helper()
		expect(payload, 200)
		var r model.PODRunView
		if err := json.Unmarshal(payload["data"], &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	create := func(index int, p model.PODProjectView, scenario string) model.PODRunView {
		return view(call("POST", "/pod/runs", tokens[index], request(p, scenario)))
	}
	get := func(index int, id int64) model.PODRunView {
		return view(call("GET", fmt.Sprintf("/pod/runs/%d", id), tokens[index], nil))
	}
	finish := func(index int, id int64) model.PODRunView {
		t.Helper()
		for tries := 0; tries < 30; tries++ {
			r := get(index, id)
			if workflow.RunTerminal(r.State) {
				return r
			}
			_, err := service.ExecutePODStep(ctx, id, limits, executor.Simulation{})
			if err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("run did not reach terminal state")
		return model.PODRunView{}
	}
	var successful model.PODRunView
	t.Run("atomic_idempotent_submit_and_payload_conflict", func(t *testing.T) {
		body := request(p, "success")
		ch := make(chan map[string]json.RawMessage, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); ch <- call("POST", "/pod/runs", tokens[0], body) }()
		}
		wg.Wait()
		a, b := view(<-ch), view(<-ch)
		assert(a.RunID == b.RunID, "duplicate requests created separate runs")
		body.TestScenario = "fail"
		expect(call("POST", "/pod/runs", tokens[0], body), 409)
		successful = finish(0, a.RunID)
		assert(successful.State == model.PODRunSucceeded && successful.Spent == 2 && successful.Reserved == 0, "success settlement incorrect")
	})
	t.Run("duplicate_delivery_has_no_second_result_or_debit", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			more, err := service.ExecutePODStep(ctx, successful.RunID, limits, executor.Simulation{})
			assert(!more && err == nil, "terminal task executed again")
		}
		var count int64
		err := repository.DB(ctx).Table("pod_credit_event").Where("run_id=? AND kind='settle'", successful.RunID).Count(&count).Error
		assert(err == nil && count == 1, "settlement was duplicated")
	})
	t.Run("foreign_run_history_credits_and_mutations_rejected", func(t *testing.T) {
		for _, path := range []string{"", "/cancel", "/retry"} {
			method := "GET"
			var body any
			if path != "" {
				method = "POST"
				body = map[string]any{}
				if path == "/retry" {
					body = map[string]any{"requestId": uuid.NewString()}
				}
			}
			expect(call(method, fmt.Sprintf("/pod/runs/%d%s", successful.RunID, path), tokens[1], body), 404)
		}
		expect(call("GET", "/pod/runs", "", nil), 401)
		listed := call("GET", fmt.Sprintf("/pod/runs?projectId=%d", p.ProjectID), tokens[1], nil)
		expect(listed, 200)
		var n int
		_ = json.Unmarshal(listed["total"], &n)
		assert(n == 0, "foreign history leaked")
	})
	t.Run("failure_blocks_descendants_independent_branch_runs_and_retry_reuses_success", func(t *testing.T) {
		r := create(0, p, "failOnce")
		r = finish(0, r.RunID)
		assert(r.State == model.PODRunFailed && r.Spent == 0, "failure charge incorrect")
		states := map[string]string{}
		for _, n := range r.Nodes {
			states[n.NodeID] = n.State
		}
		assert(states["B"] == "failed" && states["C"] == "blocked" && states["D"] == "succeeded", "dependency failure propagation incorrect")
		key := uuid.NewString()
		path := fmt.Sprintf("/pod/runs/%d/retry", r.RunID)
		r = view(call("POST", path, tokens[0], model.PODRunRetry{RequestID: key}))
		again := view(call("POST", path, tokens[0], model.PODRunRetry{RequestID: key}))
		assert(again.Attempt == 2 && again.RunID == r.RunID, "retry request not idempotent")
		r = finish(0, r.RunID)
		assert(r.State == model.PODRunSucceeded && r.Spent == 2, "failure recovery failed")
		for _, n := range r.Nodes {
			if n.NodeID == "A" || n.NodeID == "D" {
				assert(n.Attempt == 1, "successful upstream reran")
			}
		}
	})
	t.Run("cancel_queued_releases_reservation_once", func(t *testing.T) {
		r := create(0, p, "success")
		path := fmt.Sprintf("/pod/runs/%d/cancel", r.RunID)
		expect(call("POST", path, tokens[0], map[string]any{}), 200)
		r = finish(0, r.RunID)
		assert(r.State == model.PODRunCancelled && r.Spent == 0 && r.Reserved == 0, "queued cancellation charged")
		expect(call("POST", path, tokens[0], map[string]any{}), 200)
	})
	t.Run("uncertain_holds_credits_and_queries_original_key", func(t *testing.T) {
		r := create(0, p, "uncertain")
		for i := 0; i < 2; i++ {
			if _, err := service.ExecutePODStep(ctx, r.RunID, limits, executor.Simulation{}); err != nil {
				t.Fatal(err)
			}
		}
		r = get(0, r.RunID)
		assert(r.State == model.PODRunAwaitingReconciliation && r.Reserved == 2, "uncertain call released credits")
		expect(call("POST", fmt.Sprintf("/pod/runs/%d/retry", r.RunID), tokens[0], model.PODRunRetry{RequestID: uuid.NewString()}), 409)
		r = finish(0, r.RunID)
		assert(r.State == model.PODRunSucceeded && r.Spent == 2, "query recovery failed")
	})
	t.Run("lease_expiry_reconciles_and_rejects_stale_writer", func(t *testing.T) {
		r := create(0, p, "success")
		first, err := repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		if err != nil || first == nil {
			t.Fatal("claim failed", err)
		}
		second, err := repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		assert(err == nil && second == nil, "same user acquired second concurrent lease")
		if err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id=?", r.RunID, first.Node.NodeID).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		assert(errors.Is(repository.HeartbeatPODNode(ctx, first, time.Second), repository.ErrPODLease), "expired heartbeat revived lease")
		second, err = repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		if err != nil || second == nil || second.Node.NodeID != "D" || second.Reconcile {
			t.Fatal("independent ready branch was blocked by expired call", err)
		}
		independent := executor.Simulation{}.Submit(ctx, executor.Request{Node: second.Definition, Input: second.Input})
		if err := repository.FinishPODNode(ctx, second, independent.State, independent.Output, independent.Code, independent.Message); err != nil {
			t.Fatal(err)
		}
		second, err = repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		assert(err == nil && second != nil && second.Reconcile && second.Node.SubmissionKey == first.Node.SubmissionKey, "expired call was resubmitted")
		assert(errors.Is(repository.FinishPODNode(ctx, first, "succeeded", model.PODRunOutput{TestOnly: true}, "", ""), repository.ErrPODLease), "stale completion accepted")
		result := executor.Simulation{}.Query(ctx, executor.Request{Key: second.Node.SubmissionKey, Scenario: second.Run.TestScenario, Attempt: second.Node.Attempt, Node: second.Definition, Input: second.Input})
		if err := repository.FinishPODNode(ctx, second, result.State, result.Output, result.Code, result.Message); err != nil {
			t.Fatal(err)
		}
		assert(finish(0, r.RunID).State == model.PODRunSucceeded, "lease recovery did not finish")
	})
	t.Run("single_node_requires_and_reuses_same_revision_upstream", func(t *testing.T) {
		body := request(p, "success")
		body.Mode = "node"
		body.NodeID = "B"
		expect(call("POST", "/pod/runs", tokens[0], body), 400)
		body.BaseRunID = strconv.FormatInt(successful.RunID, 10)
		r := view(call("POST", "/pod/runs", tokens[0], body))
		r = finish(0, r.RunID)
		assert(len(r.Nodes) == 1 && r.Nodes[0].NodeID == "B" && r.Spent == 1, "single-node ran ancestors")
		body.RequestID = uuid.NewString()
		body.BaseRunID = strconv.FormatInt(successful.RunID, 10)
		body.ProjectID = strconv.FormatInt(q.ProjectID, 10)
		expect(call("POST", "/pod/runs", tokens[1], body), 404)
	})
	t.Run("global_user_provider_and_pending_caps", func(t *testing.T) {
		a, b := create(0, p, "success"), create(1, q, "success")
		one := limits
		one.GlobalConcurrency = 1
		c, err := repository.ClaimPODNode(ctx, a.RunID, one, time.Second)
		assert(err == nil && c != nil, "first global slot missing")
		other, err := repository.ClaimPODNode(ctx, b.RunID, one, time.Second)
		assert(err == nil && other == nil, "global cap bypassed")
		out := executor.Simulation{}.Query(ctx, executor.Request{Node: c.Definition, Input: c.Input})
		if err := repository.FinishPODNode(ctx, c, out.State, out.Output, "", ""); err != nil {
			t.Fatal(err)
		}
		one = limits
		// Keep a local-input slot free so this assertion isolates the provider cap.
		one.GlobalConcurrency = 2
		one.ProviderConcurrency = 1
		c, err = repository.ClaimPODNode(ctx, a.RunID, one, time.Second)
		assert(err == nil && c != nil && c.Node.Provider == "simulation", "provider slot not acquired")
		d, err := repository.ClaimPODNode(ctx, b.RunID, one, time.Second)
		assert(err == nil && d != nil, "independent user cannot claim local input")
		out = executor.Simulation{}.Query(ctx, executor.Request{Node: d.Definition, Input: d.Input})
		if err := repository.FinishPODNode(ctx, d, out.State, out.Output, "", ""); err != nil {
			t.Fatal(err)
		}
		other, err = repository.ClaimPODNode(ctx, b.RunID, one, time.Second)
		assert(err == nil && other == nil, "provider cap bypassed")
		low := service.PODRunService{Limits: limits}
		low.Limits.MaxUserPending = 1
		_, err = low.Create(ctx, &users[0], request(p, "success"))
		assert(err != nil, "pending cap bypassed")
		out = executor.Simulation{}.Query(ctx, executor.Request{Node: c.Definition, Input: c.Input, Attempt: c.Node.Attempt})
		if err := repository.FinishPODNode(ctx, c, out.State, out.Output, "", ""); err != nil {
			t.Fatal(err)
		}
		finish(0, a.RunID)
		finish(1, b.RunID)
	})
	t.Run("inflight_cancel_confirms_started_work_and_releases_rest", func(t *testing.T) {
		r := create(0, p, "success")
		_, err := service.ExecutePODStep(ctx, r.RunID, limits, executor.Simulation{})
		if err != nil {
			t.Fatal(err)
		}
		c, err := repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		assert(err == nil && c != nil && c.Node.NodeID == "B", "expected started text call")
		expect(call("POST", fmt.Sprintf("/pod/runs/%d/cancel", r.RunID), tokens[0], map[string]any{}), 200)
		out := executor.Simulation{}.Query(ctx, executor.Request{Node: c.Definition, Input: c.Input, Attempt: c.Node.Attempt})
		if err := repository.FinishPODNode(ctx, c, out.State, out.Output, "", ""); err != nil {
			t.Fatal(err)
		}
		r = finish(0, r.RunID)
		assert(r.State == model.PODRunCancelled && r.Spent == 1 && r.Reserved == 0, "inflight cancellation refunded consumed work or charged waiting work")
	})
	t.Run("membership_revocation_stops_pending_execution", func(t *testing.T) {
		r := create(0, p, "success")
		if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=?", workspaces[0]).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=?", workspaces[0]).Update("status", "0")
		expect(call("GET", fmt.Sprintf("/pod/runs/%d", r.RunID), tokens[0], nil), 403)
		c, err := repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		assert(err == nil && c == nil, "revoked user work was started")
		var stored model.PODRun
		if err := repository.DB(ctx).Where("run_id=?", r.RunID).Take(&stored).Error; err != nil {
			t.Fatal(err)
		}
		assert(stored.State == model.PODRunCancelled && stored.Reserved == 0, "revoked pending work not settled")
	})
	t.Run("outbox_persists_without_queue_and_can_redeliver", func(t *testing.T) {
		r := create(0, p, "success")
		rows, err := repository.DuePODOutbox(ctx, time.Second)
		assert(err == nil, "outbox select failed")
		found := false
		for _, o := range rows {
			if o.RunID == r.RunID {
				found = true
			}
		}
		assert(found, "DB commit did not create durable outbox")
		if err := repository.DB(ctx).Table("pod_outbox").Where("run_id=?", r.RunID).Update("next_delivery_at", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		rows, err = repository.DuePODOutbox(ctx, time.Second)
		found = false
		for _, o := range rows {
			if o.RunID == r.RunID && o.DeliverySeq >= 2 {
				found = true
			}
		}
		assert(err == nil && found, "lost delivery could not be repeated")
		finish(0, r.RunID)
	})
	t.Run("disabled_account_stops_pending_work", func(t *testing.T) {
		r := create(0, p, "success")
		if err := repository.DB(ctx).Table("sys_user").Where("user_id=?", users[0].UserID).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Table("sys_user").Where("user_id=?", users[0].UserID).Update("status", "0")
		claim, err := repository.ClaimPODNode(ctx, r.RunID, limits, time.Second)
		assert(err == nil && claim == nil, "disabled account started pending work")
	})
	t.Run("immutable_snapshot_and_revision_contract", func(t *testing.T) {
		r := create(0, p, "success")
		g := graph()
		g.Nodes[0].Config = json.RawMessage(`{"text":"changed later"}`)
		payload := call("PUT", fmt.Sprintf("/pod/projects/%d", p.ProjectID), tokens[0], map[string]any{"revision": strconv.FormatInt(p.Revision, 10), "graph": g})
		expect(payload, 200)
		r = finish(0, r.RunID)
		assert(string(r.Graph.Nodes[0].Config) == `{"text": "immutable input"}` || string(r.Graph.Nodes[0].Config) == `{"text":"immutable input"}`, "running snapshot was modified")
		old := request(p, "success")
		expect(call("POST", "/pod/runs", tokens[0], old), 409)
		_ = json.Unmarshal(payload["data"], &p)
	})
	t.Run("integer_ledger_conservation_and_unique_events", func(t *testing.T) {
		var account model.PODCreditAccount
		err := repository.DB(ctx).Where("workspace_id=?", workspaces[0]).Take(&account).Error
		assert(err == nil && account.Available >= 0 && account.Held == 0 && account.Spent > 0 && account.Available+account.Held+account.Spent == limits.InitialCredits, "ledger conservation failed")
		var duplicates int64
		err = repository.DB(ctx).Raw("SELECT COUNT(*) FROM (SELECT event_key FROM pod_credit_event WHERE workspace_id=? GROUP BY event_key HAVING COUNT(*)>1) d", workspaces[0]).Scan(&duplicates).Error
		assert(err == nil && duplicates == 0, "duplicate ledger events")
		expect(call("GET", "/pod/credits?pageSize=10000", tokens[0], nil), 200)
	})
	t.Run("concurrent_reservations_cannot_overspend", func(t *testing.T) {
		third := newProject(2, graph())
		low := service.PODRunService{Limits: limits}
		low.Limits.InitialCredits = 2
		type result struct {
			r   *model.PODRunView
			err error
		}
		ch := make(chan result, 2)
		for i := 0; i < 2; i++ {
			go func() { r, err := low.Create(ctx, &users[2], request(third, "success")); ch <- result{r, err} }()
		}
		a, b := <-ch, <-ch
		var winner *model.PODRunView
		if a.err == nil {
			winner = a.r
		}
		if b.err == nil {
			assert(winner == nil, "two reservations spent the same balance")
			winner = b.r
		}
		assert(winner != nil && (a.err != nil || b.err != nil), "expected one reservation and one insufficient balance")
		var account model.PODCreditAccount
		if err := repository.DB(ctx).Where("workspace_id=?", workspaces[2]).Take(&account).Error; err != nil {
			t.Fatal(err)
		}
		assert(account.Available == 0 && account.Held == 2, "reservation balance invalid")
		finish(2, winner.RunID)
	})
	t.Run("historical_run_reference_survives_project_edit_and_delete", func(t *testing.T) {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
			t.Fatal(err)
		}
		asset, err := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{}).Create(ctx, &users[0], "run-reference.png", "image/png", &buf)
		if err != nil {
			t.Fatal(err)
		}
		assetIDs = append(assetIDs, asset.AssetID)
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "image", Type: "imageInput", Version: 1, Config: json.RawMessage(fmt.Sprintf(`{"assetIds":["%d"]}`, asset.AssetID))}}, Edges: []model.PODEdge{}}
		imageProject := newProject(0, g)
		r := create(0, imageProject, "success")
		assert(finish(0, r.RunID).State == model.PODRunSucceeded, "image input failed")
		payload := call("PUT", fmt.Sprintf("/pod/projects/%d", imageProject.ProjectID), tokens[0], map[string]any{"revision": strconv.FormatInt(imageProject.Revision, 10), "graph": model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}}})
		expect(payload, 200)
		_ = json.Unmarshal(payload["data"], &imageProject)
		expect(call("DELETE", fmt.Sprintf("/pod/projects/%d", imageProject.ProjectID), tokens[0], model.PODProjectVersion{Revision: strconv.FormatInt(imageProject.Revision, 10)}), 200)
		expect(call("DELETE", fmt.Sprintf("/pod/assets/%d", asset.AssetID), tokens[0], model.PODProjectVersion{Revision: "1"}), 409)
		expect(call("GET", fmt.Sprintf("/pod/runs/%d", r.RunID), tokens[0], nil), 200)
	})
	t.Run("strict_DTO_and_unimplemented_nodes_rejected", func(t *testing.T) {
		expect(call("POST", "/pod/runs", tokens[0], map[string]any{"requestId": uuid.NewString(), "workspaceId": "1"}), 400)
		expect(call("GET", "/pod/runs/01", tokens[0], nil), 400)
		g := graph()
		g.Nodes[1].Type = "imageGenerate"
		g.Nodes[1].Config = json.RawMessage(`{}`)
		g.Nodes = g.Nodes[:2]
		g.Edges = g.Edges[:1]
		unsupported := newProject(0, g)
		expect(call("POST", "/pod/runs", tokens[0], request(unsupported, "success")), 400)
	})
	t.Run("real_queue_lost_delivery_worker_crash_and_restart", func(t *testing.T) {
		var unrelated int64
		if err := repository.DB(ctx).Table("pod_run").Where("state IN ? AND workspace_id NOT IN ?", []string{"queued", "running", "cancelRequested", "awaitingReconciliation"}, workspaces).Count(&unrelated).Error; err != nil {
			t.Fatal(err)
		}
		if unrelated != 0 {
			t.Skip("another workspace has active work; do not dispatch it in this test")
		}
		testCfg := *cfg
		testCfg.POD.InstanceID = "pod-test-" + uuid.NewString()[:8]
		inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
		defer inspector.Close()
		defer func() {
			if err := inspector.DeleteQueue(service.PODQueueName(testCfg.POD.InstanceID), true); err != nil {
				t.Error("test queue cleanup failed", err)
			}
			_ = redisx.C().Del(ctx, "pod:worker:heartbeat:"+testCfg.POD.InstanceID).Err()
		}()
		g := graph()
		g.Nodes = g.Nodes[:2]
		g.Edges = g.Edges[:1]
		project := newProject(0, g)
		r := create(0, project, "delay")
		if _, err := service.DispatchPODRuns(ctx, &testCfg); err != nil {
			t.Fatal(err)
		}
		taskID := fmt.Sprintf("run-%d", r.RunID)
		info, err := inspector.GetTaskInfo(service.PODQueueName(testCfg.POD.InstanceID), taskID)
		assert(err == nil && info.State == asynq.TaskStatePending, "outbox did not deliver to real queue")
		if err := inspector.DeleteTask(service.PODQueueName(testCfg.POD.InstanceID), taskID); err != nil {
			t.Fatal(err)
		}
		if err := repository.DB(ctx).Table("pod_outbox").Where("run_id=?", r.RunID).Update("next_delivery_at", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := service.DispatchPODRuns(ctx, &testCfg); err != nil {
			t.Fatal(err)
		}
		_, err = inspector.GetTaskInfo(service.PODQueueName(testCfg.POD.InstanceID), taskID)
		assert(err == nil, "lost queue task was not redelivered")
		worker := startPODTestWorker(t, ctx, cfg, testCfg.POD.InstanceID)
		waitFor := func(predicate func() bool) {
			t.Helper()
			deadline := time.Now().Add(25 * time.Second)
			for time.Now().Before(deadline) {
				if predicate() {
					return
				}
				select {
				case <-ctx.Done():
					t.Fatal("worker test timed out")
				case <-time.After(25 * time.Millisecond):
				}
			}
			t.Fatal("worker progress deadline exceeded")
		}
		waitFor(func() bool {
			var n int64
			err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id='B' AND state='running'", r.RunID).Count(&n).Error
			return err == nil && n == 1
		})
		worker.stop(t, true)
		// Advance only this fixture task's Redis lease too, so the restarted Asynq recoverer can reclaim it immediately.
		if err := redisx.C().ZAdd(ctx, "asynq:{"+service.PODQueueName(testCfg.POD.InstanceID)+"}:lease", redis.Z{Score: float64(time.Now().Add(-2 * time.Minute).Unix()), Member: taskID}).Err(); err != nil {
			t.Fatal(err)
		}
		var interrupted model.PODNodeRun
		if err := repository.DB(ctx).Where("run_id=? AND node_id='B'", r.RunID).Take(&interrupted).Error; err != nil {
			t.Fatal(err)
		}
		// Advance the fixture's lease clock instead of waiting 30 seconds. Redis recovery hint is a duplicate wake, not a second Submit.
		if err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id='B'", r.RunID).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		enqueueRecovery(t, ctx, cfg, testCfg.POD.InstanceID, r.RunID)
		restarted := startPODTestWorker(t, ctx, cfg, testCfg.POD.InstanceID)
		waitFor(func() bool { return workflow.RunTerminal(get(0, r.RunID).State) })
		restarted.stop(t, false)
		done := get(0, r.RunID)
		assert(done.State == model.PODRunSucceeded && done.Spent == 1 && done.Reserved == 0, "restart did not settle once")
		var recovered model.PODNodeRun
		if err := repository.DB(ctx).Where("run_id=? AND node_id='B'", r.RunID).Take(&recovered).Error; err != nil {
			t.Fatal(err)
		}
		assert(recovered.SubmissionKey == interrupted.SubmissionKey && recovered.Attempt == 1, "restart resubmitted the uncertain operation")
	})
	t.Run("worker_graceful_stop_drains_inflight_call", func(t *testing.T) {
		testCfg := *cfg
		testCfg.POD.InstanceID = "pod-drain-" + uuid.NewString()[:8]
		inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
		defer inspector.Close()
		defer func() {
			_ = inspector.DeleteQueue(service.PODQueueName(testCfg.POD.InstanceID), true)
			_ = redisx.C().Del(ctx, "pod:worker:heartbeat:"+testCfg.POD.InstanceID).Err()
		}()
		g := graph()
		g.Nodes = g.Nodes[:2]
		g.Edges = g.Edges[:1]
		project := newProject(0, g)
		r := create(0, project, "delay")
		worker := startPODTestWorker(t, ctx, cfg, testCfg.POD.InstanceID)
		deadline := time.Now().Add(20 * time.Second)
		started := false
		for time.Now().Before(deadline) {
			var count int64
			err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id='B' AND state='running'", r.RunID).Count(&count).Error
			if err == nil && count == 1 {
				started = true
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		assert(started, "worker did not start delayed call")
		worker.stop(t, false)
		var n model.PODNodeRun
		if err := repository.DB(ctx).Where("run_id=? AND node_id='B'", r.RunID).Take(&n).Error; err != nil {
			t.Fatal(err)
		}
		assert(n.State == "succeeded", "graceful shutdown abandoned inflight work")
		assert(get(0, r.RunID).State == model.PODRunSucceeded, "drained run not settled")
	})
	t.Run("history_page_cap_stable_ties_and_private_DTO", func(t *testing.T) {
		encoded, _ := json.Marshal(graph())
		now := time.Now()
		fixtures := []model.PODRun{}
		for i := 0; i < 103; i++ {
			fixtures = append(fixtures, model.PODRun{WorkspaceID: workspaces[0], UserID: users[0].UserID, ProjectID: p.ProjectID, ProjectRevision: p.Revision, ProjectName: p.Name, RequestID: uuid.NewString(), RequestHash: strings.Repeat("0", 64), Mode: "workflow", GraphJSON: encoded, BaseOutputsJSON: json.RawMessage(`{}`), State: model.PODRunSucceeded, TestScenario: "success", Attempt: 1, Settled: true, CreateBy: users[0].UserName, UpdateBy: users[0].UserName, CreateTime: now, UpdateTime: now})
		}
		if err := repository.DB(ctx).Create(&fixtures).Error; err != nil {
			t.Fatal(err)
		}
		first := call("GET", "/pod/runs?pageSize=10000&pageNum=1", tokens[0], nil)
		second := call("GET", "/pod/runs?pageSize=10000&pageNum=2", tokens[0], nil)
		expect(first, 200)
		expect(second, 200)
		var a, b []map[string]json.RawMessage
		_ = json.Unmarshal(first["rows"], &a)
		_ = json.Unmarshal(second["rows"], &b)
		assert(len(a) == 100 && len(b) > 0 && len(b) <= 100, "page hard cap failed")
		seen := map[string]bool{}
		last := int64(1<<63 - 1)
		for _, row := range a {
			var id string
			_ = json.Unmarshal(row["runId"], &id)
			number, _ := strconv.ParseInt(id, 10, 64)
			assert(number < last, "equal-time ordering lacks unique descending tiebreaker")
			last = number
			seen[id] = true
			for _, key := range []string{"requestHash", "graphJSON", "baseOutputsJSON", "userId", "leaseToken"} {
				_, leaked := row[key]
				assert(!leaked, "internal run field leaked")
			}
		}
		for _, row := range b {
			var id string
			_ = json.Unmarshal(row["runId"], &id)
			assert(!seen[id], "run repeated between stable pages")
		}
	})
}
