package podtest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
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
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/storage"
)

func exercisePODBatches(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage) {
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
		for _, query := range []string{"DELETE f FROM pod_asset_file f JOIN pod_asset a ON a.asset_id=f.asset_id WHERE a.workspace_id IN ?", "DELETE o FROM pod_outbox o JOIN pod_run r ON r.run_id=o.run_id WHERE r.workspace_id IN ?"} {
			if err := repository.DB(clean).Exec(query, spaces).Error; err != nil {
				t.Error(err)
			}
		}
		for _, table := range []string{"pod_batch_asset_ref", "pod_batch_item", "pod_batch", "pod_template_action", "pod_template_asset_ref", "pod_template_version", "pod_template", "pod_provider_asset", "pod_provider_call", "pod_run_asset_ref", "pod_node_run", "pod_retry_request", "pod_credit_event", "pod_credit_account", "pod_run", "pod_project_asset_ref", "pod_project", "pod_asset", "pod_workspace_member", "pod_workspace"} {
			if err := repository.DB(clean).Table(table).Where("workspace_id IN ?", spaces).Delete(map[string]any{}).Error; err != nil {
				t.Error(table, err)
			}
		}
		for _, u := range users {
			repository.DB(clean).Table("sys_user").Where("user_id=?", u.UserID).Delete(&model.SysUser{})
			redisx.C().Del(clean, redisx.LoginUserSessionsKey(u.UserID), redisx.LoginUserGenerationKey(u.UserID), redisx.LoginUserPermissionVersionKey(u.UserID))
		}
	})
	for range 2 {
		u := model.SysUser{UserName: "podbatch_" + uuid.NewString()[:8], NickName: "Batch fixture", Status: "0", DelFlag: "0"}
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
	assets := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{})
	var raw bytes.Buffer
	_ = png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	asset, err := assets.Create(ctx, &users[0], "batch.png", "image/png", &raw)
	if err != nil {
		t.Fatal(err)
	}
	assetID := strconv.FormatInt(asset.AssetID, 10)
	g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
		{ID: "T", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"original"}`)},
		{ID: "I", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + assetID + `"]}`)},
		{ID: "P", Type: "imageProcess", Version: 1, Config: json.RawMessage(`{"operation":"resize","width":64,"height":64}`)},
		{ID: "E", Type: "export", Version: 1, Config: json.RawMessage(`{"mode":"print","format":"png"}`)},
		{ID: "M", Type: "productPreview", Version: 1, Config: json.RawMessage(`{}`)},
	}, Edges: []model.PODEdge{{ID: "IP", Source: "I", SourcePort: "images", Target: "P", TargetPort: "images"}, {ID: "PE", Source: "P", SourcePort: "images", Target: "E", TargetPort: "images"}, {ID: "PM", Source: "P", SourcePort: "images", Target: "M", TargetPort: "images"}}}
	project, err := service.CreatePODProject(ctx, &users[0], model.PODProjectCreate{Name: "Batch source"})
	if err != nil {
		t.Fatal(err)
	}
	project, err = service.EditPODProject(ctx, &users[0], project.ProjectID, model.PODProjectUpdate{Revision: fmt.Sprint(project.Revision), Graph: &g}, model.PODGraphLimits{})
	if err != nil {
		t.Fatal(err)
	}
	templates := service.PODTemplateService{}
	template, err := templates.Save(ctx, &users[0], 0, model.PODTemplateSave{RequestID: uuid.NewString(), ProjectID: fmt.Sprint(project.ProjectID), ProjectRevision: fmt.Sprint(project.Revision), Name: "Batch template"})
	if err != nil {
		t.Fatal(err)
	}
	limits := service.PODExecutionLimits(cfg.POD.Execution)
	batches := service.PODBatchService{Runs: service.PODRunService{Limits: limits}, Assets: assets}
	draft := func() model.PODBatchCreate {
		return model.PODBatchCreate{RequestID: uuid.NewString(), TemplateID: fmt.Sprint(template.TemplateID), Version: "1", Name: "Bounded batch", ExecutionMode: limits.Mode, Items: []model.PODBatchInput{{Name: "First", Bindings: []model.PODTemplateBinding{{NodeID: "T", Text: ptrBatch("changed")}}}, {Name: "Second"}}}
	}
	var batch *model.PODBatchView
	t.Run("preview_bounded_inputs_and_privacy", func(t *testing.T) {
		body := draft()
		v, err := batches.Preview(ctx, &users[0], body)
		if err != nil || v.Items != 2 || v.TextCalls != 0 || v.ReservationMicros != 0 {
			t.Fatalf("preview %+v %v", v, err)
		}
		check := func(p map[string]json.RawMessage, want int) {
			var code int
			_ = json.Unmarshal(p["code"], &code)
			if code != want {
				t.Fatalf("code %d want %d: %s", code, want, p["msg"])
			}
		}
		check(call("POST", "/pod/batches/preview", "", body), 401)
		check(call("POST", "/pod/batches", tokens[1], body), 404)
		bad := draft()
		bad.Items = make([]model.PODBatchInput, 11)
		check(call("POST", "/pod/batches", tokens[0], bad), 400)
		bad = draft()
		bad.ExecutionMode = "qwen"
		if limits.Mode == "qwen" {
			bad.ExecutionMode = "simulation"
		}
		check(call("POST", "/pod/batches", tokens[0], bad), 409)
		p := call("POST", "/pod/batches", tokens[0], body)
		var responseCode int
		_ = json.Unmarshal(p["code"], &responseCode)
		if responseCode != 200 {
			_, directErr := batches.Create(ctx, &users[0], body)
			t.Fatalf("batch creation failed: %v", directErr)
		}
		check(p, 200)
		var created model.PODBatchView
		_ = json.Unmarshal(p["data"], &created)
		batch = &created
		if strings.Contains(string(p["data"]), "graphJSON") || strings.Contains(string(p["data"]), "workspaceId") {
			t.Fatal("private metadata leaked")
		}
		check(call("GET", fmt.Sprint("/pod/batches/", batch.BatchID), tokens[1], nil), 404)
		check(call("POST", fmt.Sprint("/pod/batches/", batch.BatchID, "/stop"), tokens[1], map[string]any{}), 404)
		var itemRows []model.PODBatchItem
		if err := repository.DB(ctx).Where("batch_id=?", batch.BatchID).Order("item_no").Find(&itemRows).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(itemRows[0].GraphJSON), "changed") || !strings.Contains(string(itemRows[1].GraphJSON), "original") {
			t.Fatal("binding contaminated the next item's immutable snapshot")
		}
		again, err := batches.Create(ctx, &users[0], body)
		if err != nil || again.BatchID != batch.BatchID {
			t.Fatal("duplicate create", err)
		}
		body.Name = "different"
		if _, err = batches.Create(ctx, &users[0], body); err == nil {
			t.Fatal("request reuse accepted changed payload")
		}
	})
	if batch == nil {
		t.Fatal("batch setup failed")
	}
	t.Run("atomic_dispatch_stop_and_zip", func(t *testing.T) {
		ok, err := repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{})
		if err != nil || !ok {
			t.Fatal("dispatch", ok, err)
		}
		stopped, err := batches.Stop(ctx, &users[0], batch.BatchID)
		if err != nil {
			t.Fatal(err)
		}
		if stopped.Items[0].RunID == 0 || stopped.Items[1].State != "cancelled" {
			t.Fatalf("stop damaged dispatched item %+v", stopped.Items)
		}
		runID := stopped.Items[0].RunID
		imageExecutor := &service.PODImageExecutor{Assets: assets}
		for range 12 {
			more, err := service.ExecutePODStep(ctx, runID, limits, executor.Simulation{}, imageExecutor)
			if err != nil {
				t.Fatal(err)
			}
			if !more {
				break
			}
		}
		run, _, err := repository.SelectPODRun(ctx, users[0].UserID, runID)
		if err != nil || run.State != "succeeded" {
			t.Fatal("batch run failed", run, err)
		}
		file, err := batches.ZIP(ctx, &users[0], batch.BatchID)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file.Name())
		defer file.Close()
		info, _ := file.Stat()
		archive, err := zip.NewReader(file, info.Size())
		if err != nil {
			t.Fatal(err)
		}
		printFound, mockupFound := false, false
		for _, f := range archive.File {
			if strings.Contains(f.Name, "/print/") {
				printFound = true
			}
			if strings.Contains(f.Name, "/mockup/") {
				mockupFound = true
			}
			if strings.Contains(f.Name, "..") || strings.Contains(f.Name, ":") {
				t.Fatal("unsafe ZIP path")
			}
		}
		if !printFound || !mockupFound {
			t.Fatal("ZIP lost print/mockup separation")
		}
		if _, err = batches.ZIP(ctx, &users[1], batch.BatchID); err == nil {
			t.Fatal("cross-user ZIP accepted")
		}
		before := int64(0)
		repository.DB(ctx).Model(&model.PODRun{}).Where("workspace_id=?", spaces[0]).Count(&before)
		if _, err = repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{}); err != nil {
			t.Fatal(err)
		}
		after := int64(0)
		repository.DB(ctx).Model(&model.PODRun{}).Where("workspace_id=?", spaces[0]).Count(&after)
		if before != after {
			t.Fatal("stopped item was dispatched")
		}
	})
	t.Run("concurrent_creation_and_dispatch_recovery", func(t *testing.T) {
		body := draft()
		var wg sync.WaitGroup
		ids := make(chan int64, 4)
		failures := make(chan error, 4)
		for range 4 {
			wg.Go(func() {
				v, err := batches.Create(ctx, &users[0], body)
				if err != nil {
					failures <- err
				} else {
					ids <- v.BatchID
				}
			})
		}
		wg.Wait()
		close(ids)
		close(failures)
		for err := range failures {
			t.Fatal(err)
		}
		var id int64
		for got := range ids {
			if id != 0 && id != got {
				t.Fatal("duplicate batch")
			}
			id = got
		}
		for range 4 {
			wg.Go(func() {
				_, err := repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{})
				if err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		// Concurrent ticks may all observe the same candidate before taking the gate.
		// Later ticks, including after a worker restart, must finish the remaining items.
		for range 2 {
			if _, err := repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{}); err != nil {
				t.Fatal(err)
			}
		}
		v, err := batches.Get(ctx, &users[0], id)
		if err != nil {
			t.Fatal(err)
		}
		unique := map[int64]bool{}
		for _, item := range v.Items {
			if item.RunID == 0 || unique[item.RunID] {
				t.Fatal("run not atomically assigned")
			}
			unique[item.RunID] = true
		}
		var runs int64
		repository.DB(ctx).Model(&model.PODRun{}).Where("run_id IN ?", []int64{v.Items[0].RunID, v.Items[1].RunID}).Count(&runs)
		if runs != 2 {
			t.Fatal("duplicate/lost run")
		}
		for _, item := range v.Items {
			if _, err := (service.PODRunService{Limits: limits}).Change(ctx, &users[0], item.RunID, ""); err != nil {
				t.Fatal(err)
			}
			_, _ = service.ExecutePODStep(ctx, item.RunID, limits, executor.Simulation{}, &service.PODImageExecutor{Assets: assets})
		}
	})
	t.Run("owner_waiting_capacity", func(t *testing.T) {
		ids := []int64{}
		defer func() {
			for _, id := range ids {
				if _, err := batches.Stop(ctx, &users[0], id); err != nil {
					t.Error(err)
				}
			}
		}()
		for range 1 {
			body := draft()
			for len(body.Items) < 10 {
				body.Items = append(body.Items, body.Items[0])
			}
			v, err := batches.Create(ctx, &users[0], body)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, v.BatchID)
		}
		body := draft()
		body.Items = body.Items[:1]
		if _, err := batches.Create(ctx, &users[0], body); err == nil {
			t.Fatal("accepted the 11th waiting item")
		}
		var waiting int64
		if err := repository.DB(ctx).Table("pod_batch_item").Where("workspace_id=? AND state='waiting'", spaces[0]).Count(&waiting).Error; err != nil || waiting != 10 {
			t.Fatalf("waiting=%d, err=%v", waiting, err)
		}
		if _, err := batches.Stop(ctx, &users[0], ids[0]); err != nil {
			t.Fatal(err)
		}
		v, err := batches.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal("stopping did not release waiting capacity", err)
		}
		ids = append(ids, v.BatchID)
	})
	t.Run("disabled_space_and_pagination", func(t *testing.T) {
		body := draft()
		body.Items = body.Items[:1]
		v, err := batches.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal(err)
		}
		if err = repository.DB(ctx).Table("pod_workspace").Where("workspace_id=?", spaces[0]).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Table("pod_workspace").Where("workspace_id=?", spaces[0]).Update("status", "0")
		_, err = repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		var item model.PODBatchItem
		repository.DB(ctx).Where("batch_id=?", v.BatchID).Take(&item)
		if item.RunID != 0 {
			t.Fatal("disabled workspace dispatched")
		}
		repository.DB(ctx).Table("pod_workspace").Where("workspace_id=?", spaces[0]).Update("status", "0")
		if _, err = batches.Stop(ctx, &users[0], v.BatchID); err != nil {
			t.Fatal(err)
		}
		rows, total, err := batches.List(ctx, &users[0], page.Query{PageNum: 1, PageSize: 1000})
		if err != nil || total < 3 || len(rows) > 100 {
			t.Fatal("pagination", err)
		}
	})
}
func ptrBatch(s string) *string { return &s }
