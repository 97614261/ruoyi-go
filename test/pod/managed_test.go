package podtest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	providerexec "ruoyi-go/pkg/executor"
)

func exercisePODManaged(t *testing.T, ctx context.Context, cfg *config.Config) {
	admin := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1, UserName: "admin"}, Permissions: []string{"*:*:*"}}
	state, err := repository.PODAdminState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.EnablePODManaged(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.DB(context.Background()).Table("pod_platform_state").Where("state_id=1").Updates(map[string]any{"managed_enabled": state.ManagedEnabled, "legacy_run_max_id": state.LegacyRunMaxID, "legacy_batch_max_id": state.LegacyBatchMaxID, "cutover_time": state.CutoverTime}).Error; err != nil {
			t.Error(err)
		}
	})
	transport := http.DefaultTransport
	http.DefaultTransport = podQwenTransport(func(*http.Request) (*http.Response, error) {
		t.Error("unexpected real model request")
		return nil, errors.New("network forbidden")
	})
	t.Cleanup(func() { http.DefaultTransport = transport })
	id := func(v int64) string { return strconv.FormatInt(v, 10) }
	limits := service.PODExecutionLimits(config.PODExecutionConfig{Mode: "managed"})
	svc := service.PODRunService{Limits: limits}
	catalog := func(op string, target int64, r model.PODCatalogWrite) *model.PODCatalogResult {
		t.Helper()
		r.RequestID = uuid.NewString()
		r.Reason = "本地 managed 替身测试"
		v, e := service.PODCatalogWrite(ctx, admin, op, target, r)
		if e != nil {
			t.Fatal(op, e)
		}
		return v
	}
	creator := func(op string, target int64, r model.PODCreatorWrite) *model.PODCatalogResult {
		t.Helper()
		r.RequestID = uuid.NewString()
		r.Reason = "本地 managed 专用用户"
		v, e := service.PODCreatorWrite(ctx, admin, op, target, r, limits)
		if e != nil {
			t.Fatal(op, e)
		}
		return v
	}
	p := catalog("provider.create", 0, model.PODCatalogWrite{Code: "managed-" + uuid.NewString()[:8], Name: "Managed offline fixture", AdapterKind: "qwen-v1", Region: "cn-beijing", Endpoint: "https://dashscope.aliyuncs.com"})
	catalog("provider.credential", p.ResourceID, model.PODCatalogWrite{Revision: id(p.Revision), SourceType: "encrypted", Secret: "fake-managed-key-never-used-on-network"})
	var provider model.PODProvider
	if e := repository.DB(ctx).Where("provider_id=?", p.ResourceID).Take(&provider).Error; e != nil {
		t.Fatal(e)
	}
	catalog("provider.status", p.ResourceID, model.PODCatalogWrite{Revision: id(provider.Revision), Status: "enabled"})
	visible := true
	m := catalog("model.create", 0, model.PODCatalogWrite{Code: "managed-text-" + uuid.NewString()[:8], Name: "Managed text fixture", ProviderID: id(p.ResourceID), Kind: "text", Visible: &visible})
	spec := model.PODCatalogWrite{Revision: id(m.Revision), OfficialModelID: "qwen-flash", TimeoutSeconds: 20, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64}, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}}
	v := catalog("model.version", m.ResourceID, spec)
	catalog("model.status", m.ResourceID, model.PODCatalogWrite{Revision: id(v.Revision), Status: "enabled"})
	modelNow := func() model.PODCatalogModel {
		t.Helper()
		var row model.PODCatalogModel
		if e := repository.DB(ctx).Where("model_id=?", m.ResourceID).Take(&row).Error; e != nil {
			t.Fatal(e)
		}
		return row
	}
	policy := model.PODCreatorPolicyInput{Status: "active", MaxConcurrentNodes: 1, MaxPendingRuns: limits.MaxUserPending, MaxDailySubmissions: 1000, MaxStorageBytes: "1073741824"}
	var users []model.SysUser
	for i := 0; i < 2; i++ {
		u := creator("creator.create", 0, model.PODCreatorWrite{UserName: "managed_" + uuid.NewString()[:8], NickName: "Managed fixture", InitialPassword: "Md-" + uuid.NewString()[:15], Policy: &policy})
		user := model.SysUser{UserID: u.ResourceID, UserName: "managed_fixture"}
		users = append(users, user)
		d, e := repository.PODCreator(ctx, user.UserID)
		if e != nil {
			t.Fatal(e)
		}
		models := []string{id(m.ResourceID)}
		creator("creator.models", user.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &models})
		creator("points.adjust", user.UserID, model.PODCreatorWrite{AccountRevision: "1", Kind: "grant", Amount: "100000"})
	}
	t.Cleanup(func() {
		for _, u := range users {
			d, e := repository.PODCreator(context.Background(), u.UserID)
			if e != nil {
				t.Error(e)
				continue
			}
			suspended := policy
			suspended.Status = "suspended"
			_, e = service.PODCreatorWrite(context.Background(), admin, "creator.policy", u.UserID, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(d.Creator.Revision), Reason: "managed 测试结束，保留账本", Policy: &suspended}, limits)
			if e != nil {
				t.Error(e)
			}
		}
		now := modelNow()
		if now.Status == "enabled" {
			catalog("model.status", m.ResourceID, model.PODCatalogWrite{Revision: id(now.Revision), Status: "disabled"})
		}
		if e := repository.PODVerifyCreatorAccounts(context.Background()); e != nil {
			t.Error(e)
		}
		if e := repository.PODVerifyManagedAccounts(context.Background()); e != nil {
			t.Error(e)
		}
	})
	owner := &users[0]
	account := func() model.PODPointsAccount {
		t.Helper()
		a, e := repository.PODPoints(ctx, owner.UserID)
		if e != nil {
			t.Fatal(e)
		}
		return *a
	}
	project := func(user *model.SysUser, chain bool) model.PODRunCreate {
		t.Helper()
		p, e := service.CreatePODProject(ctx, user, model.PODProjectCreate{Name: "Managed offline workflow"})
		if e != nil {
			t.Fatal(e)
		}
		conf, _ := json.Marshal(map[string]string{"modelId": id(m.ResourceID)})
		g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"cat print"}`)}, {ID: "B", Type: "textGenerate", Version: 1, Config: conf}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
		if chain {
			g.Nodes = append(g.Nodes, model.PODNode{ID: "C", Type: "textGenerate", Version: 1, Config: conf})
			g.Edges = append(g.Edges, model.PODEdge{ID: "BC", Source: "B", SourcePort: "text", Target: "C", TargetPort: "text"})
		}
		p, e = service.EditPODProject(ctx, user, p.ProjectID, model.PODProjectUpdate{Revision: "1", Graph: &g}, model.PODGraphLimits{})
		if e != nil {
			t.Fatal(e)
		}
		return model.PODRunCreate{ProjectID: id(p.ProjectID), Revision: id(p.Revision), Mode: "workflow", RequestID: uuid.NewString(), ExecutionMode: "managed"}
	}
	quote := func(user *model.SysUser, body model.PODRunCreate) *model.PODQuoteView {
		t.Helper()
		q, e := svc.Quote(ctx, user, model.PODQuoteRequest{Kind: "run", Run: &body})
		if e != nil {
			t.Fatal(e)
		}
		return q
	}
	create := func(body model.PODRunCreate) *model.PODRunView {
		t.Helper()
		r, e := svc.Create(ctx, owner, body)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	response := func(status int, body string) *http.Response {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	}
	success := `{"id":"managed-local-only","choices":[{"finish_reason":"stop","message":{"content":"cat graphic"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`
	var calls atomic.Int64
	executor := func(fn func(*http.Request) (*http.Response, error)) *service.PODQwenExecutor {
		return &service.PODQwenExecutor{Client: &http.Client{Transport: podQwenTransport(func(r *http.Request) (*http.Response, error) { calls.Add(1); return fn(r) })}, Images: &http.Client{}, Assets: service.NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{}), BudgetFen: 5000}
	}
	okExecutor := executor(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "qwen-flash" || body["max_tokens"] != float64(64) {
			t.Error("bound model/capabilities not used")
		}
		if r.Header.Get("Authorization") != "Bearer fake-managed-key-never-used-on-network" {
			t.Error("bound credential not used")
		}
		return response(200, success), nil
	})
	step := func(run int64, x *service.PODQwenExecutor) *model.PODRunView {
		t.Helper()
		if _, e := service.ExecutePODQwenStep(ctx, run, limits, x); e != nil {
			t.Fatal(e)
		}
		v, e := svc.Get(ctx, owner, run)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	finish := func(run int64, x *service.PODQwenExecutor) *model.PODRunView {
		t.Helper()
		for i := 0; i < 8; i++ {
			v := step(run, x)
			if v.State == "succeeded" || v.State == "failed" || v.State == "cancelled" || v.State == "awaitingReconciliation" {
				return v
			}
		}
		t.Fatal("run did not finish")
		return nil
	}
	t.Run("multiple_vendors_use_bound_versions_and_settle_once", func(t *testing.T) {
		for _, vendor := range []struct{ origin, region, model, path string }{
			{"https://api.deepseek.com", "cn", "deepseek-flash", "/chat/completions"},
			{"https://open.bigmodel.cn", "cn", "glm-4.7", "/api/paas/v4/chat/completions"},
			{"https://api.siliconflow.cn", "cn", "Qwen/Qwen3-8B", "/v1/chat/completions"},
			{"https://api.openai.com", "global", "gpt-4.1-mini", "/v1/chat/completions"},
		} {
			t.Run(vendor.model, func(t *testing.T) {
				vp := catalog("provider.create", 0, model.PODCatalogWrite{Code: "managed-" + uuid.NewString()[:8], Name: "Vendor offline fixture", AdapterKind: "openai-chat-v1", Region: vendor.region, Endpoint: vendor.origin})
				vc := catalog("provider.credential", vp.ResourceID, model.PODCatalogWrite{Revision: id(vp.Revision), SourceType: "encrypted", Secret: "fake-managed-key-never-used-on-network"})
				catalog("provider.status", vp.ResourceID, model.PODCatalogWrite{Revision: id(vc.Revision), Status: "enabled"})
				vm := catalog("model.create", 0, model.PODCatalogWrite{Code: "managed-text-" + uuid.NewString()[:8], Name: "Vendor text fixture", ProviderID: id(vp.ResourceID), Kind: "text", Visible: &visible})
				vv := catalog("model.version", vm.ResourceID, model.PODCatalogWrite{Revision: id(vm.Revision), OfficialModelID: vendor.model, TimeoutSeconds: 20, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64}, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", InputPerMillion: 7_000_000, OutputPerMillion: 11_000_000}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", InputPerMillion: 1_000_000, OutputPerMillion: 2_000_000}})
				catalog("model.status", vm.ResourceID, model.PODCatalogWrite{Revision: id(vv.Revision), Status: "enabled"})
				d, e := repository.PODCreator(ctx, owner.UserID)
				if e != nil {
					t.Fatal(e)
				}
				grants := []string{id(m.ResourceID), id(vm.ResourceID)}
				creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &grants})
				defer func() {
					d, e := repository.PODCreator(ctx, owner.UserID)
					if e != nil {
						t.Error(e)
						return
					}
					grants := []string{id(m.ResourceID)}
					creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &grants})
				}()
				p, e := service.CreatePODProject(ctx, owner, model.PODProjectCreate{Name: "Multi-vendor offline acceptance"})
				if e != nil {
					t.Fatal(e)
				}
				conf, _ := json.Marshal(map[string]string{"modelId": id(vm.ResourceID)})
				graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"flower"}`)}, {ID: "B", Type: "textGenerate", Version: 1, Config: conf}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}}}
				p, e = service.EditPODProject(ctx, owner, p.ProjectID, model.PODProjectUpdate{Revision: id(p.Revision), Graph: &graph}, model.PODGraphLimits{})
				if e != nil {
					t.Fatal(e)
				}
				body := model.PODRunCreate{ProjectID: id(p.ProjectID), Revision: id(p.Revision), Mode: "workflow", RequestID: uuid.NewString(), ExecutionMode: "managed"}
				q := quote(owner, body)
				body.QuoteID = q.QuoteID
				before := account()
				run := create(body)
				requests := 0
				x := executor(func(req *http.Request) (*http.Response, error) {
					requests++
					var payload map[string]any
					if req.URL.String() != vendor.origin+vendor.path || json.NewDecoder(req.Body).Decode(&payload) != nil || payload["model"] != vendor.model {
						t.Error("managed execution lost its provider/model binding")
					}
					return response(200, success), nil
				})
				if got := finish(run.RunID, x); got.State != "succeeded" {
					t.Fatal("vendor workflow failed", got.State)
				}
				step(run.RunID, x)
				after := account()
				var cost struct {
					ChargedMicros int64
					State         string
				}
				if e := repository.DB(ctx).Table("pod_provider_call").Select("charged_micros,state").Where("run_id=?", run.RunID).Take(&cost).Error; e != nil {
					t.Fatal(e)
				}
				if requests != 1 || cost.State != "confirmed" || cost.ChargedMicros != 125 || after.Spent-before.Spent != 20 || after.Held != before.Held {
					t.Fatalf("provider cost or points drifted: requests=%d cost=%+v before=%+v after=%+v", requests, cost, before, after)
				}
			})
		}
	})
	t.Run("platform_chain", func(t *testing.T) {
		exercisePODPlatformChain(t, ctx, cfg, owner, &users[1], m.ResourceID, okExecutor, limits)
	})
	t.Run("multi_image_archive_recovery_and_credential_verification", func(t *testing.T) {
		for _, vendor := range []struct{ adapter, origin, region, official string }{
			{"openai-chat-v1", "https://open.bigmodel.cn", "cn", "cogview-3-flash"},
			{"openai-chat-v1", "https://api.siliconflow.cn", "cn", "Kwai-Kolors/Kolors"},
			{"openai-compatible-v1", "https://relay.example.com/v1", "global", "custom-image-model"},
		} {
			t.Run(vendor.official, func(t *testing.T) {
				vp := catalog("provider.create", 0, model.PODCatalogWrite{Code: "image-fixture-" + uuid.NewString()[:8], Name: "Image protocol offline fixture", AdapterKind: vendor.adapter, Region: vendor.region, Endpoint: vendor.origin})
				credential := catalog("provider.credential", vp.ResourceID, model.PODCatalogWrite{Revision: id(vp.Revision), SourceType: "encrypted", Secret: "fake-image-key-never-used-on-network"})
				active := catalog("provider.status", vp.ResourceID, model.PODCatalogWrite{Revision: id(credential.Revision), Status: "enabled"})
				im := catalog("model.create", 0, model.PODCatalogWrite{Code: "image-fixture-" + uuid.NewString()[:8], Name: "Image fixture", ProviderID: id(vp.ResourceID), Kind: "image", Visible: &visible})
				caps := &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, Ratios: []string{"1:1"}}
				if vendor.adapter == "openai-compatible-v1" {
					caps.ImageResponse = "url"
				}
				iv := catalog("model.version", im.ResourceID, model.PODCatalogWrite{Revision: id(im.Revision), OfficialModelID: vendor.official, TimeoutSeconds: 20, Capabilities: caps, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", PerImage: 100}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", PerImage: 10}})
				catalog("model.status", im.ResourceID, model.PODCatalogWrite{Revision: id(iv.Revision), Status: "enabled"})
				d, e := repository.PODCreator(ctx, owner.UserID)
				if e != nil {
					t.Fatal(e)
				}
				original := append([]string{}, d.ModelIDs...)
				grants := append(append([]string{}, original...), id(im.ResourceID))
				creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &grants})
				defer func() {
					d, e := repository.PODCreator(ctx, owner.UserID)
					if e != nil {
						t.Error(e)
						return
					}
					creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &original})
					var current model.PODCatalogModel
					repository.DB(ctx).Where("model_id=?", im.ResourceID).Take(&current)
					catalog("model.status", im.ResourceID, model.PODCatalogWrite{Revision: id(current.Revision), Status: "disabled"})
				}()
				body := project(owner, false)
				pid, _ := strconv.ParseInt(body.ProjectID, 10, 64)
				p, e := service.GetPODProject(ctx, owner, pid)
				if e != nil {
					t.Fatal(e)
				}
				conf, _ := json.Marshal(map[string]any{"modelId": id(im.ResourceID), "count": 1, "ratio": "1:1"})
				p.Graph.Nodes[1].Type = "imageGenerate"
				p.Graph.Nodes[1].Config = conf
				p, e = service.EditPODProject(ctx, owner, pid, model.PODProjectUpdate{Revision: body.Revision, Graph: p.Graph}, model.PODGraphLimits{})
				if e != nil {
					t.Fatal(e)
				}
				body.Revision = id(p.Revision)
				body.QuoteID = quote(owner, body).QuoteID
				run := create(body)
				var pngData bytes.Buffer
				if e := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 16, 16))); e != nil {
					t.Fatal(e)
				}
				posts, downloads := 0, 0
				x := executor(func(req *http.Request) (*http.Response, error) {
					posts++
					if req.Header.Get("Authorization") != "Bearer fake-image-key-never-used-on-network" {
						t.Error("wrong credential")
					}
					data := `{"data":[{"url":"https://cdn.example.com/result.png"}]}`
					if vendor.origin == "https://api.siliconflow.cn" {
						data = `{"images":[{"url":"https://cdn.example.com/result.png"}]}`
					}
					if vendor.adapter == "openai-compatible-v1" {
						var request map[string]any
						if json.NewDecoder(req.Body).Decode(&request) != nil || request["response_format"] != "url" {
							t.Fatal("URL-only request lost its response format")
						}
					}
					return response(200, data), nil
				})
				x.CatalogImages = &http.Client{Transport: podQwenTransport(func(req *http.Request) (*http.Response, error) {
					downloads++
					if req.Header.Get("Authorization") != "" {
						t.Error("CDN received credential")
					}
					if downloads == 1 {
						return nil, io.ErrUnexpectedEOF
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(pngData.Bytes()))}, nil
				})}
				step(run.RunID, x)
				claim, e := repository.ClaimPODNode(ctx, run.RunID, limits, time.Minute)
				if e != nil || claim == nil {
					t.Fatal("image claim missing", e)
				}
				result := x.Execute(ctx, claim)
				if result.State != "awaitingReconciliation" || result.Code != "archivePending" {
					t.Fatal("archive failure lost receipt", result)
				}
				result = x.Execute(ctx, claim)
				if result.State != "succeeded" || len(result.Output.AssetIDs) != 1 || posts != 1 {
					t.Fatal("image replay failed", result.State, posts)
				}
				again := x.Execute(ctx, claim)
				if again.State != "succeeded" || posts != 1 || again.Output.AssetIDs[0] != result.Output.AssetIDs[0] {
					t.Fatal("receipt replay resubmitted or duplicated asset")
				}
				if e = repository.FinishPODNode(ctx, claim, result.State, result.Output, result.Code, result.Message); e != nil {
					t.Fatal(e)
				}
				got, e := svc.Get(ctx, owner, run.RunID)
				if e != nil || got.Billing.Spent != 10 || got.Billing.Held != 0 {
					t.Fatal("image billing mismatch", e)
				}
				view, e := repository.PODCatalogModelGet(ctx, im.ResourceID)
				if e != nil || view.VerifiedAt == nil {
					t.Fatal("success receipt not recognized", e)
				}
				catalog("provider.credential", vp.ResourceID, model.PODCatalogWrite{Revision: id(active.Revision), SourceType: "encrypted", Secret: "rotated-fake-image-key"})
				view, e = repository.PODCatalogModelGet(ctx, im.ResourceID)
				if e != nil || view.VerifiedAt != nil {
					t.Fatal("credential rotation retained old verification", e)
				}
				if e = repository.PODVerifyManagedAccounts(ctx); e != nil {
					t.Fatal(e)
				}
				if vendor.adapter == "openai-compatible-v1" {
					var current model.PODCatalogModel
					if e := repository.DB(ctx).Where("model_id=?", im.ResourceID).Take(&current).Error; e != nil {
						t.Fatal(e)
					}
					caps.ImageResponse = "b64_json"
					catalog("model.version", im.ResourceID, model.PODCatalogWrite{Revision: id(current.Revision), OfficialModelID: vendor.official, TimeoutSeconds: 20, Capabilities: caps, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", PerImage: 100}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", PerImage: 10}})
					body.RequestID = uuid.NewString()
					body.QuoteID = quote(owner, body).QuoteID
					inlineRun := create(body)
					inlinePosts := 0
					var large bytes.Buffer
					raster := image.NewNRGBA(image.Rect(0, 0, 1200, 1200))
					for i := 3; i < len(raster.Pix); i += 8 {
						raster.Pix[i] = 255
					}
					if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&large, raster); err != nil || large.Len() <= 4<<20 {
						t.Fatal("large inline image fixture invalid", err)
					}
					y := executor(func(req *http.Request) (*http.Response, error) {
						inlinePosts++
						var request map[string]any
						if json.NewDecoder(req.Body).Decode(&request) != nil || request["response_format"] != "b64_json" {
							t.Fatal("Base64 request lost its response format")
						}
						return response(200, `{"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(large.Bytes())+`"}]}`), nil
					})
					y.CatalogImages = &http.Client{Transport: podQwenTransport(func(*http.Request) (*http.Response, error) {
						t.Error("inline result attempted a network download")
						return nil, io.ErrUnexpectedEOF
					})}
					step(inlineRun.RunID, y)
					claim, e := repository.ClaimPODNode(ctx, inlineRun.RunID, limits, time.Minute)
					if e != nil || claim == nil {
						t.Fatal("inline image claim missing", e)
					}
					result := y.Execute(ctx, claim)
					if result.State != "succeeded" || len(result.Output.AssetIDs) != 1 || inlinePosts != 1 {
						t.Fatal("inline image did not archive", result.State, result.Code)
					}
					receipt, e := repository.SelectPODProviderCall(ctx, claim)
					if e != nil || receipt == nil || len(receipt.ReceiptJSON) > 4096 || !bytes.Contains(receipt.ReceiptJSON, []byte("pod-local:0")) || bytes.Contains(receipt.ReceiptJSON, []byte("base64,")) {
						t.Fatal("managed receipt did not keep compact local references", e)
					}
					replay := y.Execute(ctx, claim)
					if replay.State != "succeeded" || inlinePosts != 1 || len(replay.Output.AssetIDs) != 1 || replay.Output.AssetIDs[0] != result.Output.AssetIDs[0] {
						t.Fatal("inline replay resubmitted or duplicated the asset")
					}
					if e = repository.FinishPODNode(ctx, claim, result.State, result.Output, result.Code, result.Message); e != nil {
						t.Fatal(e)
					}
				}
			})
		}
	})
	t.Run("mode_cutover_replays_existing_run_and_batch_without_new_holds", func(t *testing.T) {
		old := service.PODRunService{Limits: service.PODExecutionLimits(config.PODExecutionConfig{Mode: "qwen"})}
		body := project(owner, false)
		body.ExecutionMode = "qwen"
		r, e := old.Create(ctx, owner, body)
		if e != nil {
			t.Fatal(e)
		}
		before := account()
		replay, e := svc.Create(ctx, owner, body)
		if e != nil || replay.RunID != r.RunID {
			t.Fatal("legacy replay lost original run", e)
		}
		changed := body
		changed.RequestID = uuid.NewString()
		if _, e = svc.Create(ctx, owner, changed); e == nil {
			t.Fatal("new legacy request accepted")
		}
		if got := finish(r.RunID, okExecutor); got.State != "cancelled" {
			t.Fatal("legacy queued run was not drained", got.State)
		}
		template, e := (service.PODTemplateService{}).Save(ctx, owner, 0, model.PODTemplateSave{RequestID: uuid.NewString(), ProjectID: body.ProjectID, ProjectRevision: body.Revision, Name: "Legacy replay fixture"})
		if e != nil {
			t.Fatal(e)
		}
		bb := model.PODBatchCreate{RequestID: uuid.NewString(), TemplateID: id(template.TemplateID), Version: "1", Name: "Legacy batch replay", ExecutionMode: "qwen", Items: []model.PODBatchInput{{Name: "One"}}}
		bs := service.PODBatchService{Runs: old}
		b, e := bs.Create(ctx, owner, bb)
		if e != nil {
			t.Fatal(e)
		}
		bs.Runs = svc
		br, e := bs.Create(ctx, owner, bb)
		if e != nil || br.BatchID != b.BatchID {
			t.Fatal("legacy batch replay lost original", e)
		}
		bb.RequestID = uuid.NewString()
		if _, e = bs.Create(ctx, owner, bb); e == nil {
			t.Fatal("new legacy batch accepted")
		}
		if _, e = bs.Stop(ctx, owner, b.BatchID); e != nil {
			t.Fatal(e)
		}
		if after := account(); after.Available != before.Available || after.Held != before.Held {
			t.Fatal("cutover changed formal points")
		}
	})
	t.Run("legacy_receipts_reconcile_without_old_key_and_unknown_never_resubmits", func(t *testing.T) {
		old := service.PODRunService{Limits: service.PODExecutionLimits(config.PODExecutionConfig{Mode: "qwen"})}
		for _, confirmed := range []bool{true, false} {
			body := project(owner, false)
			body.ExecutionMode = "qwen"
			r, e := old.Create(ctx, owner, body)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = service.ExecutePODQwenStep(ctx, r.RunID, old.Limits, okExecutor); e != nil {
				t.Fatal(e)
			}
			c, e := repository.ClaimPODNode(ctx, r.RunID, old.Limits, time.Minute)
			if e != nil || c == nil || c.Node.NodeID != "B" {
				t.Fatal("legacy claim missing", e)
			}
			if _, fresh, e := repository.BeginPODProviderCall(ctx, c, 100, 50_000_000); e != nil || !fresh {
				t.Fatal("legacy intent missing", e)
			}
			receipt, _ := json.Marshal(providerexec.QwenReceipt{State: "succeeded", Text: "durable legacy text"})
			if e = repository.SavePODProviderReceipt(ctx, c, receipt, 10, confirmed); e != nil {
				t.Fatal(e)
			}
			if e = repository.DB(ctx).Model(&model.PODNodeRun{}).Where("run_id=? AND node_id='B'", r.RunID).Update("lease_until", time.Now().Add(-time.Minute)).Error; e != nil {
				t.Fatal(e)
			}
			beforeCalls := calls.Load()
			got := step(r.RunID, okExecutor)
			if calls.Load() != beforeCalls {
				t.Fatal("legacy call resubmitted")
			}
			var node model.PODNodeRunView
			for _, n := range got.Nodes {
				if n.NodeID == "B" {
					node = n
				}
			}
			if confirmed && (node.State != "succeeded" || node.Output.Text != "durable legacy text") {
				t.Fatal("durable receipt blocked by missing old key", node)
			}
			if !confirmed && node.State != "awaitingReconciliation" {
				t.Fatal("unknown receipt lost", node.State)
			}
		}
	})
	t.Run("models_and_quotes_are_private_no_cost_or_credential", func(t *testing.T) {
		models, e := service.PODCreatorModels(ctx, owner, "text")
		if e != nil || len(models) != 1 {
			t.Fatal("authorized models unavailable", e)
		}
		b, _ := json.Marshal(models)
		for _, s := range []string{"fake-managed-key", "costRule", "endpoint", "providerId", "ciphertext"} {
			if bytes.Contains(b, []byte(s)) {
				t.Fatal("private model fields leaked")
			}
		}
		body := project(owner, false)
		before := account()
		q := quote(owner, body)
		if q.Points != 896 || account().Held != before.Held {
			t.Fatal("quote reserved points or bound incorrect")
		}
		if _, e = repository.GetPODQuote(ctx, users[1].UserID, q.QuoteID); errs.As(e) == nil || errs.As(e).Code != 404 {
			t.Fatal("other user read quote", e)
		}
		managedCfg := *cfg
		managedCfg.POD = cfg.POD
		managedCfg.POD.Execution.Mode = "managed"
		engine := router.New(&managedCfg)
		for _, path := range []string{"/pod/models", "/pod/execution-quotes/" + q.QuoteID} {
			rr := httptest.NewRecorder()
			engine.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
			var p map[string]json.RawMessage
			json.Unmarshal(rr.Body.Bytes(), &p)
			if string(p["code"]) != "401" {
				t.Fatal("unauthenticated managed API open")
			}
		}
		token, e := service.CreateToken(ctx, &model.LoginUser{UserID: owner.UserID, User: owner, Permissions: []string{model.PODStudioAccess}})
		if e != nil {
			t.Fatal(e)
		}
		httpCall := func(method, path string, input any) map[string]json.RawMessage {
			t.Helper()
			var raw []byte
			if input != nil {
				raw, _ = json.Marshal(input)
			}
			req := httptest.NewRequest(method, path, bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+token)
			if input != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()
			engine.ServeHTTP(rr, req)
			var p map[string]json.RawMessage
			if json.Unmarshal(rr.Body.Bytes(), &p) != nil || string(p["code"]) != "200" {
				t.Fatal("authenticated managed route rejected", method, path, rr.Body.String())
			}
			for _, secret := range []string{"fake-managed-key", "costRule", "endpoint", "ciphertext"} {
				if bytes.Contains(rr.Body.Bytes(), []byte(secret)) {
					t.Fatal("HTTP response leaked private catalog data")
				}
			}
			return p
		}
		httpCall("GET", "/pod/models?kind=text", nil)
		httpCall("GET", "/pod/models/"+id(m.ResourceID), nil)
		qr := httpCall("POST", "/pod/execution-quotes", model.PODQuoteRequest{Kind: "run", Run: &body})
		var quoted model.PODQuoteView
		if json.Unmarshal(qr["data"], &quoted) != nil || quoted.QuoteID == "" || quoted.Points != 896 {
			t.Fatal("HTTP quote shape changed")
		}
		if len(quoted.Lines) != 1 || quoted.Lines[0].Kind != "text" || quoted.Lines[0].OfficialModelID != "qwen-flash" || quoted.Lines[0].ModelVersionID == 0 {
			t.Fatal("HTTP quote missing frozen model identity")
		}
		httpCall("GET", "/pod/execution-quotes/"+quoted.QuoteID, nil)
		body.QuoteID = quoted.QuoteID
		created := httpCall("POST", "/pod/runs", body)
		var run model.PODRunView
		if json.Unmarshal(created["data"], &run) != nil || run.Billing == nil || run.Billing.Held != quoted.Points {
			t.Fatal("HTTP run billing missing")
		}
		for _, node := range run.Nodes {
			if node.Output.TestOnly {
				t.Fatal("queued managed node mislabeled as simulation")
			}
		}
		httpCall("POST", "/pod/runs/"+id(run.RunID)+"/cancel", struct{}{})
	})
	t.Run("quote_validation_expiry_policy_and_limit", func(t *testing.T) {
		body := project(owner, false)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		body.Mode = "node"
		body.NodeID = "B"
		if _, e := svc.Create(ctx, owner, body); e == nil {
			t.Fatal("changed quote source accepted")
		}
		body.Mode = "workflow"
		body.NodeID = ""
		if e := repository.DB(ctx).Model(&model.PODExecutionQuote{}).Where("quote_id=?", q.QuoteID).Update("expires_at", "2000-01-01").Error; e != nil {
			t.Fatal(e)
		}
		if _, e := svc.Create(ctx, owner, body); e == nil {
			t.Fatal("expired quote accepted")
		}
		q = quote(owner, body)
		body.QuoteID = q.QuoteID
		d, e := repository.PODCreator(ctx, owner.UserID)
		if e != nil {
			t.Fatal(e)
		}
		creator("creator.policy", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), Policy: &policy})
		if _, e := svc.Create(ctx, owner, body); e == nil {
			t.Fatal("changed policy accepted")
		}
		if e := repository.DB(ctx).Model(&model.PODExecutionQuote{}).Where("user_id=? AND state='ready'", owner.UserID).Update("expires_at", "2000-01-01").Error; e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 20; i++ {
			quote(owner, body)
		}
		if _, e := svc.Quote(ctx, owner, model.PODQuoteRequest{Kind: "run", Run: &body}); errs.As(e) == nil || errs.As(e).Code != 429 {
			t.Fatal("unbounded live quotes", e)
		}
		repository.DB(ctx).Model(&model.PODExecutionQuote{}).Where("user_id=? AND state='ready'", owner.UserID).Update("expires_at", "2000-01-01")
	})
	t.Run("hold_settle_replay_and_cancel", func(t *testing.T) {
		body := project(owner, false)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		before := account()
		r := create(body)
		if a := account(); a.Held != before.Held+q.Points || a.Available != before.Available-q.Points {
			t.Fatal("whole-run hold missing")
		}
		if again := create(body); again.RunID != r.RunID || account().Held != before.Held+q.Points {
			t.Fatal("submission replay charged twice")
		}
		body.RequestID = uuid.NewString()
		if _, e := svc.Create(ctx, owner, body); e == nil {
			t.Fatal("quote consumed twice")
		}
		r = finish(r.RunID, okExecutor)
		if r.State != "succeeded" || r.Billing == nil || r.Billing.Spent != 20 || r.Billing.Held != 0 || r.Billing.Released != 876 {
			t.Fatal("receipt settlement incorrect", r.Billing)
		}
		if a := account(); a.Spent != before.Spent+20 || a.Held != before.Held {
			t.Fatal("formal ledger mismatch")
		}
		body = project(owner, false)
		q = quote(owner, body)
		body.QuoteID = q.QuoteID
		r = create(body)
		count := calls.Load()
		if _, e := svc.Change(ctx, owner, r.RunID, ""); e != nil {
			t.Fatal(e)
		}
		r = finish(r.RunID, okExecutor)
		if r.State != "cancelled" || r.Billing.Held != 0 || r.Billing.Spent != 0 || calls.Load() != count {
			t.Fatal("cancel submitted or failed to refund")
		}
	})
	t.Run("managed_run_without_owned_snapshot_cannot_execute", func(t *testing.T) {
		body := project(owner, false)
		body.QuoteID = quote(owner, body).QuoteID
		r := create(body)
		if e := repository.DB(ctx).Model(&model.PODExecutionSnapshot{}).Where("scope_kind='run' AND scope_id=?", r.RunID).Update("user_id", users[1].UserID).Error; e != nil {
			t.Fatal(e)
		}
		claim, claimErr := repository.ClaimPODNode(ctx, r.RunID, limits, time.Minute)
		if e := repository.DB(ctx).Model(&model.PODExecutionSnapshot{}).Where("scope_kind='run' AND scope_id=?", r.RunID).Update("user_id", owner.UserID).Error; e != nil {
			t.Fatal(e)
		}
		if claimErr == nil || claim != nil {
			t.Fatal("managed run bypassed missing owner snapshot")
		}
		if _, e := svc.Change(ctx, owner, r.RunID, ""); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("version_change_requires_new_quote", func(t *testing.T) {
		body := project(owner, false)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		now := modelNow()
		spec.Revision = id(now.Revision)
		catalog("model.version", m.ResourceID, spec)
		if _, e := svc.Create(ctx, owner, body); e == nil {
			t.Fatal("stale price accepted")
		}
	})
	t.Run("concurrent_holds_do_not_overspend", func(t *testing.T) {
		user := &users[1]
		a, e := repository.PODPoints(ctx, user.UserID)
		if e != nil {
			t.Fatal(e)
		}
		creator("points.adjust", user.UserID, model.PODCreatorWrite{AccountRevision: id(a.Revision), Kind: "deduct", Amount: id(a.Available - 896)})
		b1 := project(user, false)
		b2 := project(user, false)
		b1.QuoteID = quote(user, b1).QuoteID
		b2.QuoteID = quote(user, b2).QuoteID
		var wg sync.WaitGroup
		var passed atomic.Int64
		var runID atomic.Int64
		for _, body := range []model.PODRunCreate{b1, b2} {
			wg.Add(1)
			go func(b model.PODRunCreate) {
				defer wg.Done()
				r, e := svc.Create(ctx, user, b)
				if e == nil {
					passed.Add(1)
					runID.Store(r.RunID)
				}
			}(body)
		}
		wg.Wait()
		if passed.Load() != 1 {
			t.Fatal("parallel holds exceeded balance", passed.Load())
		}
		a, e = repository.PODPoints(ctx, user.UserID)
		if e != nil || a.Available != 0 || a.Held != 896 {
			t.Fatal("overspent", e)
		}
		if _, e = svc.Change(ctx, user, runID.Load(), ""); e != nil {
			t.Fatal(e)
		}
		if _, e = service.ExecutePODQwenStep(ctx, runID.Load(), limits, okExecutor); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("retry_failed_only_original_prices_and_idempotency", func(t *testing.T) {
		body := project(owner, true)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		r := create(body)
		var n int
		x := executor(func(*http.Request) (*http.Response, error) {
			n++
			if n == 1 {
				return response(200, success), nil
			}
			return response(400, `{"code":"InvalidParameter","message":"fixture"}`), nil
		})
		r = finish(r.RunID, x)
		if r.State != "failed" || r.Billing.Spent != 20 {
			t.Fatal("fixture did not fail after first success")
		}
		var first model.PODRunModelBinding
		repository.DB(ctx).Where("run_id=? AND node_id='C'", r.RunID).Take(&first)
		now := modelNow()
		updated := spec
		updated.Revision = id(now.Revision)
		pr := *spec.PointRule
		pr.InputPerMillion *= 2
		updated.PointRule = &pr
		catalog("model.version", m.ResourceID, updated)
		rq, e := svc.Quote(ctx, owner, model.PODQuoteRequest{Kind: "retry", RunID: id(r.RunID)})
		if e != nil || len(rq.Lines) != 1 || rq.Lines[0].NodeID != "C" || rq.Lines[0].ModelVersionID != first.ModelVersionID || rq.Points != 896 {
			t.Fatal("retry changed versions or included success", e)
		}
		key := uuid.NewString()
		before := account()
		r, e = svc.Change(ctx, owner, r.RunID, key, rq.QuoteID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = svc.Change(ctx, owner, r.RunID, key, rq.QuoteID); e != nil {
			t.Fatal(e)
		}
		if account().Held != before.Held+896 {
			t.Fatal("retry held twice")
		}
		count := calls.Load()
		r = finish(r.RunID, okExecutor)
		if r.State != "succeeded" || calls.Load() != count+1 || r.Billing.Spent != 40 || r.Billing.Held != 0 {
			t.Fatal("retry reran successful node or mis-settled")
		}
	})
	t.Run("batch_transfer_and_stop_waiting_only", func(t *testing.T) {
		body := project(owner, false)
		template, e := (service.PODTemplateService{}).Save(ctx, owner, 0, model.PODTemplateSave{RequestID: uuid.NewString(), ProjectID: body.ProjectID, ProjectRevision: body.Revision, Name: "Managed fixed batch"})
		if e != nil {
			t.Fatal(e)
		}
		batchBody := model.PODBatchCreate{RequestID: uuid.NewString(), TemplateID: id(template.TemplateID), Version: "1", Name: "Managed batch", ExecutionMode: "managed", Items: []model.PODBatchInput{{Name: "One", Bindings: []model.PODTemplateBinding{{NodeID: "A", Text: strPtr("one")}}}, {Name: "Two", Bindings: []model.PODTemplateBinding{{NodeID: "A", Text: strPtr("two")}}}}}
		q, e := svc.Quote(ctx, owner, model.PODQuoteRequest{Kind: "batch", Batch: &batchBody})
		if e != nil {
			t.Fatal(e)
		}
		batchBody.QuoteID = q.QuoteID
		before := account()
		bs := service.PODBatchService{Runs: svc}
		batch, e := bs.Create(ctx, owner, batchBody)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = bs.Create(ctx, owner, batchBody); e != nil {
			t.Fatal(e)
		}
		if account().Held != before.Held+q.Points {
			t.Fatal("batch held twice")
		}
		if ok, e := repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{}); e != nil || !ok {
			t.Fatal("batch not dispatched", e)
		}
		batch, e = bs.Get(ctx, owner, batch.BatchID)
		if e != nil {
			t.Fatal(e)
		}
		if account().Held != before.Held+q.Points {
			t.Fatal("dispatch double-held")
		}
		stopRequest := model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "管理停止本轮专用批次待派发项"}
		if _, e = service.PODOperation(ctx, admin, "batch.stop", batch.BatchID, stopRequest); e != nil {
			t.Fatal(e)
		}
		if _, e = service.PODOperation(ctx, admin, "batch.stop", batch.BatchID, stopRequest); e != nil {
			t.Fatal(e)
		}
		batch, e = bs.Stop(ctx, owner, batch.BatchID)
		if e != nil {
			t.Fatal(e)
		}
		if batch.Billing.Held*2 != q.Points || batch.Billing.Released*2 != q.Points {
			t.Fatal("stop released active item", batch.Billing)
		}
		for _, item := range batch.Items {
			if item.RunID > 0 {
				r := finish(item.RunID, okExecutor)
				if r.State != "succeeded" {
					t.Fatal("batch run failed")
				}
			}
		}
		batch, e = bs.Get(ctx, owner, batch.BatchID)
		if e != nil || batch.Billing.Held != 0 || batch.Billing.Spent != 30 {
			t.Fatal("batch summary wrong", e, batch.Billing)
		}
	})
	t.Run("node_branch_and_inline_polish_share_formal_billing", func(t *testing.T) {
		for _, mode := range []string{"node", "branch", "polish"} {
			body := project(owner, false)
			if mode == "node" {
				base := body
				base.QuoteID = quote(owner, base).QuoteID
				r := finish(create(base).RunID, okExecutor)
				body.BaseRunID = id(r.RunID)
			}
			body.Mode = mode
			body.RequestID = uuid.NewString()
			body.NodeID = "B"
			if mode == "polish" {
				body.NodeID = "A"
				pid, _ := strconv.ParseInt(body.ProjectID, 10, 64)
				p, e := service.GetPODProject(ctx, owner, pid)
				if e != nil {
					t.Fatal(e)
				}
				c, _ := json.Marshal(map[string]string{"text": "cat print", "polishModelId": id(m.ResourceID)})
				p.Graph.Nodes[0].Config = c
				p, e = service.EditPODProject(ctx, owner, pid, model.PODProjectUpdate{Revision: body.Revision, Graph: p.Graph}, model.PODGraphLimits{})
				if e != nil {
					t.Fatal(e)
				}
				body.Revision = id(p.Revision)
			}
			q := quote(owner, body)
			body.QuoteID = q.QuoteID
			r := finish(create(body).RunID, okExecutor)
			if r.State != "succeeded" || r.Billing == nil || r.Billing.Spent != 30 || len(q.Lines) != 1 {
				t.Fatal("action billing diverged", mode, r.State, r.Billing)
			}
		}
	})
	t.Run("grant_revocation_before_submit_releases_without_network", func(t *testing.T) {
		body := project(owner, false)
		body.QuoteID = quote(owner, body).QuoteID
		r := create(body)
		d, e := repository.PODCreator(ctx, owner.UserID)
		if e != nil {
			t.Fatal(e)
		}
		empty := []string{}
		creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &empty})
		count := calls.Load()
		r = finish(r.RunID, okExecutor)
		if calls.Load() != count || r.State != "failed" || r.Billing.Held != 0 || r.Billing.Spent != 0 {
			t.Fatal("revoked model submitted or hold leaked")
		}
		d, e = repository.PODCreator(ctx, owner.UserID)
		if e != nil {
			t.Fatal(e)
		}
		models := []string{id(m.ResourceID)}
		creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &models})
	})
	t.Run("batch_configuration_failure_releases_exact_item_once", func(t *testing.T) {
		body := project(owner, false)
		template, e := (service.PODTemplateService{}).Save(ctx, owner, 0, model.PODTemplateSave{RequestID: uuid.NewString(), ProjectID: body.ProjectID, ProjectRevision: body.Revision, Name: "Managed failed dispatch"})
		if e != nil {
			t.Fatal(e)
		}
		batchBody := model.PODBatchCreate{RequestID: uuid.NewString(), TemplateID: id(template.TemplateID), Version: "1", Name: "Failed dispatch", ExecutionMode: "managed", Items: []model.PODBatchInput{{Name: "One", Bindings: []model.PODTemplateBinding{{NodeID: "A", Text: strPtr("one")}}}}}
		q, e := svc.Quote(ctx, owner, model.PODQuoteRequest{Kind: "batch", Batch: &batchBody})
		if e != nil {
			t.Fatal(e)
		}
		batchBody.QuoteID = q.QuoteID
		bs := service.PODBatchService{Runs: svc}
		batch, e := bs.Create(ctx, owner, batchBody)
		if e != nil {
			t.Fatal(e)
		}
		now := modelNow()
		catalog("model.status", m.ResourceID, model.PODCatalogWrite{Revision: id(now.Revision), Status: "disabled"})
		if _, e = repository.DispatchPODBatch(ctx, limits, model.PODGraphLimits{}); e != nil {
			t.Fatal(e)
		}
		batch, e = bs.Get(ctx, owner, batch.BatchID)
		if e != nil || batch.Billing.Held != 0 || batch.Billing.Released != q.Points || batch.Items[0].State != "dispatchFailed" {
			t.Fatal("failed dispatch did not release", e, batch.Billing)
		}
		for i := 0; i < 2; i++ {
			if _, e = bs.Stop(ctx, owner, batch.BatchID); e != nil {
				t.Fatal(e)
			}
		}
		now = modelNow()
		catalog("model.status", m.ResourceID, model.PODCatalogWrite{Revision: id(now.Revision), Status: "enabled"})
	})
	t.Run("partial_images_archive_and_receipt_replay_settle_once", func(t *testing.T) {
		im := catalog("model.create", 0, model.PODCatalogWrite{Code: "managed-image-" + uuid.NewString()[:8], Name: "Managed image fixture", ProviderID: id(p.ResourceID), Kind: "image", Visible: &visible})
		version := catalog("model.version", im.ResourceID, model.PODCatalogWrite{Revision: id(im.Revision), OfficialModelID: "qwen-image-2.1-turbo", TimeoutSeconds: 20, Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 2, MaxReferenceImages: 0, Ratios: []string{"1:1"}}, CostRule: &model.PODModelRate{Version: 1, Unit: "micros", Currency: "CNY", PerImage: 100}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", PerImage: 10}})
		catalog("model.status", im.ResourceID, model.PODCatalogWrite{Revision: id(version.Revision), Status: "enabled"})
		t.Cleanup(func() {
			var row model.PODCatalogModel
			repository.DB(context.Background()).Where("model_id=?", im.ResourceID).Take(&row)
			catalog("model.status", row.ModelID, model.PODCatalogWrite{Revision: id(row.Revision), Status: "disabled"})
		})
		d, e := repository.PODCreator(ctx, owner.UserID)
		if e != nil {
			t.Fatal(e)
		}
		models := []string{id(m.ResourceID), id(im.ResourceID)}
		creator("creator.models", owner.UserID, model.PODCreatorWrite{Revision: id(d.Creator.Revision), ModelIDs: &models})
		body := project(owner, false)
		pid, _ := strconv.ParseInt(body.ProjectID, 10, 64)
		project, e := service.GetPODProject(ctx, owner, pid)
		if e != nil {
			t.Fatal(e)
		}
		conf, _ := json.Marshal(map[string]any{"modelId": id(im.ResourceID), "count": 2, "ratio": "1:1"})
		project.Graph.Nodes[1].Type = "imageGenerate"
		project.Graph.Nodes[1].Config = conf
		project, e = service.EditPODProject(ctx, owner, pid, model.PODProjectUpdate{Revision: body.Revision, Graph: project.Graph}, model.PODGraphLimits{})
		if e != nil {
			t.Fatal(e)
		}
		body.Revision = id(project.Revision)
		q := quote(owner, body)
		if q.Points != 20 {
			t.Fatal("image quote used fixed legacy price")
		}
		body.QuoteID = q.QuoteID
		r := create(body)
		var imageData bytes.Buffer
		if e := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 16, 16))); e != nil {
			t.Fatal(e)
		}
		x := executor(func(req *http.Request) (*http.Response, error) {
			var payload map[string]any
			if json.NewDecoder(req.Body).Decode(&payload) != nil || payload["model"] != "qwen-image-2.1-turbo" {
				t.Error("wrong managed image model")
			}
			return response(200, `{"request_id":"managed-partial-images","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/managed-local.png"}]}}]},"usage":{"image_count":1}}`), nil
		})
		x.Images = &http.Client{Transport: podQwenTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageData.Bytes()))}, nil
		})}
		step(r.RunID, x)
		claim, e := repository.ClaimPODNode(ctx, r.RunID, limits, time.Minute)
		if e != nil || claim == nil {
			t.Fatal("image claim unavailable", e)
		}
		count := calls.Load()
		result := x.Execute(ctx, claim)
		if result.State != "failed" || len(result.Output.AssetIDs) != 1 {
			t.Fatal("partial image not archived", result.Code)
		}
		if again := x.Execute(ctx, claim); again.State != result.State || calls.Load() != count+1 {
			t.Fatal("receipt replay submitted or lost output")
		}
		if e = repository.FinishPODNode(ctx, claim, result.State, result.Output, result.Code, result.Message); e != nil {
			t.Fatal(e)
		}
		r, e = svc.Get(ctx, owner, r.RunID)
		if e != nil || r.Billing.Spent != 10 || r.Billing.Released != 10 || r.Billing.Held != 0 {
			t.Fatal("partial image settlement wrong", e, r.Billing)
		}
		if e = repository.PODVerifyManagedAccounts(ctx); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("unknown_call_retains_hold_and_never_resubmits", func(t *testing.T) {
		body := project(owner, false)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		r := create(body)
		count := calls.Load()
		x := executor(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })
		r = finish(r.RunID, x)
		if r.State != "awaitingReconciliation" || r.Billing.Held != q.Points {
			t.Fatal("unknown call released hold")
		}
		beforeAdmin := account()
		if _, e := service.PODOperation(ctx, admin, "task.cancel", r.RunID, model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "管理取消本轮未知结果测试"}); e != nil {
			t.Fatal(e)
		}
		if after := account(); after.Held != beforeAdmin.Held || after.Spent != beforeAdmin.Spent {
			t.Fatal("admin cancellation released unknown call points")
		}
		if _, e := svc.Change(ctx, owner, r.RunID, ""); e != nil {
			t.Fatal(e)
		}
		step(r.RunID, x)
		if calls.Load() != count+1 {
			t.Fatal("unknown call resubmitted")
		}
		if _, e := svc.Quote(ctx, owner, model.PODQuoteRequest{Kind: "retry", RunID: id(r.RunID)}); e == nil {
			t.Fatal("unknown call retry permitted")
		}
	})
	t.Run("actual_overbounds_disable_model_cap_user_charge", func(t *testing.T) {
		body := project(owner, false)
		q := quote(owner, body)
		body.QuoteID = q.QuoteID
		r := create(body)
		x := executor(func(*http.Request) (*http.Response, error) {
			return response(200, `{"id":"overbound-local","choices":[{"finish_reason":"stop","message":{"content":"fixture"}}],"usage":{"prompt_tokens":2000,"completion_tokens":2000}}`), nil
		})
		r = finish(r.RunID, x)
		if r.Billing.Spent != q.Points || r.Billing.Held != 0 || modelNow().Status != "disabled" {
			t.Fatal("overbound charge not capped or model stayed enabled")
		}
		var call model.PODProviderCall
		if e := repository.DB(ctx).Where("run_id=?", r.RunID).Take(&call).Error; e != nil || call.ChargedMicros != 6000 {
			t.Fatal("actual provider usage lost", e)
		}
	})
}

func strPtr(s string) *string { return &s }
