// Package podtest runs only against an explicitly supplied isolated POD config.
package podtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/validate"
)

func TestPODFoundation(t *testing.T) {
	path := os.Getenv("POD_TEST_CONFIG")
	if path == "" {
		t.Skip("set POD_TEST_CONFIG to an initialized isolated POD config")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidatePOD(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if err := repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		t.Fatal(err)
	}
	defer redisx.Close()
	if online, err := redisx.C().Exists(ctx, "pod:worker:heartbeat:"+cfg.POD.InstanceID).Result(); err != nil || online != 0 {
		t.Fatal("请先停止本地 POD Worker，再运行可控集成测试；不停止其他进程")
	}
	owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
	if err != nil || owner != cfg.POD.InstanceID {
		t.Fatal("Redis instance mismatch")
	}
	service.InitToken(cfg.JWT)
	if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		t.Fatal("catalog configuration unavailable")
	}
	service.InitCaptcha(cfg.Captcha.Type)
	if err := validate.Register(); err != nil {
		t.Fatal(err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine := router.New(cfg)
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = middleware.ShutdownOperLogPool(drainCtx)
		_ = service.ShutdownPermissionRevocationQueue(drainCtx)
	}()
	var credentials struct {
		State    string `json:"state"`
		Accounts []struct {
			UserName string `json:"userName"`
			Password string `json:"password"`
		} `json:"accounts"`
	}
	accountPath := filepath.Join(filepath.Dir(filepath.Dir(path)), ".local", "accounts.json")
	contents, err := os.ReadFile(accountPath)
	if err != nil {
		t.Fatal("local test account file unavailable")
	}
	if json.Unmarshal(contents, &credentials) != nil || credentials.State != "ready" {
		t.Fatal("local test accounts are not ready")
	}
	call := func(method, path, token string, body any) map[string]json.RawMessage {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		engine.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("%s: unexpected HTTP status %d", path, rr.Code)
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(rr.Body.Bytes(), &payload) != nil {
			t.Fatal("non-JSON response")
		}
		if (strings.HasPrefix(path, "/pod/usage") || strings.HasPrefix(path, "/pod-admin/")) && string(payload["code"]) == "200" && rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private usage response must not be cached")
		}
		return payload
	}
	assertCode := func(payload map[string]json.RawMessage, want int) {
		t.Helper()
		var code int
		_ = json.Unmarshal(payload["code"], &code)
		if code != want {
			t.Fatalf("business code=%d want=%d", code, want)
		}
	}
	tokens := map[string]string{}
	for _, account := range credentials.Accounts {
		captchaID := "pod-foundation-" + uuid.NewString()
		if err := redisx.C().Set(ctx, redisx.CaptchaKey(captchaID), "2468", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		payload := call(http.MethodPost, "/login", "", model.LoginBody{Username: account.UserName, Password: account.Password, UUID: captchaID, Code: "2468"})
		if string(payload["code"]) != "200" { t.Fatalf("test login %s: code=%s message=%s",account.UserName,payload["code"],payload["msg"]) }
		assertCode(payload, 200)
		var token string
		_ = json.Unmarshal(payload["token"], &token)
		if token == "" {
			t.Fatal("login token missing")
		}
		tokens[account.UserName] = token
	}
	defer func() {
		for _, token := range tokens {
			call(http.MethodPost, "/logout", token, nil)
		}
	}()
	var first, second model.PODBootstrap
	t.Run("authentication_required", func(t *testing.T) { assertCode(call("GET", "/pod/bootstrap", "", nil), 401) })
	t.Run("creator_bootstrap_and_honest_capabilities", func(t *testing.T) {
		payload := call("GET", "/pod/bootstrap", tokens["pod_creator"], nil)
		assertCode(payload, 200)
		if json.Unmarshal(payload["data"], &first) != nil {
			t.Fatal("bootstrap contract mismatch")
		}
		if first.Workspace.WorkspaceID <= 0 || len(first.NodeTypes) != 7 {
			t.Fatal("workspace or node definitions missing")
		}
		if first.Capabilities.Execution != (cfg.POD.Execution.Mode == "simulation") || !first.Capabilities.ProjectEditing || !first.Capabilities.Assets || first.AssetLimits.MaxUploadBytes <= 0 {
			t.Fatal("unimplemented capabilities were enabled")
		}
	})
	t.Run("two_personal_workspaces", func(t *testing.T) {
		payload := call("GET", "/pod/bootstrap", tokens["pod_creator_b"], nil)
		assertCode(payload, 200)
		if json.Unmarshal(payload["data"], &second) != nil {
			t.Fatal("bootstrap contract mismatch")
		}
		if first.Workspace.WorkspaceID == second.Workspace.WorkspaceID || first.UserID == second.UserID {
			t.Fatal("workspace isolation failed")
		}
	})
	t.Run("client_cannot_choose_another_workspace", func(t *testing.T) {
		payload := call("GET", "/pod/bootstrap?workspaceId="+strconv.FormatInt(second.Workspace.WorkspaceID, 10), tokens["pod_creator"], nil)
		assertCode(payload, 200)
		var actual model.PODBootstrap
		_ = json.Unmarshal(payload["data"], &actual)
		if actual.Workspace.WorkspaceID != first.Workspace.WorkspaceID {
			t.Fatal("client selected another owner")
		}
	})
	t.Run("creator_cannot_read_admin_users", func(t *testing.T) { assertCode(call("GET", "/system/user/list", tokens["pod_creator"], nil), 403) })
	t.Run("usage_endpoint_is_private_and_paginated", func(t *testing.T) {
		assertCode(call("GET", "/pod/usage", "", nil), 401)
		assertCode(call("GET", "/pod/usage", tokens["admin"], nil), 403)
		payload := call("GET", "/pod/usage?workspaceId="+strconv.FormatInt(second.Workspace.WorkspaceID, 10)+"&userId="+second.UserID+"&pageSize=999", tokens["pod_creator"], nil)
		assertCode(payload, 200)
		var usage model.PODUsageView
		if json.Unmarshal(payload["data"], &usage) != nil || usage.Rows == nil || len(usage.Rows) > 100 || usage.BudgetScope != "instanceCumulative" {
			t.Fatal("invalid usage response")
		}
		for _, row := range usage.Rows {
			var owned int64
			if err := repository.DB(ctx).Table("pod_run").Where("run_id=? AND workspace_id=?", row.RunID, first.Workspace.WorkspaceID).Count(&owned).Error; err != nil || owned != 1 {
				t.Fatal("usage spoofed another workspace")
			}
		}
	})
	t.Run("creator_cannot_read_admin_roles", func(t *testing.T) { assertCode(call("GET", "/system/role/list", tokens["pod_creator"], nil), 403) })
	t.Run("admin_does_not_bypass_membership", func(t *testing.T) { assertCode(call("GET", "/pod/bootstrap", tokens["admin"], nil), 403) })
	t.Run("admin_foundation", func(t *testing.T) { exercisePODAdmin(t, ctx, call, tokens) })
	t.Run("model_catalog", func(t *testing.T) { exercisePODCatalog(t, ctx, cfg, call, tokens) })
	t.Run("creator_policy", func(t *testing.T) { exercisePODCreators(t, ctx, cfg, call, tokens) })
	t.Run("managed_execution", func(t *testing.T) { exercisePODManaged(t, ctx, cfg) })
	t.Run("operations_official", func(t *testing.T) { exercisePODOperationsOfficial(t, ctx, cfg, call, tokens) })
	t.Run("membership_revocation_is_immediate", func(t *testing.T) {
		q := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id = ?", first.Workspace.WorkspaceID)
		if err := q.Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Table("pod_workspace_member").Where("workspace_id = ?", first.Workspace.WorkspaceID).Update("status", "0").Error; err != nil {
				t.Error("restore membership failed")
			}
		}()
		assertCode(call("GET", "/pod/bootstrap", tokens["pod_creator"], nil), 403)
		assertCode(call("GET", "/pod/usage", tokens["pod_creator"], nil), 403)
	})
	t.Run("workspace_disable_is_immediate", func(t *testing.T) {
		if err := repository.DB(ctx).Table("pod_workspace").Where("workspace_id = ?", first.Workspace.WorkspaceID).Update("status", "1").Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := repository.DB(ctx).Table("pod_workspace").Where("workspace_id = ?", first.Workspace.WorkspaceID).Update("status", "0").Error; err != nil {
				t.Error("restore workspace failed")
			}
		}()
		assertCode(call("GET", "/pod/bootstrap", tokens["pod_creator"], nil), 403)
		assertCode(call("GET", "/pod/usage", tokens["pod_creator"], nil), 403)
	})
	t.Run("redis_owner_cannot_be_overwritten", func(t *testing.T) {
		if redisx.ClaimPODInstance(ctx, "another-pod-instance") == nil {
			t.Fatal("foreign owner accepted")
		}
		actual, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
		if err != nil || actual != owner {
			t.Fatal("owner changed")
		}
	})
	t.Run("schema_instance_mismatch_rejected", func(t *testing.T) {
		if repository.VerifyPODSchema(ctx, "another-pod-instance") == nil {
			t.Fatal("foreign database accepted")
		}
	})
	t.Run("existing_database_not_reinstalled", func(t *testing.T) {
		exists, err := repository.PODDatabaseExists(ctx, cfg)
		if err != nil || !exists {
			t.Fatal("database existence check failed")
		}
		err = repository.InstallPODFoundation(ctx, cfg, strings.Repeat("x", 60), []repository.PODSeedUser{{UserName: "unused-a"}, {UserName: "unused-b"}})
		if err == nil || !strings.Contains(err.Error(), "已存在") {
			t.Fatal("expected an existing-database refusal")
		}
	})
	t.Run("captcha_remains_enabled", func(t *testing.T) {
		payload := call("GET", "/captchaImage", "", nil)
		assertCode(payload, 200)
		var enabled bool
		_ = json.Unmarshal(payload["captchaEnabled"], &enabled)
		if !enabled {
			t.Fatal("captcha was disabled")
		}
		var id string
		_ = json.Unmarshal(payload["uuid"], &id)
		_ = redisx.C().Del(ctx, redisx.CaptchaKey(id)).Err()
	})
	t.Run("project_stage2", func(t *testing.T) { exercisePODProjects(t, ctx, call, tokens, first) })
	t.Run("asset_stage3", func(t *testing.T) { exercisePODAssets(t, ctx, engine, cfg, call, tokens, first) })
	t.Run("execution_stage4", func(t *testing.T) { exercisePODRuns(t, ctx, cfg, call) })
	t.Run("qwen_stage5", func(t *testing.T) { exercisePODQwen(t, ctx, cfg) })
	t.Run("creator_usage", func(t *testing.T) { exercisePODUsage(t, ctx) })
	t.Run("image_stage6", func(t *testing.T) { exercisePODImageWork(t, ctx, cfg) })
	t.Run("templates_stage7", func(t *testing.T) { exercisePODTemplates(t, ctx, cfg, call, engine) })
	t.Run("bounded_batches", func(t *testing.T) { exercisePODBatches(t, ctx, cfg, call) })
}
