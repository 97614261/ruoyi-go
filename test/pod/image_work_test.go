package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/storage"
)

type imageFailStore struct {
	storage.Store
	puts int
}

func (s *imageFailStore) Put(ctx context.Context, ext string, data []byte) (string, error) {
	s.puts++
	if s.puts == 3 {
		return "", errors.New("injected storage failure")
	}
	return s.Store.Put(ctx, ext, data)
}

func exercisePODImageWork(t *testing.T, ctx context.Context, cfg *config.Config) {
	var users []model.SysUser
	var spaces []int64
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupCreatorFixtures(t, clean, spaces)
		var files []model.PODAssetFile
		if err := repository.DB(clean).Table("pod_asset_file f").Select("f.*").Joins("JOIN pod_asset a ON a.asset_id=f.asset_id").Where("a.workspace_id IN ?", spaces).Find(&files).Error; err != nil {
			t.Error(err)
		}
		for _, file := range files {
			_ = (storage.Local{Path: cfg.POD.StorageRoot}).Remove(clean, file.StorageKey)
		}
		if err := repository.DB(clean).Exec("DELETE f FROM pod_asset_file f JOIN pod_asset a ON a.asset_id=f.asset_id WHERE a.workspace_id IN ?", spaces).Error; err != nil {
			t.Error(err)
		}
		if err := repository.DB(clean).Exec("DELETE o FROM pod_outbox o JOIN pod_run r ON r.run_id=o.run_id WHERE r.workspace_id IN ?", spaces).Error; err != nil {
			t.Error(err)
		}
		for _, table := range []string{"pod_provider_asset", "pod_provider_call", "pod_run_asset_ref", "pod_node_run", "pod_retry_request", "pod_credit_event", "pod_credit_account", "pod_run", "pod_project_asset_ref", "pod_project", "pod_asset", "pod_workspace_member", "pod_workspace"} {
			if err := repository.DB(clean).Table(table).Where("workspace_id IN ?", spaces).Delete(map[string]any{}).Error; err != nil {
				t.Error(err)
			}
		}
		for _, u := range users {
			if err := repository.DB(clean).Table("sys_user").Where("user_id=?", u.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for range 2 {
		u := model.SysUser{UserName: "podimg_" + uuid.NewString()[:8], NickName: "Image fixture", Status: "0", DelFlag: "0"}
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
	assets := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{})
	ids := []string{}
	for index := range 2 {
		src := image.NewNRGBA(image.Rect(0, 0, 256, 128))
		for y := 16; y < 112; y++ {
			for x := 16; x < 240; x++ {
				src.SetNRGBA(x, y, color.NRGBA{R: uint8(100 + index*80), G: 80, B: 200, A: 200})
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, src)
		asset, err := assets.Create(ctx, &users[0], "fixture.png", "image/png", &b)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, strconv.FormatInt(asset.AssetID, 10))
	}
	limits := service.PODExecutionLimits(config.PODExecutionConfig{Mode: "qwen"})
	svc := service.PODRunService{Limits: limits}
	provider := &service.PODQwenExecutor{Assets: assets}
	node := func(id, kind, config string) model.PODNode {
		return model.PODNode{ID: id, Type: kind, Version: 1, Config: json.RawMessage(config)}
	}
	edge := func(from, to string) model.PODEdge {
		return model.PODEdge{ID: from + to, Source: from, SourcePort: "images", Target: to, TargetPort: "images"}
	}
	makeRun := func(g model.PODGraph) (*model.PODRunView, model.PODRunCreate) {
		t.Helper()
		p, err := service.CreatePODProject(ctx, &users[0], model.PODProjectCreate{Name: "Image stage6 fixture"})
		if err != nil {
			t.Fatal(err)
		}
		p, err = service.EditPODProject(ctx, &users[0], p.ProjectID, model.PODProjectUpdate{Name: &p.Name, Revision: strconv.FormatInt(p.Revision, 10), Graph: &g}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		body := model.PODRunCreate{ExecutionMode: "qwen", ProjectID: strconv.FormatInt(p.ProjectID, 10), Revision: strconv.FormatInt(p.Revision, 10), RequestID: uuid.NewString(), Mode: "workflow"}
		r, err := svc.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal(err)
		}
		return r, body
	}
	drain := func(id int64) *model.PODRunView {
		t.Helper()
		for range 20 {
			more, err := service.ExecutePODQwenStep(ctx, id, limits, provider)
			if err != nil {
				t.Fatal(err)
			}
			if !more {
				break
			}
		}
		r, err := svc.Get(ctx, &users[0], id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	input := node("A", "imageInput", `{"assetIds":["`+ids[0]+`","`+ids[1]+`"]}`)
	t.Run("artwork_mockup_and_export_are_separate_private_assets", func(t *testing.T) {
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{input, node("B", "imageProcess", `{"operation":"resize","width":128,"height":128}`), node("C", "productPreview", `{"side":"back","color":"#122334","printX":1,"printY":1,"printScale":0.9}`), node("D", "export", `{"mode":"mockup","format":"jpeg","fileName":"fixture"}`), node("E", "export", `{"mode":"print","format":"png","fileName":"print"}`)}, Edges: []model.PODEdge{edge("A", "B"), edge("B", "C"), edge("C", "D"), edge("B", "E")}}
		r, body := makeRun(g)
		r = drain(r.RunID)
		if r.State != model.PODRunSucceeded {
			t.Fatalf("run state %s nodes %+v", r.State, r.Nodes)
		}
		for _, n := range r.Nodes {
			if len(n.Output.AssetIDs) != 2 {
				t.Fatalf("multi-image lost: %s", n.NodeID)
			}
			if n.NodeID == "A" {
				continue
			}
			for _, info := range n.Output.Images {
				if info.Kind == "mockup" && (info.Width != 1000 || info.Height != 1200) {
					t.Fatal("mockup size", info)
				}
				if n.NodeID == "E" && (info.DPI >= 150 || info.Warning == "" || info.Kind != "print") {
					t.Fatal("print DPI check", info)
				}
				aid, _ := strconv.ParseInt(info.AssetID, 10, 64)
				meta, err := service.GetPODAsset(ctx, &users[0], aid)
				if err != nil || meta.ParentAssetID < 1 {
					t.Fatal("lineage lost", err)
				}
				file, _, _, err := assets.Open(ctx, &users[0], aid, "original")
				if err != nil {
					t.Fatal(err)
				}
				_, format, err := image.DecodeConfig(file)
				_ = file.Close()
				if err != nil || (n.NodeID == "D" && format != "jpeg") {
					t.Fatal("export format", format, err)
				}
				if _, err = service.GetPODAsset(ctx, &users[1], aid); err == nil {
					t.Fatal("cross-account metadata exposed")
				}
				if f, _, _, err := assets.Open(ctx, &users[1], aid, "original"); err == nil {
					f.Close()
					t.Fatal("cross-account download exposed")
				}
			}
		}
		before := int64(0)
		repository.DB(ctx).Table("pod_asset").Where("workspace_id=?", spaces[0]).Count(&before)
		again, err := svc.Create(ctx, &users[0], body)
		if err != nil || again.RunID != r.RunID {
			t.Fatal("idempotency", err)
		}
		_ = drain(r.RunID)
		after := int64(0)
		repository.DB(ctx).Table("pod_asset").Where("workspace_id=?", spaces[0]).Count(&after)
		if before != after {
			t.Fatal("duplicate assets on replay")
		}
		// Selecting a saved mockup as a fresh image input cannot launder it into printable artwork.
		mockup := r.Nodes[2].Output.AssetIDs[0]
		g = model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{node("A", "imageInput", `{"assetIds":["`+mockup+`"]}`), node("B", "imageProcess", `{"width":256,"height":256}`), node("C", "export", `{"mode":"print"}`)}, Edges: []model.PODEdge{edge("A", "B"), edge("B", "C")}}
		bad, _ := makeRun(g)
		bad = drain(bad.RunID)
		if bad.State != model.PODRunFailed || bad.Nodes[2].ErrorCode != "exportKind" {
			t.Fatal("mockup provenance erased", bad.State, bad.Nodes)
		}
		var calls int64
		repository.DB(ctx).Table("pod_provider_call").Where("workspace_id=?", spaces[0]).Count(&calls)
		if calls != 0 {
			t.Fatal("local image path made paid calls")
		}
	})
	t.Run("partial_archive_resumes_without_duplicate_first_image", func(t *testing.T) {
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{input, node("B", "imageProcess", `{"operation":"resize","width":64,"height":64}`)}, Edges: []model.PODEdge{edge("A", "B")}}
		r, _ := makeRun(g)
		if _, err := service.ExecutePODQwenStep(ctx, r.RunID, limits, provider); err != nil {
			t.Fatal(err)
		}
		fault := &imageFailStore{Store: assets.Store}
		provider.Assets = &service.PODAssetService{Store: fault, Limits: assets.Limits}
		if _, err := service.ExecutePODQwenStep(ctx, r.RunID, limits, provider); err != nil {
			t.Fatal(err)
		}
		rows := []model.PODNodeRun{}
		repository.DB(ctx).Where("run_id=? AND node_id='B'", r.RunID).Find(&rows)
		if len(rows) != 1 || rows[0].State != "awaitingReconciliation" {
			t.Fatal("archive not pending", rows)
		}
		first, err := repository.SelectPODProviderAssets(ctx, rows[0].SubmissionKey, spaces[0])
		if err != nil || len(first) != 1 {
			t.Fatal("first archive missing", err)
		}
		provider.Assets = assets
		r = drain(r.RunID)
		if r.State != model.PODRunSucceeded {
			t.Fatal("resume failed", r.State)
		}
		after, err := repository.SelectPODProviderAssets(ctx, rows[0].SubmissionKey, spaces[0])
		if err != nil || len(after) != 2 || after[0] != first[0] {
			t.Fatal("first image duplicated", err)
		}
	})
	t.Run("real_cpu_models_through_run_and_private_archive", func(t *testing.T) {
		url := os.Getenv("POD_IMAGE_AI_TEST_URL")
		if url == "" {
			t.Skip("set POD_IMAGE_AI_TEST_URL to an already running local CPU service")
		}
		limits.ImageAIURL = url
		limits.ImageAIEnabled = true
		svc.Limits = limits
		provider.ImageAIURL = url
		defer func() {
			limits.ImageAIURL = ""
			limits.ImageAIEnabled = false
			svc.Limits = limits
			provider.ImageAIURL = ""
		}()
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{input, node("B", "imageProcess", `{"operation":"removeBackground","method":"ai"}`), node("C", "imageProcess", `{"operation":"upscale","method":"ai","scale":2}`), node("D", "productPreview", `{}`), node("E", "export", `{"mode":"mockup"}`)}, Edges: []model.PODEdge{edge("A", "B"), edge("B", "C"), edge("C", "D"), edge("D", "E")}}
		r, _ := makeRun(g)
		r = drain(r.RunID)
		if r.State != model.PODRunSucceeded {
			t.Fatalf("real CPU chain: %s %+v", r.State, r.Nodes)
		}
		if r.Nodes[2].Output.Images[0].Width != 512 || r.Nodes[2].Output.Images[0].Height != 256 {
			t.Fatal("AI output size incorrect")
		}
	})
}
