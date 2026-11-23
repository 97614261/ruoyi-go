package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func exercisePODOperationsOfficial(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string) {
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1, UserName: "admin"}, Permissions: []string{"*:*:*"}}
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	expect := func(v map[string]json.RawMessage, code int) {
		t.Helper()
		if string(v["code"]) != strconv.Itoa(code) {
			t.Fatalf("code %s want %d: %s", v["code"], code, v["msg"])
		}
	}
	limits := service.PODExecutionLimits(cfg.POD.Execution)
	policy := model.PODCreatorPolicyInput{Status: "active", MaxConcurrentNodes: 1, MaxPendingRuns: limits.MaxUserPending, MaxDailySubmissions: 1000, MaxStorageBytes: "1073741824"}
	creator, err := service.PODCreatorWrite(ctx, admin, "creator.create", 0, model.PODCreatorWrite{RequestID: uuid.NewString(), Reason: "官方模板专用本地验收", UserName: "official_" + uuid.NewString()[:8], NickName: "官方模板测试", InitialPassword: "Of-" + uuid.NewString()[:15], Policy: &policy}, limits)
	if err != nil {
		t.Fatal(err)
	}
	user := &model.LoginUser{UserID: creator.ResourceID, User: &model.SysUser{UserID: creator.ResourceID, UserName: "official_fixture"}, Permissions: []string{model.PODStudioAccess}}
	t.Cleanup(func() {
		d, e := repository.PODCreator(context.Background(), user.UserID)
		if e != nil {
			t.Error(e)
			return
		}
		policy.Status = "suspended"
		_, e = service.PODCreatorWrite(context.Background(), admin, "creator.policy", user.UserID, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(d.Creator.Revision), Reason: "本轮结束，停用保留历史", Policy: &policy}, limits)
		if e != nil {
			t.Error(e)
		}
		if e = repository.PODVerifyCreatorAccounts(context.Background()); e != nil {
			t.Error(e)
		}
	})
	svc := service.NewPODOfficialService(cfg.POD.StorageRoot, model.PODGraphLimits{}, model.PODAssetLimits{})
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 24, 24))); err != nil {
		t.Fatal(err)
	}
	uploadRequest := uuid.NewString()
	asset, err := svc.Upload(ctx, admin, uploadRequest, "本地原创测试图片", "official.png", "image/png", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("upload_replay_and_content_conflict", func(t *testing.T) {
		again, e := svc.Upload(ctx, admin, uploadRequest, "本地原创测试图片", "official.png", "image/png", bytes.NewReader(buf.Bytes()))
		if e != nil || again.ResourceID != asset.ResourceID {
			t.Fatal("upload replay differs", e)
		}
		_, e = svc.Upload(ctx, admin, uploadRequest, "changed license", "official.png", "image/png", bytes.NewReader(buf.Bytes()))
		if b := errs.As(e); b == nil || b.Code != 409 {
			t.Fatal("upload conflict not rejected", e)
		}
	})
	graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "text", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"official-private-marker"}`)}, {ID: "image", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + id(asset.ResourceID) + `"]}`)}}, Edges: []model.PODEdge{}}
	makeRequest := model.PODOfficialWrite{RequestID: uuid.NewString(), Reason: "本地官方模板验收", Code: "official-" + uuid.NewString()[:8], Name: "本地官方模板测试", Graph: &graph}
	created, err := svc.Write(ctx, admin, 0, "official.create", makeRequest)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.Get(ctx, admin, created.ResourceID, 0, true)
	if err != nil || detail.Snapshot == nil {
		t.Fatal("read draft", err)
	}
	version := detail.Snapshot.VersionID
	tid := created.ResourceID
	t.Run("draft_hidden_and_creator_cannot_admin", func(t *testing.T) {
		_, e := svc.Get(ctx, user, tid, 0, false)
		if b := errs.As(e); b == nil || b.Code != 404 {
			t.Fatal("draft visible", e)
		}
		for _, path := range []string{"/pod-admin/runs", "/pod-admin/batches", "/pod-admin/usage", "/pod-admin/private-grants", "/pod-admin/catalog/templates", "/pod-admin/catalog/assets"} {
			expect(call("GET", path, tokens["pod_creator"], nil), 403)
			expect(call("GET", path, "", nil), 401)
		}
	})
	published, err := svc.Write(ctx, admin, tid, "official.publish", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(created.Revision), VersionID: id(version), Reason: "发布本地测试模板"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d, e := svc.Get(context.Background(), admin, tid, 0, true)
		if e == nil {
			_, e = svc.Write(context.Background(), admin, tid, "official.unpublish", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(d.Template.Revision), Reason: "测试结束下架保留"})
		}
		if e != nil {
			t.Error(e)
		}
	})
	use := model.PODTemplateUse{RequestID: uuid.NewString(), Version: id(version), Name: "官方副本测试", Bindings: []model.PODTemplateBinding{}}
	var projectID int64
	t.Run("interrupted_copy_retains_one_plan_and_resumes_exact_files", func(t *testing.T) {
		file, e := repository.PODOfficialFile(ctx, asset.ResourceID, "thumbnail")
		if e != nil {
			t.Fatal(e)
		}
		source := filepath.Join(svc.Store.Path, file.StorageKey)
		moved := source + ".test-hold"
		if e = os.Rename(source, moved); e != nil {
			t.Fatal(e)
		}
		restored := false
		defer func() {
			if !restored {
				if e := os.Rename(moved, source); e != nil {
					t.Error(e)
				}
			}
		}()
		intent := use
		intent.RequestID = uuid.NewString()
		intent.Name = "中断后继续的副本"
		if _, e = svc.Use(ctx, user, tid, intent); errs.As(e) == nil || errs.As(e).Code != 409 {
			t.Fatal("missing source did not retain pending copy", e)
		}
		a, e := repository.PODOfficialUseAction(ctx, user.UserID, intent.RequestID)
		if e != nil || a.State != "copying" || a.ProjectID != 0 {
			t.Fatal("plan missing", e)
		}
		var reservation model.PODStorageReservation
		if e = repository.DB(ctx).Where("user_id=? AND operation_key=?", user.UserID, a.StorageOperationKey).Take(&reservation).Error; e != nil || reservation.State != "held" || reservation.ReservedBytes <= 0 {
			t.Fatal("storage hold missing", e)
		}
		if _, e = svc.Use(ctx, user, tid, intent); errs.As(e) == nil || errs.As(e).Code != 409 {
			t.Fatal("pending retry unexpectedly succeeded", e)
		}
		if e = os.Rename(moved, source); e != nil {
			t.Fatal(e)
		}
		restored = true
		completed, e := svc.Use(ctx, user, tid, intent)
		if e != nil || completed.State != "completed" {
			t.Fatal("resume failed", e)
		}
		again, e := svc.Use(ctx, user, tid, intent)
		if e != nil || again.ProjectID != completed.ProjectID {
			t.Fatal("resume duplicated result", e)
		}
		if e = repository.DB(ctx).Where("reservation_id=?", reservation.ReservationID).Take(&reservation).Error; e != nil || reservation.State != "committed" || reservation.CommittedBytes != reservation.ReservedBytes {
			t.Fatal("storage hold not committed exactly", e)
		}
	})
	t.Run("copy_is_private_atomic_and_idempotent_under_concurrency", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan *model.PODOfficialUseView, 2)
		errors := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); v, e := svc.Use(ctx, user, tid, use); results <- v; errors <- e }()
		}
		wg.Wait()
		close(results)
		close(errors)
		for e := range errors {
			if e != nil {
				t.Fatal(e)
			}
		}
		for v := range results {
			if v == nil || v.State != "completed" || v.ProjectID == 0 {
				t.Fatal("copy incomplete")
			}
			if projectID != 0 && projectID != v.ProjectID {
				t.Fatal("duplicate project")
			}
			projectID = v.ProjectID
		}
		p, e := service.GetPODProject(ctx, user.User, projectID)
		if e != nil {
			t.Fatal(e)
		}
		var copied model.PODGraph
		raw, _ := json.Marshal(p.Graph)
		_ = json.Unmarshal(raw, &copied)
		if strings.Contains(string(raw), `"assetIds":["`+id(asset.ResourceID)+`"]`) {
			var personal int64
			_ = repository.DB(ctx).Table("pod_project_asset_ref").Select("asset_id").Where("project_id=?", projectID).Scan(&personal).Error
			if personal == asset.ResourceID {
				t.Fatal("platform id not copied")
			}
		}
		if e := repository.PODVerifyCreatorAccounts(ctx); e != nil {
			t.Fatal(e)
		}
		changed := use
		changed.Name = "other"
		if _, e = svc.Use(ctx, user, tid, changed); errs.As(e) == nil || errs.As(e).Code != 409 {
			t.Fatal("request changed without conflict", e)
		}
	})
	t.Run("personal_replacement_never_copies_or_borrows_other_users_image", func(t *testing.T) {
		var personal int64
		if e := repository.DB(ctx).Table("pod_project_asset_ref").Select("asset_id").Where("project_id=?", projectID).Scan(&personal).Error; e != nil {
			t.Fatal(e)
		}
		ids := []string{id(personal)}
		intent := use
		intent.RequestID = uuid.NewString()
		intent.Bindings = []model.PODTemplateBinding{{NodeID: "image", AssetIDs: &ids}}
		var own model.PODAsset
		if e := repository.DB(ctx).Table("pod_asset").Where("asset_id=?", personal).Take(&own).Error; e != nil {
			t.Fatal(e)
		}
		before := int64(0)
		if e := repository.DB(ctx).Table("pod_asset").Where("workspace_id=?", own.WorkspaceID).Count(&before).Error; e != nil {
			t.Fatal(e)
		}
		copied, e := svc.Use(ctx, user, tid, intent)
		if e != nil || copied.State != "completed" {
			t.Fatal("personal replacement failed", e)
		}
		a, e := repository.PODOfficialUseAction(ctx, user.UserID, intent.RequestID)
		if e != nil {
			t.Fatal(e)
		}
		var plan model.PODOfficialCopyPlan
		_ = json.Unmarshal(a.CopyMapJSON, &plan)
		if len(plan.Copies) != 0 || len(plan.PersonalNodes) != 1 {
			t.Fatal("replaced image still copied platform files")
		}
		var after int64
		if e := repository.DB(ctx).Table("pod_asset").Where("workspace_id=?", own.WorkspaceID).Count(&after).Error; e != nil || before != after {
			t.Fatal("personal replacement duplicated image", e)
		}
		var foreign model.PODAsset
		if e := repository.DB(ctx).Table("pod_asset").Where("workspace_id<>? AND del_flag='0'", a.WorkspaceID).Take(&foreign).Error; e == nil {
			ids = []string{id(foreign.AssetID)}
			intent.RequestID = uuid.NewString()
			intent.Bindings = []model.PODTemplateBinding{{NodeID: "image", AssetIDs: &ids}}
			if _, e = svc.Use(ctx, user, tid, intent); e == nil {
				t.Fatal("foreign private image accepted")
			}
		}
	})
	t.Run("three_starters_publish_and_copy_without_running_models", func(t *testing.T) {
		seeds, e := workflow.StarterTemplates()
		if e != nil {
			t.Fatal(e)
		}
		var personal int64
		if e = repository.DB(ctx).Table("pod_project_asset_ref").Select("asset_id").Where("project_id=?", projectID).Scan(&personal).Error; e != nil || personal == 0 {
			t.Fatal("personal test image unavailable", e)
		}
		var before int64
		if e = repository.DB(ctx).Table("pod_run").Where("user_id=?", user.UserID).Count(&before).Error; e != nil {
			t.Fatal(e)
		}
		for _, seed := range seeds {
			created, e := svc.Write(ctx, admin, 0, "official.create", model.PODOfficialWrite{RequestID: uuid.NewString(), Reason: "入门模板副本验收", Code: "starter-test-" + uuid.NewString()[:8], Name: seed.Name, Description: seed.Description, Graph: &seed.Graph})
			if e != nil {
				t.Fatal(seed.Code, e)
			}
			d, e := svc.Get(ctx, admin, created.ResourceID, 0, true)
			if e != nil || d.Snapshot == nil {
				t.Fatal("starter draft missing", e)
			}
			_, e = svc.Write(ctx, admin, created.ResourceID, "official.publish", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(created.Revision), VersionID: id(d.Snapshot.VersionID), Reason: "入门模板发布验收"})
			if e != nil {
				t.Fatal("starter publish", e)
			}
			intent := model.PODTemplateUse{RequestID: uuid.NewString(), Version: id(d.Snapshot.VersionID), Name: seed.Name}
			for _, n := range seed.Graph.Nodes {
				if n.Type == "imageInput" {
					ids := []string{id(personal)}
					intent.Bindings = append(intent.Bindings, model.PODTemplateBinding{NodeID: n.ID, AssetIDs: &ids})
				}
			}
			copied, e := svc.Use(ctx, user, created.ResourceID, intent)
			if e != nil || copied.State != "completed" || copied.ProjectID == 0 {
				t.Fatal("starter copy", seed.Code, e)
			}
			again, e := svc.Use(ctx, user, created.ResourceID, intent)
			if e != nil || again.ProjectID != copied.ProjectID {
				t.Fatal("starter copy replay duplicated project", e)
			}
			project, e := service.GetPODProject(ctx, user.User, copied.ProjectID)
			if e != nil || len(project.Graph.Nodes) != len(seed.Graph.Nodes) {
				t.Fatal("starter graph lost nodes", e)
			}
		}
		var after int64
		if e = repository.DB(ctx).Table("pod_run").Where("user_id=?", user.UserID).Count(&after).Error; e != nil || after != before {
			t.Fatal("copying starters must not execute models", e)
		}
	})
	t.Run("publish_versions_do_not_change_existing_projects_and_unpublish_keeps_replay", func(t *testing.T) {
		g := graph
		g.Nodes = append([]model.PODNode(nil), graph.Nodes...)
		g.Nodes[0].Config = json.RawMessage(`{"text":"new version"}`)
		next, e := svc.Write(ctx, admin, tid, "official.version", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(published.Revision), Graph: &g, Reason: "新增测试版本"})
		if e != nil {
			t.Fatal(e)
		}
		old, e := repository.PODOfficialVersion(ctx, tid, version)
		if e != nil || !strings.Contains(string(old.GraphJSON), "official-private-marker") || strings.Contains(string(old.GraphJSON), "new version") {
			t.Fatal("immutable version overwritten", e)
		}
		p, e := service.GetPODProject(ctx, user.User, projectID)
		raw, _ := json.Marshal(p)
		if e != nil || !strings.Contains(string(raw), "official-private-marker") {
			t.Fatal("old project changed with template", e)
		}
		_, e = svc.Write(ctx, admin, tid, "official.unpublish", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(next.Revision), Reason: "下架测试"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = svc.Get(ctx, user, tid, 0, false); errs.As(e) == nil || errs.As(e).Code != 404 {
			t.Fatal("unpublished visible", e)
		}
		again, e := svc.Use(ctx, user, tid, use)
		if e != nil || again.ProjectID != projectID {
			t.Fatal("completed replay lost after unpublish", e)
		}
		fresh := use
		fresh.RequestID = uuid.NewString()
		if _, e = svc.Use(ctx, user, tid, fresh); errs.As(e) == nil || errs.As(e).Code != 404 {
			t.Fatal("fresh use allowed after unpublish", e)
		}
	})
	runSvc := service.PODRunService{Limits: limits}
	var project model.PODProject
	if err := repository.DB(ctx).Table("pod_project").Where("project_id=?", projectID).Take(&project).Error; err != nil {
		t.Fatal(err)
	}
	run, err := runSvc.Create(ctx, user.User, model.PODRunCreate{ExecutionMode: "simulation", ProjectID: id(projectID), Revision: id(project.Revision), RequestID: uuid.NewString(), Mode: "workflow"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("metadata_never_contains_private_content_and_admin_needs_grant", func(t *testing.T) {
		for _, path := range []string{"/pod-admin/runs", "/pod-admin/runs/" + id(run.RunID) + "/nodes", "/pod-admin/usage"} {
			body := call("GET", path, tokens["admin"], nil)
			expect(body, 200)
			raw, _ := json.Marshal(body)
			for _, s := range []string{"official-private-marker", "graph", "outputJSON", "storageKey", "receiptJSON", "projectName"} {
				if strings.Contains(string(raw), s) {
					t.Fatal("metadata leaked", s)
				}
			}
		}
		_, e := service.PODPrivateRun(ctx, admin, run.RunID)
		if b := errs.As(e); b == nil || b.Code != 403 {
			t.Fatal("root bypassed grant", e)
		}
	})
	t.Run("grant_revoke_expiry_and_run_grant_does_not_grant_asset", func(t *testing.T) {
		r := model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "排查专用测试资源", OperatorUserID: "1", OwnerUserID: id(user.UserID), ResourceType: "run", ResourceID: id(run.RunID), ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}
		grant, e := service.PODOperation(ctx, admin, "private.grant", 0, r)
		if e != nil {
			t.Fatal(e)
		}
		again, e := service.PODOperation(ctx, admin, "private.grant", 0, r)
		if e != nil || again.ResourceID != grant.ResourceID {
			t.Fatal("grant replay", e)
		}
		private, e := service.PODPrivateRun(ctx, admin, run.RunID)
		if e != nil || private.Graph == nil {
			t.Fatal("grant not honored", e)
		}
		hook := "pod-official-private-audit-" + uuid.NewString()
		if e = repository.DB(ctx).Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if audit, ok := tx.Statement.Dest.(*model.PODAdminAudit); ok && audit.ActionType == "private.read" {
				tx.AddError(errors.New("fixture audit unavailable"))
			}
		}); e != nil {
			t.Fatal(e)
		}
		value, blocked := service.PODPrivateRun(ctx, admin, run.RunID)
		_ = repository.DB(ctx).Callback().Create().Remove(hook)
		if blocked == nil || value != nil {
			t.Fatal("private content escaped failed audit")
		}
		var personal int64
		_ = repository.DB(ctx).Table("pod_project_asset_ref").Select("asset_id").Where("project_id=?", projectID).Scan(&personal).Error
		file, _, _, e := svc.Personal.AdminOpen(ctx, admin, personal, "original")
		if file != nil {
			file.Close()
		}
		if b := errs.As(e); b == nil || b.Code != 403 {
			t.Fatal("run grant leaked asset", e)
		}
		assetGrant := r
		assetGrant.RequestID = uuid.NewString()
		assetGrant.ResourceType = "asset"
		assetGrant.ResourceID = id(personal)
		assetPermission, e := service.PODOperation(ctx, admin, "private.grant", 0, assetGrant)
		if e != nil {
			t.Fatal(e)
		}
		file, _, _, e = svc.Personal.AdminOpen(ctx, admin, personal, "original")
		if e != nil || file == nil {
			t.Fatal("valid asset grant could not read", e)
		}
		file.Close()
		if e = repository.DB(ctx).Model(&model.PODPrivateGrant{}).Where("grant_id=?", assetPermission.ResourceID).Update("expires_at", time.Now().Add(-time.Minute)).Error; e != nil {
			t.Fatal(e)
		}
		file, _, _, e = svc.Personal.AdminOpen(ctx, admin, personal, "original")
		if file != nil {
			file.Close()
		}
		if errs.As(e) == nil || errs.As(e).Code != 403 {
			t.Fatal("expired stored asset grant accepted", e)
		}
		_, e = service.PODOperation(ctx, admin, "private.revoke", grant.ResourceID, model.PODOperationWrite{RequestID: uuid.NewString(), Revision: id(grant.Revision), Reason: "测试撤销"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = service.PODPrivateRun(ctx, admin, run.RunID); errs.As(e) == nil || errs.As(e).Code != 403 {
			t.Fatal("revocation stale", e)
		}
		r.RequestID = uuid.NewString()
		r.ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)
		if _, e = service.PODOperation(ctx, admin, "private.grant", 0, r); errs.As(e) == nil || errs.As(e).Code != 400 {
			t.Fatal("expired grant accepted", e)
		}
		wrong := r
		wrong.RequestID = uuid.NewString()
		wrong.OwnerUserID = "1"
		wrong.ExpiresAt = time.Now().Add(time.Hour).Format(time.RFC3339)
		if _, e = service.PODOperation(ctx, admin, "private.grant", 0, wrong); errs.As(e) == nil || errs.As(e).Code != 404 {
			t.Fatal("wrong owner grant accepted", e)
		}
	})
	t.Run("cancel_is_audited_and_idempotent", func(t *testing.T) {
		r := model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "取消本地测试运行"}
		v, e := service.PODOperation(ctx, admin, "task.cancel", run.RunID, r)
		if e != nil {
			t.Fatal(e)
		}
		again, e := service.PODOperation(ctx, admin, "task.cancel", run.RunID, r)
		if e != nil || v.ResourceID != again.ResourceID {
			t.Fatal("cancel replay failed", e)
		}
		var audits int64
		_ = repository.DB(ctx).Table("pod_admin_audit").Where("action_type='task.cancel' AND resource_id=?", v.ResourceID).Count(&audits).Error
		if audits != 1 {
			t.Fatal("cancel audit duplicated")
		}
	})
	t.Run("bounded_lists_denials_and_restore_invariants", func(t *testing.T) {
		for _, path := range []string{"/pod-admin/runs?beginDate=2000-01-01", "/pod-admin/usage?beginDate=invalid", "/pod-admin/batches?userId=0001"} {
			expect(call("GET", path, tokens["admin"], nil), 400)
		}
		expect(call("POST", "/pod-admin/runs/"+id(run.RunID)+"/cancel", tokens["pod_creator"], model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "not allowed"}), 403)
		f, e := service.PODTaskFilter("", "", "", "", "")
		if e != nil {
			t.Fatal(e)
		}
		rows, total, e := service.PODAdminUsage(ctx, admin, f, page.Query{PageNum: 1, PageSize: 100})
		if e != nil || total < int64(len(rows)) {
			t.Fatal("usage paging", e)
		}
		for _, row := range rows {
			var actual struct {
				Calls    int64
				Charged  int64
				Reserved int64
			}
			q := repository.DB(ctx).Table("pod_provider_call c").Joins("JOIN pod_run r ON r.run_id=c.run_id").Joins("LEFT JOIN pod_run_model_binding b ON b.run_id=c.run_id AND b.node_id=c.node_id").Where("r.user_id=? AND COALESCE(b.model_id,0)=? AND c.model=? AND c.create_time>=? AND c.create_time<?", row.UserID, row.ModelID, row.Model, f.Begin, f.End).Select("COUNT(*) calls,COALESCE(SUM(c.charged_micros),0) charged,COALESCE(SUM(c.reserved_micros),0) reserved")
			if e = q.Scan(&actual).Error; e != nil || actual.Calls != row.Calls || actual.Charged != row.ChargedMicros || actual.Reserved != row.ReservedMicros {
				t.Fatal("usage lost call history", e)
			}
		}
		if _, e := repository.PODVerifyOfficialData(ctx); e != nil {
			t.Fatal(e)
		}
	})
}
