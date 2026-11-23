package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
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
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/storage"
)

type podQwenTransport func(*http.Request) (*http.Response, error)

func (f podQwenTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func exercisePODQwen(t *testing.T, ctx context.Context, cfg *config.Config) {
	var users []model.SysUser
	var spaces []int64

	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupCreatorFixtures(t, cleanup, spaces)
		var ids []int64
		if err := repository.DB(cleanup).Table("pod_asset").Where("workspace_id IN ?", spaces).Pluck("asset_id", &ids).Error; err != nil {
			t.Error(err)
		}
		if len(ids) > 0 {
			var files []model.PODAssetFile
			if err := repository.DB(cleanup).Table("pod_asset_file").Where("asset_id IN ?", ids).Find(&files).Error; err != nil {
				t.Error(err)
			}
			for _, f := range files {
				if err := (storage.Local{Path: cfg.POD.StorageRoot}).Remove(cleanup, f.StorageKey); err != nil {
					t.Error(err)
				}
			}
			if err := repository.DB(cleanup).Table("pod_asset_file").Where("asset_id IN ?", ids).Delete(&model.PODAssetFile{}).Error; err != nil {
				t.Error(err)
			}
		}
		if err := repository.DB(cleanup).Exec("DELETE o FROM pod_outbox o JOIN pod_run r ON r.run_id=o.run_id WHERE r.workspace_id IN ?", spaces).Error; err != nil {
			t.Error(err)
		}
		for _, table := range []string{"pod_provider_asset", "pod_provider_call", "pod_run_asset_ref", "pod_node_run", "pod_retry_request", "pod_credit_event", "pod_credit_account", "pod_run", "pod_project_asset_ref", "pod_project", "pod_asset", "pod_workspace_member", "pod_workspace"} {
			if err := repository.DB(cleanup).Table(table).Where("workspace_id IN ?", spaces).Delete(map[string]any{}).Error; err != nil {
				t.Error(table, err)
			}
		}
		for _, u := range users {
			if err := repository.DB(cleanup).Table("sys_user").Where("user_id=?", u.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for i := 0; i < 2; i++ {
		hash, err := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		u := model.SysUser{UserName: "podqwen_" + uuid.NewString()[:8], NickName: "Qwen fixture", Password: string(hash), Status: "0", DelFlag: "0"}
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
	limits := service.PODExecutionLimits(config.PODExecutionConfig{Mode: "qwen"})
	svc := service.PODRunService{Limits: limits}
	makeRun := func(t *testing.T, kind string, extra ...model.PODNode) *model.PODRunView {
		t.Helper()
		p, err := service.CreatePODProject(ctx, &users[0], model.PODProjectCreate{Name: "Qwen local HTTP fixture"})
		if err != nil {
			t.Fatal(err)
		}
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"a cat print"}`)}, {ID: "B", Type: kind, Version: 1, Config: json.RawMessage(`{}`)}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
		g.Nodes = append(g.Nodes, extra...)
		for _, node := range extra {
			if node.Type == "imageGenerate" || node.Type == "textGenerate" {
				g.Edges = append(g.Edges, model.PODEdge{ID: "A" + node.ID, Source: "A", SourcePort: "text", Target: node.ID, TargetPort: "text"})
			}
		}
		p, err = service.EditPODProject(ctx, &users[0], p.ProjectID, model.PODProjectUpdate{Revision: "1", Graph: &g}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		r, err := svc.Create(ctx, &users[0], model.PODRunCreate{ExecutionMode: "qwen", ProjectID: strconv.FormatInt(p.ProjectID, 10), Revision: strconv.FormatInt(p.Revision, 10), Mode: "workflow", RequestID: uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	response := func(body string) *http.Response {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	}
	provider := func(t *testing.T, fn podQwenTransport) *service.PODQwenExecutor {
		p, err := executor.NewQwen("https://dashscope.aliyuncs.com", "local-fixture-not-an-api-key", &http.Client{Transport: fn})
		if err != nil {
			t.Fatal(err)
		}
		return &service.PODQwenExecutor{Provider: p, Images: &http.Client{Transport: fn}, Assets: service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{}), BudgetFen: 5000}
	}
	step := func(t *testing.T, id int64, p *service.PODQwenExecutor) *model.PODRunView {
		t.Helper()
		if _, err := service.ExecutePODQwenStep(ctx, id, limits, p); err != nil {
			t.Fatal(err)
		}
		r, err := svc.Get(ctx, &users[0], id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	usageForRun := func(t *testing.T, id int64) model.PODUsageRun {
		t.Helper()
		v, err := svc.Usage(ctx, &users[0], page.Query{PageNum: 1, PageSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range v.Rows {
			if row.RunID == id {
				return row
			}
		}
		t.Fatal("usage run missing")
		return model.PODUsageRun{}
	}
	t.Run("inline_polish_and_branch_keep_canvas_and_idempotency", func(t *testing.T) {
		project, err := service.CreatePODProject(ctx, &users[0], model.PODProjectCreate{Name: "Inline action fixture"})
		if err != nil {
			t.Fatal(err)
		}
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
			{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"blue flowers"}`)},
			{ID: "B", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{"instruction":"one prompt"}`)},
			{ID: "unrelated", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":[]}`)},
		}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
		project, err = service.EditPODProject(ctx, &users[0], project.ProjectID, model.PODProjectUpdate{Revision: "1", Graph: &g}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		body := model.PODRunCreate{ExecutionMode: "qwen", ProjectID: strconv.FormatInt(project.ProjectID, 10), Revision: strconv.FormatInt(project.Revision, 10), Mode: "polish", NodeID: "A", RequestID: uuid.NewString()}
		r, err := svc.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal(err)
		}
		repeated, err := svc.Create(ctx, &users[0], body)
		if err != nil || repeated.RunID != r.RunID {
			t.Fatal("duplicate inline request created another run")
		}
		if _, err := svc.Create(ctx, &users[1], body); err == nil {
			t.Fatal("cross-owner inline action accepted")
		}
		bad := body
		bad.RequestID = uuid.NewString()
		bad.NodeID = "B"
		if _, err := svc.Create(ctx, &users[0], bad); err == nil {
			t.Fatal("polish accepted non-input node")
		}
		bad = body
		bad.RequestID = uuid.NewString()
		bad.BaseRunID = strconv.FormatInt(r.RunID, 10)
		if _, err := svc.Create(ctx, &users[0], bad); err == nil {
			t.Fatal("polish accepted historical upstream")
		}
		bad = body
		bad.RequestID = uuid.NewString()
		bad.Revision = "1"
		if _, err := svc.Create(ctx, &users[0], bad); err == nil {
			t.Fatal("polish accepted stale canvas revision")
		}
		calls := 0
		p := provider(t, func(*http.Request) (*http.Response, error) {
			calls++
			return response(`{"id":"inline-fixture","choices":[{"finish_reason":"stop","message":{"content":"polished blue flowers"}}],"usage":{"prompt_tokens":30,"completion_tokens":10}}`), nil
		})
		for i := 0; i < 4 && !workflow.RunTerminal(r.State); i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunSucceeded || r.Mode != "polish" || r.TargetNodeID != "A" || calls != 1 || r.Nodes[1].Output.Text != "polished blue flowers" {
			t.Fatal("inline polish did not use one durable text call")
		}
		var stored model.PODProject
		if err := repository.DB(ctx).Table("pod_project").Where("project_id=?", project.ProjectID).Take(&stored).Error; err != nil {
			t.Fatal(err)
		}
		var unchanged model.PODGraph
		var unchangedConfig map[string]any
		if json.Unmarshal(stored.GraphJSON, &unchanged) != nil || stored.Revision != project.Revision || len(unchanged.Nodes) != 3 || unchanged.Nodes[0].Type != "textInput" {
			t.Fatal("polish changed the stored canvas")
		}
		if json.Unmarshal(unchanged.Nodes[0].Config, &unchangedConfig) != nil || unchangedConfig["text"] != "blue flowers" {
			t.Fatal("polish changed the original text")
		}
		bad = body
		bad.RequestID = uuid.NewString()
		bad.Mode = "node"
		bad.NodeID = "B"
		bad.BaseRunID = strconv.FormatInt(r.RunID, 10)
		if _, err := svc.Create(ctx, &users[0], bad); err == nil {
			t.Fatal("polish output contaminated ordinary upstream reuse")
		}
		body.RequestID = uuid.NewString()
		body.Mode = "branch"
		body.NodeID = "B"
		branch, err := svc.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4 && !workflow.RunTerminal(branch.State); i++ {
			branch = step(t, branch.RunID, p)
		}
		if branch.State != model.PODRunSucceeded || len(branch.Nodes) != 2 || len(branch.Graph.Nodes) != 2 || branch.TargetNodeID != "B" || calls != 2 {
			t.Fatal("branch executed unrelated node or missed upstream")
		}
	})
	t.Run("real_text_receipt_usage_and_private_history", func(t *testing.T) {
		calls := 0
		p := provider(t, func(*http.Request) (*http.Response, error) {
			calls++
			return response(`{"id":"local-text-request","choices":[{"finish_reason":"stop","message":{"content":"cat graphic"}}],"usage":{"prompt_tokens":1000,"completion_tokens":1000}}`), nil
		})
		r := makeRun(t, "textGenerate")
		for i := 0; i < 4 && !workflow.RunTerminal(r.State); i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunSucceeded || r.TestOnly || calls != 1 || r.Nodes[1].Output.Usage == nil || r.Nodes[1].Output.Usage.EstimatedCostMicros != 1650 {
			t.Fatal("real text/usage did not complete")
		}
		if _, err := svc.Get(ctx, &users[1], r.RunID); err == nil {
			t.Fatal("cross-user history exposed")
		}
		_ = step(t, r.RunID, p)
		if calls != 1 {
			t.Fatal("duplicate wake invoked provider")
		}
		var events int64
		if err := repository.DB(ctx).Table("pod_credit_event").Where("run_id=?", r.RunID).Count(&events).Error; err != nil || events != 0 {
			t.Fatal("real call consumed simulation ledger")
		}
		encoded, _ := json.Marshal(r)
		if strings.Contains(string(encoded), "submissionKey") || strings.Contains(string(encoded), "local-fixture-not-an-api-key") {
			t.Fatal("internal data leaked")
		}
	})
	t.Run("image_archive_retry_never_regenerates_and_protects_asset", func(t *testing.T) {
		calls, downloads := 0, 0
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 32, 32))); err != nil {
			t.Fatal(err)
		}
		p := provider(t, func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" {
				calls++
				return response(`{"request_id":"local-image-request","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/local.png?Expires=1&Signature=private"}]}}]},"usage":{"image_count":1}}`), nil
			}
			downloads++
			if req.Header.Get("Authorization") != "" {
				t.Fatal("download received API key")
			}
			if downloads == 1 {
				return nil, errors.New("local archive outage")
			}
			r := response(pngData.String())
			r.Header.Set("Content-Type", "image/png")
			return r, nil
		})
		r := makeRun(t, "imageGenerate")
		r = step(t, r.RunID, p)
		r = step(t, r.RunID, p)
		if r.State != model.PODRunAwaitingReconciliation || calls != 1 {
			t.Fatal("archive failure did not retain original call")
		}
		before := usageForRun(t, r.RunID)
		if before.ChargedMicros != 100000 || before.GeneratedImages != 1 || before.ReservedMicros != 0 || before.PendingCalls != 0 {
			t.Fatal("confirmed generation disappeared while archive was pending")
		}
		r = step(t, r.RunID, p)
		if r.State != model.PODRunSucceeded || calls != 1 || downloads != 2 || len(r.Nodes[1].Output.AssetIDs) != 1 {
			t.Fatal("archive retry regenerated or failed")
		}
		if after := usageForRun(t, r.RunID); after.PODUsageStats != before.PODUsageStats {
			t.Fatal("archive recovery charged twice")
		}
		id, _ := strconv.ParseInt(r.Nodes[1].Output.AssetIDs[0], 10, 64)
		asset, err := service.GetPODAsset(ctx, &users[0], id)
		if err != nil || asset.Source != "generated" || asset.ReferenceCount != 1 {
			t.Fatal("generated asset missing history protection")
		}
		if _, err := service.ChangePODAssetDeletion(ctx, &users[0], id, "1", false); err == nil {
			t.Fatal("referenced generated asset deleted")
		}
		if _, err := service.GetPODAsset(ctx, &users[1], id); err == nil {
			t.Fatal("cross-user generated asset exposed")
		}
		encoded, _ := json.Marshal(r)
		if strings.Contains(string(encoded), "Signature") || strings.Contains(string(encoded), "aliyuncs.com") {
			t.Fatal("signed result URL leaked")
		}
	})
	t.Run("reference_and_two_images_resume_partial_archive_without_regeneration", func(t *testing.T) {
		var imageData bytes.Buffer
		if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 48, 32))); err != nil {
			t.Fatal(err)
		}
		assets := service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{})
		reference, err := assets.Create(ctx, &users[0], "reference.png", "image/png", bytes.NewReader(imageData.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		project, err := service.CreatePODProject(ctx, &users[0], model.PODProjectCreate{Name: "Reference archive fixture"})
		if err != nil {
			t.Fatal(err)
		}
		refConfig, _ := json.Marshal(map[string]any{"assetIds": []string{strconv.FormatInt(reference.AssetID, 10)}})
		graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
			{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"blue flower"}`)},
			{ID: "R", Type: "imageInput", Version: 1, Config: refConfig},
			{ID: "B", Type: "imageGenerate", Version: 1, Config: json.RawMessage(`{"ratio":"16:9","count":2}`)},
		}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}, {ID: "RB", Source: "R", SourcePort: "images", Target: "B", TargetPort: "images"}}}
		project, err = service.EditPODProject(ctx, &users[0], project.ProjectID, model.PODProjectUpdate{Revision: "1", Graph: &graph}, model.PODGraphLimits{})
		if err != nil {
			t.Fatal(err)
		}
		body := model.PODRunCreate{ExecutionMode: "qwen", ProjectID: strconv.FormatInt(project.ProjectID, 10), Revision: strconv.FormatInt(project.Revision, 10), Mode: "branch", NodeID: "B", RequestID: uuid.NewString()}
		r, err := svc.Create(ctx, &users[0], body)
		if err != nil {
			t.Fatal(err)
		}
		calls, firstDownloads, secondDownloads := 0, 0, 0
		transport := func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" {
				calls++
				var wire struct {
					Input struct {
						Messages []struct {
							Content []map[string]string `json:"content"`
						} `json:"messages"`
					} `json:"input"`
					Parameters struct {
						N    int    `json:"n"`
						Size string `json:"size"`
					} `json:"parameters"`
				}
				if json.NewDecoder(req.Body).Decode(&wire) != nil || wire.Parameters.N != 2 || wire.Parameters.Size != "1280*720" || len(wire.Input.Messages) != 1 || len(wire.Input.Messages[0].Content) != 2 || !strings.HasPrefix(wire.Input.Messages[0].Content[0]["image"], "data:image/png;base64,") {
					t.Fatal("reference or multi-image request lost")
				}
				return response(`{"request_id":"reference-multi","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/first.png"},{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/second.png"}]}}]},"usage":{"image_count":2}}`), nil
			}
			if req.Header.Get("Authorization") != "" {
				t.Fatal("reference key leaked into archive request")
			}
			if req.URL.Path == "/first.png" {
				firstDownloads++
			} else {
				secondDownloads++
				if secondDownloads == 1 {
					return nil, errors.New("second image archive interrupted")
				}
			}
			out := response(imageData.String())
			out.Header.Set("Content-Type", "image/png")
			return out, nil
		}
		p := provider(t, transport)
		for i := 0; i < 5 && r.State != model.PODRunAwaitingReconciliation; i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunAwaitingReconciliation || calls != 1 || firstDownloads != 1 || secondDownloads != 1 {
			t.Fatal("partial archive did not preserve provider response")
		}
		// A new executor models a restarted process; durable mappings keep the first archive.
		p = provider(t, transport)
		for i := 0; i < 4 && !workflow.RunTerminal(r.State); i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunSucceeded || calls != 1 || firstDownloads != 1 || secondDownloads != 2 {
			t.Fatal("archive resume repeated a model call or first download")
		}
		var output model.PODRunOutput
		for _, node := range r.Nodes {
			if node.NodeID == "B" {
				output = node.Output
			}
		}
		if len(output.AssetIDs) != 2 || output.AssetIDs[0] == output.AssetIDs[1] || output.Usage == nil || output.Usage.EstimatedCostMicros != 200000 {
			t.Fatal("two archived assets or billed usage lost")
		}
		repeated, err := svc.Create(ctx, &users[0], body)
		if err != nil || repeated.RunID != r.RunID {
			t.Fatal("reloaded request created new run")
		}
		for _, id := range output.AssetIDs {
			assetID, _ := strconv.ParseInt(id, 10, 64)
			asset, err := service.GetPODAsset(ctx, &users[0], assetID)
			if err != nil || asset.Width != 48 || asset.Height != 32 || asset.ReferenceCount != 1 {
				t.Fatal("image history did not retain private asset")
			}
			if _, err := service.GetPODAsset(ctx, &users[1], assetID); err == nil {
				t.Fatal("cross-account multi-image archive exposed")
			}
		}
		_ = step(t, r.RunID, p)
		if calls != 1 || firstDownloads != 1 || secondDownloads != 2 {
			t.Fatal("finished run was regenerated")
		}
	})
	t.Run("ambiguous_call_keeps_budget_and_never_resubmits", func(t *testing.T) {
		calls := 0
		p := provider(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("local response lost") })
		r := makeRun(t, "imageGenerate")
		for i := 0; i < 4; i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunAwaitingReconciliation || calls != 1 {
			t.Fatal("uncertain request was resubmitted")
		}
		if usage := usageForRun(t, r.RunID); usage.ReservedMicros != 100000 || usage.ChargedMicros != 0 || usage.PendingCalls != 1 || usage.GeneratedImages != 0 {
			t.Fatal("unknown usage treated as known fee or generated images")
		}
		var call model.PODProviderCall
		if err := repository.DB(ctx).Where("run_id=?", r.RunID).Take(&call).Error; err != nil || call.ReservedMicros != 100000 || call.ChargedMicros != 0 {
			t.Fatal("uncertain budget released")
		}
		if _, err := svc.Change(ctx, &users[0], r.RunID, uuid.NewString()); err == nil {
			t.Fatal("uncertain run retried as confirmed failure")
		}
	})
	t.Run("confirmed_archive_survives_lost_completion_without_a_second_call", func(t *testing.T) {
		calls, downloads := 0, 0
		var pngData bytes.Buffer
		if err := jpeg.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
			t.Fatal(err)
		}
		p := provider(t, func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" {
				calls++
				return response(`{"request_id":"lost-completion","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/local.png"}]}}]},"usage":{"image_count":1}}`), nil
			}
			downloads++
			image := response(pngData.String())
			image.Header.Set("Content-Type", "image/jpeg")
			return image, nil
		})
		r := makeRun(t, "imageGenerate")
		_ = step(t, r.RunID, p)
		c, err := repository.ClaimPODNode(ctx, r.RunID, limits, service.PODNodeLease)
		if err != nil || c == nil || c.Node.NodeID != "B" {
			t.Fatal("image claim missing", err)
		}
		result := p.Execute(ctx, c)
		if result.State != "succeeded" || len(result.Output.AssetIDs) != 1 {
			t.Fatal("initial archive failed")
		}
		assetID, _ := strconv.ParseInt(result.Output.AssetIDs[0], 10, 64)
		asset, err := service.GetPODAsset(ctx, &users[0], assetID)
		if err != nil || asset.MediaType != "image/jpeg" {
			t.Fatal("generated image was forced to PNG instead of preserving its actual format", err)
		}
		// Simulate process loss after receipt/archive commit and before node completion.
		if err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id='B'", r.RunID).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		r = step(t, r.RunID, p)
		if r.State != model.PODRunSucceeded || calls != 1 || downloads != 1 || len(r.Nodes[1].Output.AssetIDs) != 1 || r.Nodes[1].Output.AssetIDs[0] != result.Output.AssetIDs[0] {
			t.Fatal("recovery repeated paid call, download or asset insertion")
		}
		if err := repository.FinishPODNode(ctx, c, result.State, result.Output, "", ""); !errors.Is(err, repository.ErrPODLease) {
			t.Fatal("stale worker overwrote recovered result", err)
		}
	})
	t.Run("old_mode_request_and_simulation_worker_cannot_pay", func(t *testing.T) {
		if _, err := svc.Create(ctx, &users[0], model.PODRunCreate{ProjectID: "1", Revision: "1", Mode: "workflow", RequestID: uuid.NewString(), TestScenario: "success"}); err == nil {
			t.Fatal("legacy test request became paid")
		}
		r := makeRun(t, "imageGenerate")
		if _, err := service.ExecutePODStep(ctx, r.RunID, limits, executor.Simulation{}); err != nil {
			t.Fatal(err)
		}
		r, err := svc.Get(ctx, &users[0], r.RunID)
		if err != nil || r.State != model.PODRunAwaitingReconciliation {
			t.Fatal("simulation worker did not preserve real mode")
		}
	})
	t.Run("uncertain_call_does_not_starve_an_independent_ready_node", func(t *testing.T) {
		calls := 0
		p := provider(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("local response lost") })
		r := makeRun(t, "imageGenerate", model.PODNode{ID: "C", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"independent"}`)})
		for i := 0; i < 4; i++ {
			r = step(t, r.RunID, p)
		}
		if calls != 1 || len(r.Nodes) != 3 || r.Nodes[2].State != "succeeded" || r.Nodes[2].Output.Text != "independent" {
			t.Fatal("pending provider call starved an independent branch")
		}
	})
	t.Run("uncertain_call_does_not_starve_another_pending_archive", func(t *testing.T) {
		calls, downloads := 0, 0
		var pngData bytes.Buffer
		if err := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 32, 32))); err != nil {
			t.Fatal(err)
		}
		p := provider(t, func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" {
				calls++
				if calls == 1 {
					return nil, errors.New("first branch response lost")
				}
				return response(`{"request_id":"second-branch","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/local.png"}]}}]},"usage":{"image_count":1}}`), nil
			}
			downloads++
			if downloads == 1 {
				return nil, errors.New("temporary archive outage")
			}
			image := response(pngData.String())
			image.Header.Set("Content-Type", "image/png")
			return image, nil
		})
		r := makeRun(t, "imageGenerate", model.PODNode{ID: "C", Type: "imageGenerate", Version: 1, Config: json.RawMessage(`{"prompt":"independent print"}`)})
		for i := 0; i < 3; i++ {
			r = step(t, r.RunID, p)
		}
		for index, nodeID := range []string{"B", "C"} {
			if err := repository.DB(ctx).Table("pod_node_run").Where("run_id=? AND node_id=?", r.RunID, nodeID).Update("update_time", time.Now().Add(time.Duration(index-2)*time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 2; i++ {
			r = step(t, r.RunID, p)
		}
		if calls != 2 || downloads != 2 || r.Nodes[1].State != "awaitingReconciliation" || r.Nodes[2].State != "succeeded" {
			t.Fatal("one uncertain call starved a recoverable archive")
		}
	})
	t.Run("exhausted_budget_rejects_before_http", func(t *testing.T) {
		p := provider(t, func(*http.Request) (*http.Response, error) {
			t.Fatal("budget-denied run called provider")
			return nil, nil
		})
		p.BudgetFen = 1
		r := makeRun(t, "imageGenerate")
		for i := 0; i < 3 && !workflow.RunTerminal(r.State); i++ {
			r = step(t, r.RunID, p)
		}
		if r.State != model.PODRunFailed || r.Nodes[1].ErrorCode != "budgetExceeded" {
			t.Fatal("insufficient budget did not fail before submission")
		}
	})
	t.Run("concurrent_reservations_cannot_exceed_remaining_budget", func(t *testing.T) {
		if err := repository.DB(ctx).Table("pod_creator_policy").Where("user_id=?", users[0].UserID).Update("max_concurrent_nodes", 2).Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Table("pod_creator_policy").Where("user_id=?", users[0].UserID).Update("max_concurrent_nodes", 1)
		a, b := makeRun(t, "imageGenerate"), makeRun(t, "imageGenerate")
		p := provider(t, func(*http.Request) (*http.Response, error) {
			t.Fatal("budget race must not call HTTP")
			return nil, nil
		})
		_ = step(t, a.RunID, p)
		_ = step(t, b.RunID, p)
		ca, err := repository.ClaimPODNode(ctx, a.RunID, limits, service.PODNodeLease)
		if err != nil || ca == nil {
			t.Fatal("claim A", err)
		}
		lb := limits
		lb.UserConcurrency = 2
		cb, err := repository.ClaimPODNode(ctx, b.RunID, lb, service.PODNodeLease)
		if err != nil || cb == nil {
			t.Fatal("claim B", err)
		}
		var used int64
		if err := repository.DB(ctx).Model(&model.PODProviderCall{}).Select("COALESCE(SUM(reserved_micros+charged_micros),0)").Scan(&used).Error; err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, c := range []*repository.PODClaim{ca, cb} {
			wg.Add(1)
			go func(c *repository.PODClaim) {
				defer wg.Done()
				_, _, err := repository.BeginPODProviderCall(ctx, c, 100000, used+100000)
				results <- err
			}(c)
		}
		wg.Wait()
		close(results)
		succeeded, denied := 0, 0
		for err := range results {
			if err == nil {
				succeeded++
			} else if errors.Is(err, repository.ErrPODBudget) {
				denied++
			} else {
				t.Fatal(err)
			}
		}
		if succeeded != 1 || denied != 1 {
			t.Fatal("concurrent budget over-reserved", succeeded, denied)
		}
	})
}
