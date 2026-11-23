package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/storage"
)

func exercisePODTemplates(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage, handler http.Handler) {
	users := []model.SysUser{}
	spaces := []int64{}
	tokens := []string{}
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanupCreatorFixtures(t, clean, spaces)
		for _, token := range tokens {
			call("POST", "/logout", token, nil)
		}
		var files []model.PODAssetFile
		if err := repository.DB(clean).Table("pod_asset_file f").Select("f.*").Joins("JOIN pod_asset a ON a.asset_id=f.asset_id").Where("a.workspace_id IN ?", spaces).Find(&files).Error; err != nil {
			t.Error(err)
		}
		for _, f := range files {
			if err := (storage.Local{Path: cfg.POD.StorageRoot}).Remove(clean, f.StorageKey); err != nil {
				t.Error(err)
			}
		}
		if err := repository.DB(clean).Exec("DELETE f FROM pod_asset_file f JOIN pod_asset a ON a.asset_id=f.asset_id WHERE a.workspace_id IN ?", spaces).Error; err != nil {
			t.Error(err)
		}
		if err := repository.DB(clean).Exec("DELETE o FROM pod_outbox o JOIN pod_run r ON r.run_id=o.run_id WHERE r.workspace_id IN ?", spaces).Error; err != nil {
			t.Error(err)
		}
		for _, table := range []string{"pod_template_action", "pod_template_asset_ref", "pod_template_version", "pod_template", "pod_provider_asset", "pod_provider_call", "pod_run_asset_ref", "pod_node_run", "pod_retry_request", "pod_credit_event", "pod_credit_account", "pod_run", "pod_project_asset_ref", "pod_project", "pod_asset", "pod_workspace_member", "pod_workspace"} {
			if err := repository.DB(clean).Table(table).Where("workspace_id IN ?", spaces).Delete(map[string]any{}).Error; err != nil {
				t.Error(table, err)
			}
		}
		for _, u := range users {
			if err := repository.DB(clean).Table("sys_user").Where("user_id=?", u.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
			if err := redisx.C().Del(clean, redisx.LoginUserSessionsKey(u.UserID), redisx.LoginUserGenerationKey(u.UserID), redisx.LoginUserPermissionVersionKey(u.UserID)).Err(); err != nil {
				t.Error(err)
			}
		}
	})
	for range 2 {
		u := model.SysUser{UserName: "podtpl_" + uuid.NewString()[:8], NickName: "Template fixture", Status: "0", DelFlag: "0"}
		if err := repository.DB(ctx).Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		w, err := service.InitPODWorkspace(ctx, &u)
		if err != nil {
			t.Fatal(err)
		}
		spaces = append(spaces, w.WorkspaceID)
		token, err := service.CreateToken(ctx, &model.LoginUser{UserID: u.UserID, User: &u, Permissions: []string{model.PODStudioAccess}})
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	check := func(payload map[string]json.RawMessage, want int) {
		t.Helper()
		var code int
		_ = json.Unmarshal(payload["code"], &code)
		if code != want {
			var msg string
			_ = json.Unmarshal(payload["msg"], &msg)
			t.Fatalf("template code=%d want=%d message=%s", code, want, msg)
		}
	}
	var paidBefore int64
	if err := repository.DB(ctx).Table("pod_provider_call").Count(&paidBefore).Error; err != nil {
		t.Fatal(err)
	}
	assets := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{})
	assetIDs := []string{}
	for i := range users {
		im := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		im.SetNRGBA(30, 30, color.NRGBA{G: 180, A: 255})
		var raw bytes.Buffer
		_ = png.Encode(&raw, im)
		a, err := assets.Create(ctx, &users[i], "template-fixture.png", "image/png", &raw)
		if err != nil {
			t.Fatal(err)
		}
		assetIDs = append(assetIDs, strconv.FormatInt(a.AssetID, 10))
	}
	graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
		{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"title":"创意要求","text":"original"}`)},
		{ID: "B", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + assetIDs[0] + `"]}`)},
		{ID: "b", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + assetIDs[0] + `"]}`)},
		{ID: "C", Type: "imageProcess", Version: 1, Config: json.RawMessage(`{"operation":"resize","width":64,"height":64}`)},
		{ID: "D", Type: "export", Version: 1, Config: json.RawMessage(`{"mode":"print","format":"png"}`)},
	}, Edges: []model.PODEdge{{ID: "BC", Source: "B", SourcePort: "images", Target: "C", TargetPort: "images"}, {ID: "CD", Source: "C", SourcePort: "images", Target: "D", TargetPort: "images"}}}
	var project model.PODProjectView
	p := call("POST", "/pod/projects", tokens[0], model.PODProjectCreate{Name: "Template source fixture"})
	check(p, 200)
	_ = json.Unmarshal(p["data"], &project)
	p = call("PUT", fmt.Sprint("/pod/projects/", project.ProjectID), tokens[0], model.PODProjectUpdate{Revision: strconv.FormatInt(project.Revision, 10), Graph: &graph})
	check(p, 200)
	_ = json.Unmarshal(p["data"], &project)
	body := model.PODTemplateSave{RequestID: uuid.NewString(), ProjectID: strconv.FormatInt(project.ProjectID, 10), ProjectRevision: strconv.FormatInt(project.Revision, 10), Name: "Personal print", Description: "A reusable fixture"}
	var original model.PODTemplateView
	t.Run("private_snapshot_and_idempotent_creation", func(t *testing.T) {
		check(call("GET", "/pod/templates", "", nil), 401)
		payload := call("POST", "/pod/templates", tokens[0], body)
		check(payload, 200)
		if json.Unmarshal(payload["data"], &original) != nil || original.TemplateID < 1 || original.CurrentVersion != 1 || original.Snapshot == nil || len(original.Snapshot.Slots) != 3 {
			t.Fatal("invalid template contract")
		}
		if strings.Contains(string(payload["data"]), "workspaceId") || strings.Contains(string(payload["data"]), "storageKey") {
			t.Fatal("internal ownership or file metadata leaked")
		}
		again := call("POST", "/pod/templates", tokens[0], body)
		check(again, 200)
		var same model.PODTemplateView
		_ = json.Unmarshal(again["data"], &same)
		if same.TemplateID != original.TemplateID {
			t.Fatal("duplicate template")
		}
		changed := body
		changed.Name = "changed request"
		check(call("POST", "/pod/templates", tokens[0], changed), 409)
		check(call("GET", fmt.Sprint("/pod/templates/", original.TemplateID), tokens[1], nil), 404)
		check(call("GET", fmt.Sprint("/pod/templates/", original.TemplateID, "/versions/1"), tokens[1], nil), 404)
		stolen := body
		stolen.RequestID = uuid.NewString()
		check(call("POST", "/pod/templates", tokens[1], stolen), 404)
		check(call("POST", "/pod/templates", tokens[0], map[string]any{"workspaceId": spaces[1]}), 400)
	})
	if original.TemplateID < 1 {
		t.Fatal("template creation failed")
	}
	path := fmt.Sprint("/pod/templates/", original.TemplateID)
	var used model.PODProjectView
	text := "replacement"
	ownImages := []string{assetIDs[0]}
	use := model.PODTemplateUse{RequestID: uuid.NewString(), Version: "1", Name: "Template instance", Bindings: []model.PODTemplateBinding{{NodeID: "A", Text: &text}, {NodeID: "B", AssetIDs: &ownImages}}}
	t.Run("slot_validation_private_assets_and_project_replay", func(t *testing.T) {
		bad := use
		bad.RequestID = uuid.NewString()
		foreign := []string{assetIDs[1]}
		bad.Bindings = []model.PODTemplateBinding{{NodeID: "B", AssetIDs: &foreign}}
		check(call("POST", path+"/use", tokens[0], bad), 400)
		bad.RequestID = uuid.NewString()
		bad.Bindings = []model.PODTemplateBinding{{NodeID: "C", Text: &text}}
		check(call("POST", path+"/use", tokens[0], bad), 400)
		bad.RequestID = uuid.NewString()
		bad.Bindings = []model.PODTemplateBinding{{NodeID: "A", Text: &text}, {NodeID: "A", Text: &text}}
		check(call("POST", path+"/use", tokens[0], bad), 400)
		bad.RequestID = uuid.NewString()
		bad.Version = "01"
		bad.Bindings = nil
		check(call("POST", path+"/use", tokens[0], bad), 400)
		check(call("POST", path+"/use", tokens[1], use), 404)
		p := call("POST", path+"/use", tokens[0], use)
		check(p, 200)
		_ = json.Unmarshal(p["data"], &used)
		if used.Graph == nil || !strings.Contains(string(used.Graph.Nodes[0].Config), text) || used.Revision != 1 {
			t.Fatal("slot bindings not copied into independent project")
		}
		p = call("POST", path+"/use", tokens[0], use)
		check(p, 200)
		var same model.PODProjectView
		_ = json.Unmarshal(p["data"], &same)
		if same.ProjectID != used.ProjectID {
			t.Fatal("duplicate project")
		}
		changed := use
		changed.Name = "changed request"
		check(call("POST", path+"/use", tokens[0], changed), 409)
	})
	t.Run("template_instance_runs_local_processing_and_private_download", func(t *testing.T) {
		body := model.PODRunCreate{ExecutionMode: "simulation", ProjectID: strconv.FormatInt(used.ProjectID, 10), Revision: strconv.FormatInt(used.Revision, 10), RequestID: uuid.NewString(), Mode: "workflow", TestScenario: "success"}
		payload := call("POST", "/pod/runs", tokens[0], body)
		check(payload, 200)
		var run model.PODRunView
		if err := json.Unmarshal(payload["data"], &run); err != nil || run.RunID < 1 {
			t.Fatal("template run was not created", err)
		}
		limits := service.PODExecutionLimits(config.PODExecutionConfig{Mode: "simulation"})
		for range 20 {
			more, err := service.ExecutePODStep(ctx, run.RunID, limits, executor.Simulation{}, &service.PODImageExecutor{Assets: assets})
			if err != nil {
				t.Fatal(err)
			}
			if !more {
				break
			}
		}
		payload = call("GET", fmt.Sprint("/pod/runs/", run.RunID), tokens[0], nil)
		check(payload, 200)
		if err := json.Unmarshal(payload["data"], &run); err != nil || run.State != model.PODRunSucceeded {
			t.Fatal("template image flow failed", err, run.State)
		}
		var exportID string
		for _, n := range run.Nodes {
			if n.NodeID == "D" && len(n.Output.AssetIDs) == 1 {
				exportID = n.Output.AssetIDs[0]
			}
		}
		if exportID == "" {
			t.Fatal("template flow did not archive an export")
		}
		download := func(token string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("GET", "/pod/assets/"+exportID+"/download", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			return rr
		}
		rr := download(tokens[0])
		decoded, format, err := image.DecodeConfig(rr.Body)
		if err != nil || rr.Code != 200 || format != "png" || decoded.Width != 64 || decoded.Height != 64 || !strings.Contains(rr.Header().Get("Content-Disposition"), "attachment") {
			t.Fatal("template export download invalid", err)
		}
		foreign := download(tokens[1])
		var denied map[string]json.RawMessage
		if err := json.Unmarshal(foreign.Body.Bytes(), &denied); err != nil {
			t.Fatal("foreign account received download bytes")
		}
		check(denied, 404)
	})
	t.Run("append_versions_are_immutable_and_old_projects_stay_fixed", func(t *testing.T) {
		graph.Nodes[0].Config = json.RawMessage(`{"text":"version-two"}`)
		graph.Nodes[1].Config = json.RawMessage(`{"assetIds":[]}`)
		graph.Nodes[2].Config = json.RawMessage(`{"assetIds":[]}`)
		p := call("PUT", fmt.Sprint("/pod/projects/", project.ProjectID), tokens[0], model.PODProjectUpdate{Revision: strconv.FormatInt(project.Revision, 10), Graph: &graph})
		check(p, 200)
		_ = json.Unmarshal(p["data"], &project)
		check(call("POST", "/pod/templates", tokens[0], body), 200)
		next := body
		next.RequestID = uuid.NewString()
		next.ProjectRevision = strconv.FormatInt(project.Revision, 10)
		next.Revision = strconv.FormatInt(original.Revision, 10)
		p = call("POST", path+"/versions", tokens[0], next)
		check(p, 200)
		_ = json.Unmarshal(p["data"], &original)
		if original.CurrentVersion != 2 || original.Snapshot == nil || original.Snapshot.Version != 2 {
			t.Fatal("new version missing")
		}
		check(call("POST", path+"/versions", tokens[0], next), 200)
		next.RequestID = uuid.NewString()
		check(call("POST", path+"/versions", tokens[0], next), 409)
		p = call("GET", path+"/versions/1", tokens[0], nil)
		check(p, 200)
		var old model.PODTemplateVersionView
		_ = json.Unmarshal(p["data"], &old)
		if old.Graph == nil || !strings.Contains(string(old.Graph.Nodes[0].Config), "original") {
			t.Fatal("old version changed")
		}
		p = call("GET", fmt.Sprint("/pod/projects/", used.ProjectID), tokens[0], nil)
		check(p, 200)
		var instance model.PODProjectView
		_ = json.Unmarshal(p["data"], &instance)
		if instance.Graph == nil || !strings.Contains(string(instance.Graph.Nodes[0].Config), "replacement") {
			t.Fatal("old project followed template update")
		}
		p = call("GET", path+"/versions?pageSize=1", tokens[0], nil)
		check(p, 200)
		var rows []model.PODTemplateVersionView
		_ = json.Unmarshal(p["rows"], &rows)
		if len(rows) != 1 || rows[0].Version != 2 || rows[0].Graph != nil {
			t.Fatal("version page leaked graph or ordering incorrect")
		}
	})
	t.Run("metadata_copy_trash_and_historical_reference_protection", func(t *testing.T) {
		update := model.PODTemplateUpdate{Revision: strconv.FormatInt(original.Revision, 10), Name: "Renamed template", Description: "new description"}
		p := call("PUT", path, tokens[0], update)
		check(p, 200)
		_ = json.Unmarshal(p["data"], &original)
		check(call("PUT", path, tokens[0], update), 409)
		copyBody := model.PODTemplateCopy{RequestID: uuid.NewString(), Version: "1", Name: "Independent copy"}
		p = call("POST", path+"/copy", tokens[0], copyBody)
		check(p, 200)
		var copied model.PODTemplateView
		_ = json.Unmarshal(p["data"], &copied)
		if copied.TemplateID == original.TemplateID || copied.CurrentVersion != 1 || copied.Snapshot == nil || !strings.Contains(string(copied.Snapshot.Graph.Nodes[0].Config), "original") {
			t.Fatal("copy did not freeze selected version")
		}
		check(call("POST", path+"/copy", tokens[0], copyBody), 200)
		p = call("DELETE", path, tokens[0], model.PODProjectVersion{Revision: strconv.FormatInt(original.Revision, 10)})
		check(p, 200)
		_ = json.Unmarshal(p["data"], &original)
		nextUse := use
		nextUse.RequestID = uuid.NewString()
		check(call("POST", path+"/use", tokens[0], nextUse), 404)
		check(call("POST", path+"/use", tokens[0], use), 200)
		check(call("POST", path+"/copy", tokens[0], model.PODTemplateCopy{RequestID: uuid.NewString(), Version: "1", Name: "trash copy"}), 404)
		check(call("POST", path+"/restore", tokens[1], model.PODProjectVersion{Revision: strconv.FormatInt(original.Revision, 10)}), 404)
		p = call("POST", path+"/restore", tokens[0], model.PODProjectVersion{Revision: strconv.FormatInt(original.Revision, 10)})
		check(p, 200)
		_ = json.Unmarshal(p["data"], &original)
		// Remove the instance's references so only historical template versions retain this image.
		empty := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}}
		check(call("PUT", fmt.Sprint("/pod/projects/", used.ProjectID), tokens[0], model.PODProjectUpdate{Revision: strconv.FormatInt(used.Revision, 10), Graph: &empty}), 200)
		assetPath := "/pod/assets/" + assetIDs[0]
		p = call("GET", assetPath, tokens[0], nil)
		check(p, 200)
		var asset model.PODAssetView
		_ = json.Unmarshal(p["data"], &asset)
		if asset.ReferenceCount < 4 {
			t.Fatal("template history references not counted")
		}
		check(call("DELETE", assetPath, tokens[0], model.PODProjectVersion{Revision: strconv.FormatInt(asset.Revision, 10)}), 409)
	})
	t.Run("pagination_cap_and_stable_ties", func(t *testing.T) {
		for i := 0; i < 101; i++ {
			check(call("POST", path+"/copy", tokens[0], model.PODTemplateCopy{RequestID: uuid.NewString(), Version: "2", Name: fmt.Sprintf("Page fixture %03d", i)}), 200)
		}
		if err := repository.DB(ctx).Table("pod_template").Where("workspace_id=?", spaces[0]).Update("update_time", "2026-11-12 12:00:00").Error; err != nil {
			t.Fatal(err)
		}
		p := call("GET", "/pod/templates?pageNum=1&pageSize=1000", tokens[0], nil)
		check(p, 200)
		var one []model.PODTemplateView
		_ = json.Unmarshal(p["rows"], &one)
		if len(one) != 100 {
			t.Fatal("page size not capped")
		}
		p = call("GET", "/pod/templates?pageNum=2&pageSize=100", tokens[0], nil)
		check(p, 200)
		var two []model.PODTemplateView
		_ = json.Unmarshal(p["rows"], &two)
		seen := map[int64]bool{}
		for _, r := range one {
			if r.Snapshot != nil {
				t.Fatal("list returned graph")
			}
			seen[r.TemplateID] = true
		}
		for _, r := range two {
			if seen[r.TemplateID] {
				t.Fatal("unstable tie pagination")
			}
		}
		p = call("GET", "/pod/templates", tokens[1], nil)
		check(p, 200)
		var total int
		_ = json.Unmarshal(p["total"], &total)
		if total != 0 {
			t.Fatal("cross-account list exposed templates")
		}
	})
	t.Run("revoked_member_denied_and_concurrent_request_is_single", func(t *testing.T) {
		svc := service.PODTemplateService{}
		req := model.PODTemplateCopy{RequestID: uuid.NewString(), Version: "2", Name: "Concurrent fixture"}
		ids := make(chan int64, 2)
		failures := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := svc.Copy(ctx, &users[0], original.TemplateID, req)
				if err != nil {
					failures <- err
					return
				}
				ids <- r.TemplateID
			}()
		}
		wg.Wait()
		close(ids)
		close(failures)
		for err := range failures {
			t.Error(err)
		}
		var first int64
		for id := range ids {
			if first != 0 && first != id {
				t.Fatal("concurrent duplicate template")
			}
			first = id
		}
		if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=? AND user_id=?", spaces[0], users[0].UserID).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		check(call("GET", path, tokens[0], nil), 403)
		if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id=? AND user_id=?", spaces[0], users[0].UserID).Update("status", "0").Error; err != nil {
			t.Fatal(err)
		}
	})
	var paidAfter int64
	if err := repository.DB(ctx).Table("pod_provider_call").Count(&paidAfter).Error; err != nil {
		t.Fatal(err)
	}
	if paidAfter != paidBefore {
		t.Fatal("template operation created paid call")
	}
}
