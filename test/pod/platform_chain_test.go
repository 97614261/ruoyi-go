package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
)

// Reuse the managed fixture's isolated accounts/model and network-blocking adapter.
// The HTTP path must preserve the same ownership and ledger facts across modules.
func exercisePODPlatformChain(t *testing.T, ctx context.Context, cfg *config.Config, owner, other *model.SysUser, modelID int64, executor *service.PODQwenExecutor, limits model.PODExecutionLimits) {
	t.Helper()
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	managed := *cfg
	managed.POD.Execution.Mode = "managed"
	engine := router.New(&managed)
	session := func(userID int64, permissions []string) string {
		t.Helper()
		u, err := repository.SelectUserByID(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		login := &model.LoginUser{UserID: userID, User: u, Permissions: permissions}
		token, err := service.CreateToken(ctx, login)
		if err != nil {
			t.Fatal("create local chain session", err)
		}
		t.Cleanup(func() {
			if err := service.Logout(context.Background(), login.Token); err != nil {
				t.Error("close local chain session", err)
			}
		})
		return token
	}
	admin := session(1, []string{"*:*:*"})
	creator := session(owner.UserID, []string{model.PODStudioAccess})
	foreign := session(other.UserID, []string{model.PODStudioAccess})
	serve := func(method, path, token, contentType string, data []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(data)).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", contentType)
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		return rr
	}
	decode := func(rr *httptest.ResponseRecorder, want int) map[string]json.RawMessage {
		t.Helper()
		var body map[string]json.RawMessage
		if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &body) != nil {
			t.Fatal("chain API did not return its JSON envelope", rr.Code)
		}
		if string(body["code"]) != strconv.Itoa(want) {
			t.Fatalf("chain business code %s want %d: %s", body["code"], want, body["msg"])
		}
		return body
	}
	call := func(method, path, token string, data any, want int, out any) map[string]json.RawMessage {
		t.Helper()
		var raw []byte
		if data != nil {
			var err error
			raw, err = json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
		}
		rr := serve(method, path, token, "application/json", raw)
		body := decode(rr, want)
		if want == 200 && rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("chain response can cache private data")
		}
		if out != nil && json.Unmarshal(body["data"], out) != nil {
			t.Fatal("chain response DTO mismatch")
		}
		return body
	}
	account, err := repository.PODPoints(ctx, owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	before := *account
	var template model.PODCatalogResult
	var versionID int64
	var project model.PODProjectView
	var run model.PODRunView
	var quote model.PODQuoteView
	var use model.PODTemplateUse
	path := ""
	t.Log("publish and copy with a platform image over HTTP")
	{
		var imageData, upload bytes.Buffer
		if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 24, 24))); err != nil {
			t.Fatal(err)
		}
		writer := multipart.NewWriter(&upload)
		part, err := writer.CreateFormFile("file", "platform-chain.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(imageData.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		var asset model.PODCatalogResult
		result := decode(serve("POST", "/pod-admin/catalog/assets?requestId="+uuid.NewString()+"&licenseNote=Original-local-test-image", admin, writer.FormDataContentType(), upload.Bytes()), 200)
		if json.Unmarshal(result["data"], &asset) != nil || asset.ResourceID <= 0 {
			t.Fatal("platform image upload DTO mismatch")
		}
		conf, _ := json.Marshal(map[string]string{"modelId": id(modelID)})
		graph := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{
			{ID: "text", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"platform-default-marker"}`)},
			{ID: "reference", Type: "imageInput", Version: 1, Config: json.RawMessage(`{"assetIds":["` + id(asset.ResourceID) + `"]}`)},
			{ID: "generate", Type: "textGenerate", Version: 1, Config: conf},
		}, Edges: []model.PODEdge{{ID: "text-generate", Source: "text", SourcePort: "text", Target: "generate", TargetPort: "text"}}}
		call("POST", "/pod-admin/catalog/templates", admin, model.PODOfficialWrite{RequestID: uuid.NewString(), Reason: "平台链路专用测试", Code: "chain-" + uuid.NewString()[:8], Name: "平台链路模板", Graph: &graph}, 200, &template)
		path = "/pod-admin/catalog/templates/" + id(template.ResourceID)
		t.Cleanup(func() {
			u := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{"*:*:*"}}
			svc := service.NewPODOfficialService(cfg.POD.StorageRoot, model.PODGraphLimits{}, model.PODAssetLimits{})
			d, err := svc.Get(context.Background(), u, template.ResourceID, 0, true)
			if err == nil && d.Template.Status == "published" {
				_, err = svc.Write(context.Background(), u, template.ResourceID, "official.unpublish", model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(d.Template.Revision), Reason: "链路测试结束下架保留"})
			}
			if err != nil {
				t.Error(err)
			}
		})
		var detail model.PODOfficialDetail
		call("GET", path, admin, nil, 200, &detail)
		if detail.Snapshot == nil {
			t.Fatal("official draft has no version snapshot")
		}
		versionID = detail.Snapshot.VersionID
		call("GET", "/pod/catalog/templates/"+id(template.ResourceID), creator, nil, 404, nil)
		publish := model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(template.Revision), VersionID: id(versionID), Reason: "本地链路发布"}
		call("POST", path+"/publish", admin, publish, 200, &template)
		var replay model.PODCatalogResult
		call("POST", path+"/publish", admin, publish, 200, &replay)
		if replay.ResourceID != template.ResourceID || replay.Revision != template.Revision {
			t.Fatal("publish replay created another mutation")
		}
		var audits []model.PODAdminAudit
		if err := repository.DB(ctx).Where("action_type=? AND resource_type=? AND resource_id=?", "official.publish", "officialTemplate", template.ResourceID).Find(&audits).Error; err != nil || len(audits) != 1 {
			t.Fatal("publish did not create exactly one audit", err)
		}
		var audit model.PODAdminAuditView
		call("GET", "/pod-admin/audits/"+id(audits[0].AuditID), admin, nil, 200, &audit)
		if audit.ActorUserID != 1 || audit.ResourceID != template.ResourceID || audit.ActionType != "official.publish" || audit.After == nil || audit.After.Operation == nil || audit.After.Operation.State != "published" {
			t.Fatal("publish audit lost its actor, target or resulting state")
		}
		use = model.PODTemplateUse{RequestID: uuid.NewString(), Version: id(versionID), Name: "private-chain-project", Bindings: []model.PODTemplateBinding{{NodeID: "text", Text: strPtr("creator-private-chain-marker")}}}
		var copied, again model.PODOfficialUseView
		usePath := "/pod/catalog/templates/" + id(template.ResourceID) + "/use"
		call("POST", usePath, creator, use, 200, &copied)
		call("POST", usePath, creator, use, 200, &again)
		if copied.State != "completed" || copied.ProjectID <= 0 || copied.ProjectID != again.ProjectID {
			t.Fatal("official copy did not produce one private project")
		}
		call("GET", "/pod/projects/"+id(copied.ProjectID), creator, nil, 200, &project)
		call("GET", "/pod/projects/"+id(copied.ProjectID), foreign, nil, 404, nil)
		raw, _ := json.Marshal(project.Graph)
		if !bytes.Contains(raw, []byte("creator-private-chain-marker")) || bytes.Contains(raw, []byte("platform-default-marker")) {
			t.Fatal("input binding did not reach the private project")
		}
		var refs []model.PODProjectAssetRef
		if err := repository.DB(ctx).Table("pod_project_asset_ref").Where("project_id=?", project.ProjectID).Find(&refs).Error; err != nil || len(refs) != 1 {
			t.Fatal("private image reference not copied", err)
		}
		rr := serve("GET", "/pod/assets/"+id(refs[0].AssetID)+"/content", creator, "", nil)
		if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "image/") || !strings.Contains(rr.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("copied image unavailable or publicly cacheable")
		}
		decode(serve("GET", "/pod/assets/"+id(refs[0].AssetID)+"/content", foreign, "", nil), 404)
	}
	t.Log("quote, execute, settle and report the same facts")
	{
		body := model.PODRunCreate{ProjectID: id(project.ProjectID), Revision: id(project.Revision), Mode: "workflow", ExecutionMode: "managed", RequestID: uuid.NewString()}
		call("POST", "/pod/execution-quotes", creator, model.PODQuoteRequest{Kind: "run", Run: &body}, 200, &quote)
		if quote.Points <= 20 {
			t.Fatal("quote did not reserve the declared nonzero upper bound")
		}
		body.QuoteID = quote.QuoteID
		call("POST", "/pod/runs", creator, body, 200, &run)
		var replay model.PODRunView
		call("POST", "/pod/runs", creator, body, 200, &replay)
		if replay.RunID != run.RunID {
			t.Fatal("submit replay created a second billed run")
		}
		for i := 0; i < 8 && run.State != "succeeded"; i++ {
			if _, err := service.ExecutePODQwenStep(ctx, run.RunID, limits, executor); err != nil {
				t.Fatal(err)
			}
			call("GET", "/pod/runs/"+id(run.RunID), creator, nil, 200, &run)
		}
		if run.State != "succeeded" || run.Billing.Spent != 20 || run.Billing.Held != 0 || run.Billing.Released != quote.Points-20 {
			t.Fatal("run and formal points settlement differ", run.State, run.Billing)
		}
		if _, err := service.ExecutePODQwenStep(ctx, run.RunID, limits, executor); err != nil {
			t.Fatal(err)
		}
		account, err := repository.PODPoints(ctx, owner.UserID)
		if err != nil || account.Available != before.Available-20 || account.Spent != before.Spent+20 || account.Held != before.Held {
			t.Fatal("account did not settle exactly once", err)
		}
		var calls int64
		if err = repository.DB(ctx).Table("pod_provider_call").Where("run_id=?", run.RunID).Count(&calls).Error; err != nil || calls != 1 {
			t.Fatal("duplicate provider call", err)
		}
		rows := call("GET", "/pod-admin/usage?userId="+id(owner.UserID)+"&modelId="+id(modelID), admin, nil, 200, nil)
		var usage []model.PODUsageGroup
		if json.Unmarshal(rows["rows"], &usage) != nil || len(usage) != 1 || usage[0].Calls != 1 || usage[0].PointsSpent != 20 || usage[0].ChargedMicros != 20 || usage[0].PointsHeld != 0 {
			t.Fatal("admin usage differs from settled call")
		}
		meta := call("GET", "/pod-admin/runs/"+id(run.RunID), admin, nil, 200, nil)
		raw, _ := json.Marshal(meta)
		for _, secret := range []string{"creator-private-chain-marker", "cat graphic", "private-chain-project", "storageKey", "receiptJSON"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("admin task metadata exposed private content")
			}
		}
		call("GET", "/pod-admin/private/runs/"+id(run.RunID), admin, nil, 403, nil)
	}
	t.Log("unpublish preserves the copy and revocation blocks a new quote")
	{
		call("POST", path+"/unpublish", admin, model.PODOfficialWrite{RequestID: uuid.NewString(), Revision: id(template.Revision), Reason: "链路测试下架保留"}, 200, &template)
		var replay model.PODOfficialUseView
		usePath := "/pod/catalog/templates/" + id(template.ResourceID) + "/use"
		call("POST", usePath, creator, use, 200, &replay)
		if replay.ProjectID != project.ProjectID {
			t.Fatal("unpublish invalidated an existing private copy")
		}
		fresh := use
		fresh.RequestID = uuid.NewString()
		call("POST", usePath, creator, fresh, 404, nil)
		call("GET", "/pod/runs/"+id(run.RunID), foreign, nil, 404, nil)
		var creatorDetail model.PODCreatorDetail
		call("GET", "/pod-admin/creators/"+id(owner.UserID), admin, nil, 200, &creatorDetail)
		previous := creatorDetail.ModelIDs
		empty := []string{}
		call("PUT", "/pod-admin/creators/"+id(owner.UserID)+"/models", admin, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(creatorDetail.Creator.Revision), Reason: "仅撤回本轮专用模型范围", ModelIDs: &empty}, 200, nil)
		defer func() {
			call("GET", "/pod-admin/creators/"+id(owner.UserID), admin, nil, 200, &creatorDetail)
			modelIDs := append([]string{}, previous...)
			call("PUT", "/pod-admin/creators/"+id(owner.UserID)+"/models", admin, model.PODCreatorWrite{RequestID: uuid.NewString(), Revision: id(creatorDetail.Creator.Revision), Reason: "恢复本轮套件原模型范围以继续专项", ModelIDs: &modelIDs}, 200, nil)
		}()
		body := model.PODRunCreate{ProjectID: id(project.ProjectID), Revision: id(project.Revision), Mode: "workflow", ExecutionMode: "managed", RequestID: uuid.NewString()}
		call("POST", "/pod/execution-quotes", creator, model.PODQuoteRequest{Kind: "run", Run: &body}, 409, nil)
		call("GET", "/pod/runs/"+id(run.RunID), creator, nil, 200, nil)
	}
	for _, verify := range []func(context.Context) error{repository.PODVerifyCreatorAccounts, repository.PODVerifyManagedAccounts} {
		if err := verify(ctx); err != nil {
			t.Fatal("platform chain broke recovery invariants", err)
		}
	}
	if _, err := repository.PODVerifyOfficialData(ctx); err != nil {
		t.Fatal(err)
	}
}
