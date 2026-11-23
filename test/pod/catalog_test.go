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

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/types"
)

func exercisePODCatalog(t *testing.T, ctx context.Context, cfg *config.Config, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string) {
	if !service.PODCatalogReady() {
		t.Fatal("catalog integration requires an independent test keyring")
	}
	transport := http.DefaultTransport
	http.DefaultTransport = podQwenTransport(func(*http.Request) (*http.Response, error) {
		t.Error("catalog must not contact a model provider")
		return nil, errors.New("network forbidden")
	})
	defer func() { http.DefaultTransport = transport }()
	t.Setenv("POD_CATALOG_TEST_KEY", "fake-catalog-env-key")
	admin := tokens["admin"]
	check := func(p map[string]json.RawMessage, want int) {
		t.Helper()
		if string(p["code"]) != strconv.Itoa(want) {
			t.Fatalf("catalog business code %s want %d", p["code"], want)
		}
	}
	decode := func(p map[string]json.RawMessage, v any) {
		t.Helper()
		if json.Unmarshal(p["data"], v) != nil {
			t.Fatal("catalog DTO invalid")
		}
	}
	write := func(method, path string, r model.PODCatalogWrite, want int) model.PODCatalogResult {
		t.Helper()
		if r.RequestID == "" {
			r.RequestID = uuid.NewString()
		}
		if r.Reason == "" {
			r.Reason = "本地模型目录自动验收，非生产配置"
		}
		p := call(method, "/pod-admin/"+path, admin, r)
		check(p, want)
		var out model.PODCatalogResult
		if want == 200 {
			decode(p, &out)
		}
		return out
	}
	provider := func(id string) model.PODProviderView {
		t.Helper()
		p := call("GET", "/pod-admin/providers/"+id, admin, nil)
		check(p, 200)
		var v model.PODProviderView
		decode(p, &v)
		return v
	}
	getModel := func(id string) model.PODCatalogModelView {
		t.Helper()
		p := call("GET", "/pod-admin/models/"+id, admin, nil)
		check(p, 200)
		var v model.PODCatalogModelView
		decode(p, &v)
		return v
	}
	id := func(v int64) string { return strconv.FormatInt(v, 10) }
	for _, path := range []string{"providers", "models", "model-defaults"} {
		check(call("GET", "/pod-admin/"+path, "", nil), 401)
		check(call("GET", "/pod-admin/"+path, tokens["pod_creator"], nil), 403)
	}
	check(call("POST", "/pod-admin/providers", tokens["pod_creator"], map[string]any{}), 403)
	suffix := uuid.NewString()[:8]
	request := model.PODCatalogWrite{RequestID: uuid.NewString(), Reason: "本地目录幂等验收", Code: "local-test-" + suffix, Name: "本地验收供应商 " + suffix, AdapterKind: "qwen-v1", Region: "cn-beijing", Endpoint: "https://dashscope.aliyuncs.com"}
	created := write("POST", "providers", request, 200)
	pid := id(created.ResourceID)
	if second := write("POST", "providers", request, 200); second != created {
		t.Fatal("replay created another provider")
	}
	request.Name = "different body"
	write("POST", "providers", request, 409)
	request.RequestID = uuid.NewString()
	write("POST", "providers", request, 409)
	var models []string
	probe := write("POST", "models", model.PODCatalogWrite{Code: "local-probe-" + suffix, Name: "配置提示验收", ProviderID: pid, Kind: "text"}, 200)
	probeID := id(probe.ResourceID)
	missing := getModel(probeID)
	if missing.ProviderCredentialConfigured || missing.CurrentVersion != 0 || missing.ProviderAdapterKind != "qwen-v1" || missing.ProviderEndpoint != request.Endpoint {
		t.Fatal("missing configuration hints incorrect")
	}
	listed := call("GET", "/pod-admin/models?providerId="+pid, admin, nil)
	check(listed, 200)
	var filtered []model.PODCatalogModelView
	if json.Unmarshal(listed["rows"], &filtered) != nil || len(filtered) != 1 || filtered[0].ModelID != probe.ResourceID || filtered[0].ProviderCredentialConfigured {
		t.Fatal("provider filter or list configuration hints incorrect")
	}
	defer func() {
		for _, mid := range models {
			m := getModel(mid)
			if m.Status == "enabled" {
				write("POST", "models/"+mid+"/status", model.PODCatalogWrite{Revision: id(m.Revision), Status: "disabled"}, 200)
			}
		}
		p := provider(pid)
		if p.Status == "enabled" {
			write("POST", "providers/"+pid+"/status", model.PODCatalogWrite{Revision: id(p.Revision), Status: "disabled"}, 200)
		}
	}()
	write("PUT", "providers/"+pid, model.PODCatalogWrite{Revision: "1", Name: "changed", Endpoint: "https://evil.test"}, 400)
	write("POST", "providers/"+pid+"/status", model.PODCatalogWrite{Revision: "1", Status: "enabled"}, 409)
	write("PUT", "providers/"+pid+"/credential", model.PODCatalogWrite{Revision: "1", SourceType: "env", EnvName: "NOT_ALLOWLISTED"}, 400)
	secret := "fake-catalog-secret-" + suffix
	credential := model.PODCatalogWrite{RequestID: uuid.NewString(), Reason: "本地虚拟凭据验收", Revision: "1", SourceType: "encrypted", Secret: secret}
	cr := write("PUT", "providers/"+pid+"/credential", credential, 200)
	if cr.Version != 1 {
		t.Fatal("credential version missing")
	}
	if !getModel(probeID).ProviderCredentialConfigured {
		t.Fatal("configured credential not reflected in model view")
	}
	write("PUT", "providers/"+pid+"/credential", credential, 200)
	credential.Secret += "different"
	write("PUT", "providers/"+pid+"/credential", credential, 409)
	var stored model.PODProviderCredential
	if err := repository.DB(ctx).Where("provider_id=? AND version=1", created.ResourceID).Take(&stored).Error; err != nil || bytes.Contains(stored.Ciphertext, []byte(secret)) || len(stored.Nonce) != 12 {
		t.Fatal("credential is not encrypted")
	}
	vault, err := modelcatalog.NewVault(cfg.POD.Catalog.KeyringFile, cfg.JWT.Secret, cfg.POD.Catalog.CredentialEnvs)
	if err != nil {
		t.Fatal("test vault unavailable")
	}
	if plain, err := vault.Open(stored.ProviderID, stored.Version, stored.KeyID, stored.Nonce, stored.Ciphertext); err != nil || plain != secret {
		t.Fatal("stored credential cannot be authenticated")
	}
	write("PUT", "providers/"+pid+"/credential", model.PODCatalogWrite{Revision: "2", SourceType: "env", EnvName: "POD_CATALOG_TEST_KEY"}, 200)
	var env model.PODProviderCredential
	if err := repository.DB(ctx).Where("provider_id=? AND version=2", created.ResourceID).Take(&env).Error; err != nil || len(env.Ciphertext) != 0 || env.EnvName != "POD_CATALOG_TEST_KEY" {
		t.Fatal("environment reference stored a secret")
	}
	write("PUT", "providers/"+pid+"/credential", model.PODCatalogWrite{Revision: "3", SourceType: "encrypted", Secret: secret}, 200)
	p := provider(pid)
	if p.CredentialVersion != 3 || p.Revision != 4 || !p.CredentialConfigured {
		t.Fatal("credential rotation state invalid")
	}
	write("POST", "providers/"+pid+"/status", model.PODCatalogWrite{Revision: "4", Status: "enabled"}, 200)
	var currentProvider model.PODProvider
	if err := repository.DB(ctx).Where("provider_id=?", created.ResourceID).Take(&currentProvider).Error; err != nil {
		t.Fatal(err)
	}
	for i, official := range []string{"qwen-flash", "qwen-plus", "qwen-image-2.1-turbo"} {
		kind := "text"
		if i == 2 {
			kind = "image"
		}
		m := write("POST", "models", model.PODCatalogWrite{Code: "local-" + suffix + "-" + strconv.Itoa(i), Name: "本地验收 " + official, ProviderID: pid, Kind: kind}, 200)
		mid := id(m.ResourceID)
		models = append(models, mid)
		r := model.PODCatalogWrite{Revision: "1", OfficialModelID: official, TimeoutSeconds: 120, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 32000, MaxOutputTokens: 2048}, CostRule: &model.PODModelRate{Version: 1, Currency: "CNY", Unit: "micros", InputPerMillion: 1000000, OutputPerMillion: 2000000}, PointRule: &model.PODModelRate{Version: 1, Unit: "points", InputPerMillion: 100, OutputPerMillion: 200}}
		if kind == "image" {
			r.Capabilities = &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 4, MaxReferenceImages: 3, Ratios: []string{"1:1", "3:4", "4:3", "9:16", "16:9"}}
			r.CostRule.InputPerMillion = 0
			r.CostRule.OutputPerMillion = 0
			r.CostRule.PerImage = 100
			r.PointRule.InputPerMillion = 0
			r.PointRule.OutputPerMillion = 0
			r.PointRule.PerImage = 1
		}
		write("POST", "models/"+mid+"/status", model.PODCatalogWrite{Revision: "1", Status: "enabled"}, 409)
		invalid := r
		invalid.OfficialModelID = "unsupported"
		write("POST", "models/"+mid+"/versions", invalid, 400)
		v := write("POST", "models/"+mid+"/versions", r, 200)
		if v.Version != 1 {
			t.Fatal("initial model version invalid")
		}
		var before model.PODCatalogModelVersion
		if err := repository.DB(ctx).Where("model_id=? AND version=1", m.ResourceID).Take(&before).Error; err != nil {
			t.Fatal(err)
		}
		r.Revision = "2"
		r.CostRule.PerImage++
		if kind == "text" {
			r.CostRule.PerImage = 0
			r.CostRule.InputPerMillion++
		}
		v = write("POST", "models/"+mid+"/versions", r, 200)
		if v.Version != 2 {
			t.Fatal("new model version not appended")
		}
		var after model.PODCatalogModelVersion
		if err := repository.DB(ctx).Where("model_version_id=?", before.ModelVersionID).Take(&after).Error; err != nil || !bytes.Equal(before.CostRuleJSON, after.CostRuleJSON) || before.SpecHash != after.SpecHash {
			t.Fatal("historical model version changed")
		}
		after = model.PODCatalogModelVersion{}
		if err := repository.DB(ctx).Where("model_id=? AND version=2", m.ResourceID).Take(&after).Error; err != nil {
			t.Fatal(err)
		}
		check(call("POST", "/pod-admin/models/"+mid+"/check-config", admin, nil), 200)
		empty, _ := modelcatalog.NewVault("", "jwt", nil)
		if err := repository.PODCatalogCheck(ctx, m.ResourceID, empty); err == nil {
			t.Fatal("missing decryption key accepted")
		}
		originalHash := after.SpecHash
		if err := repository.DB(ctx).Model(&after).Update("spec_hash", strings.Repeat("0", 64)).Error; err != nil {
			t.Fatal(err)
		}
		check(call("POST", "/pod-admin/models/"+mid+"/check-config", admin, nil), 400)
		if err := repository.DB(ctx).Model(&after).Update("spec_hash", originalHash).Error; err != nil {
			t.Fatal("restore tampered fixture failed")
		}
		check(call("POST", "/pod-admin/models/"+mid+"/check-config", admin, nil), 200)
		// Two different actions based on one revision must have one winner.
		actor := &model.LoginUser{UserID: 1, User: &model.SysUser{UserID: 1}, Permissions: []string{model.AllPermission}}
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for j := 0; j < 2; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := service.PODCatalogWrite(ctx, actor, "model.status", m.ResourceID, model.PODCatalogWrite{RequestID: uuid.NewString(), Revision: "3", Reason: "并发版本验收", Status: "enabled"})
				if err == nil {
					codes <- 200
				} else if b := errs.As(err); b != nil {
					codes <- b.Code
				} else {
					codes <- 500
				}
			}()
		}
		wg.Wait()
		close(codes)
		got := map[int]int{}
		for code := range codes {
			got[code]++
		}
		if got[200] != 1 || got[409] != 1 {
			t.Fatal("concurrent revisions did not have one winner")
		}
		write("PUT", "models/"+mid, model.PODCatalogWrite{Revision: "3", Name: "stale"}, 409)
	}
	defaults := call("GET", "/pod-admin/model-defaults", admin, nil)
	check(defaults, 200)
	var original model.PODCatalogDefaults
	decode(defaults, &original)
	text, image := models[0], models[2]
	set := write("PUT", "model-defaults", model.PODCatalogWrite{Revision: id(original.Revision), TextModelID: &text, ImageModelID: &image}, 200)
	defer func() {
		current := call("GET", "/pod-admin/model-defaults", admin, nil)
		var d model.PODCatalogDefaults
		decode(current, &d)
		if d.TextModelID != text || d.ImageModelID != image {
			t.Error("defaults changed by another action; refusing cleanup overwrite")
			return
		}
		write("PUT", "model-defaults", model.PODCatalogWrite{Revision: id(d.Revision), TextModelID: &original.TextModelID, ImageModelID: &original.ImageModelID}, 200)
	}()
	write("PUT", "model-defaults", model.PODCatalogWrite{Revision: id(set.Revision), TextModelID: &image, ImageModelID: &text}, 409)
	for _, route := range []string{"providers/" + pid, "models/" + models[0], "models/" + models[0] + "/versions", "actions/" + credential.RequestID} {
		p := call("GET", "/pod-admin/"+route, admin, nil)
		check(p, 200)
		b, _ := json.Marshal(p)
		for _, value := range []string{secret, "fake-catalog-env-key", "ciphertext", "keyId", "envName", "requestHash"} {
			if bytes.Contains(b, []byte(value)) {
				t.Fatal("catalog response leaked a credential field")
			}
		}
	}
	var audits []model.PODAdminAudit
	if err := repository.DB(ctx).Where("resource_type='provider' AND resource_id=?", created.ResourceID).Find(&audits).Error; err != nil || len(audits) < 4 {
		t.Fatal("catalog writes were not audited")
	}
	for _, a := range audits {
		b, _ := json.Marshal(a)
		if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte(stored.KeyID)) || bytes.Contains(b, []byte("POD_CATALOG_TEST_KEY")) {
			t.Fatal("audit retained credential material")
		}
	}
	exerciseCatalogActor(t, ctx, call, admin, created, credential.RequestID)
	if err := service.InitPODCatalog(config.PODCatalogConfig{}, cfg.JWT.Secret); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
			t.Error("restore catalog vault failed")
		}
	}()
	check(call("GET", "/pod-admin/models", admin, nil), 200)
	write("PUT", "providers/"+pid, model.PODCatalogWrite{Revision: "5", Name: "blocked"}, 503)
	// Restore before deferred fixture shutdown.
	if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		t.Fatal(err)
	}
}

func exerciseCatalogActor(t *testing.T, ctx context.Context, call func(string, string, string, any) map[string]json.RawMessage, admin string, p model.PODCatalogResult, request string) {
	t.Helper()
	user := model.SysUser{UserName: "podcat_" + uuid.NewString()[:8], NickName: "目录授权验收", UserType: "00", Password: "unused-no-login", Status: "0", DelFlag: "0", CreateBy: "pod-catalog-test", CreateTime: types.Now()}
	sort := 1
	role := model.SysRole{RoleName: "目录验收", RoleKey: "podcat_" + uuid.NewString()[:8], RoleSort: &sort, DataScope: model.DataScopeSelf, Status: "0", DelFlag: "0", CreateBy: "pod-catalog-test", CreateTime: types.Now()}
	defer func() {
		_ = service.RefreshOnlineUsersByID(ctx, user.UserID)
		repository.DB(ctx).Exec("DELETE FROM sys_user_role WHERE user_id=?", user.UserID)
		repository.DB(ctx).Exec("DELETE FROM sys_role_menu WHERE role_id=?", role.RoleID)
		repository.DB(ctx).Delete(&user)
		repository.DB(ctx).Delete(&role)
	}()
	if repository.DB(ctx).Create(&user).Error != nil || repository.DB(ctx).Create(&role).Error != nil {
		t.Fatal("catalog actor fixture failed")
	}
	var menus []int64
	if err := repository.DB(ctx).Table("sys_menu").Where("perms IN ?", []string{model.PODAdminAccess, "podAdmin:provider:edit", "podAdmin:provider:list"}).Pluck("menu_id", &menus).Error; err != nil || len(menus) != 3 {
		t.Fatal("catalog permissions missing")
	}
	if repository.DB(ctx).Exec("INSERT INTO sys_user_role(user_id,role_id) VALUES(?,?)", user.UserID, role.RoleID).Error != nil {
		t.Fatal("assign catalog role failed")
	}
	for _, mid := range menus {
		if repository.DB(ctx).Exec("INSERT INTO sys_role_menu(role_id,menu_id) VALUES(?,?)", role.RoleID, mid).Error != nil {
			t.Fatal("assign catalog menu failed")
		}
	}
	actor := &model.LoginUser{UserID: user.UserID, User: &user, Permissions: []string{model.PODAdminAccess, "podAdmin:provider:edit", "podAdmin:provider:list"}}
	token, err := service.CreateToken(ctx, actor)
	if err != nil {
		t.Fatal("catalog test session unavailable")
	}
	defer call("POST", "/logout", token, nil)
	if string(call("GET", "/pod-admin/actions/"+request, token, nil)["code"]) != "404" {
		t.Fatal("another actor discovered a management action")
	}
	if string(call("PUT", "/pod-admin/providers/"+strconv.FormatInt(p.ResourceID, 10)+"/credential", token, model.PODCatalogWrite{})["code"]) != "403" {
		t.Fatal("edit permission allowed credential rotation")
	}
	if err := repository.DB(ctx).Exec("DELETE FROM sys_role_menu WHERE role_id=?", role.RoleID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.PODCatalogWrite(ctx, actor, "provider.update", p.ResourceID, model.PODCatalogWrite{RequestID: uuid.NewString(), Revision: "5", Reason: "撤权验收", Name: "must not write"})
	if b := errs.As(err); b == nil || b.Code != 403 {
		t.Fatal("transaction trusted a stale permission snapshot")
	}
}
