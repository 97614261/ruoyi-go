package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/page"
)

var catalogVault atomic.Pointer[modelcatalog.Vault]
var catalogCode = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
var catalogModelCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)

func InitPODCatalog(c config.PODCatalogConfig, jwt string) error {
	v, err := modelcatalog.NewVault(c.KeyringFile, jwt, c.CredentialEnvs)
	if err != nil {
		return err
	}
	catalogVault.Store(v)
	return nil
}
func CatalogPermission(op string) string {
	switch op {
	case "official.create", "official.update", "official.version", "official.asset":
		return "podAdmin:template:edit"
	case "official.publish", "official.unpublish":
		return "podAdmin:template:publish"
	case "task.cancel", "batch.stop":
		return "podAdmin:task:cancel"
	case "private.grant", "private.revoke":
		return "podAdmin:privacy:grant"
	case "private.read":
		return "podAdmin:private:read"
	case "creator.create":
		return "podAdmin:creator:add"
	case "creator.policy":
		return "podAdmin:creator:edit"
	case "creator.models":
		return "podAdmin:creator:grant"
	case "points.adjust":
		return "podAdmin:points:adjust"
	case "provider.create", "provider.update", "provider.status":
		return "podAdmin:provider:edit"
	case "provider.credential":
		return "podAdmin:provider:credential"
	case "model.create", "model.update", "model.version", "model.status", "defaults.update":
		return "podAdmin:model:edit"
	}
	return ""
}
func PODCatalogReady() bool { return catalogVault.Load().Ready() }

func validateCatalogWrite(op string, id int64, r model.PODCatalogWrite) (int64, error) {
	u, err := uuid.Parse(r.RequestID)
	if err != nil || u == uuid.Nil || u.String() != r.RequestID || !catalogText(r.Reason, 200) {
		return 0, errs.WithCode(400, "请求编号须为规范 UUID，操作原因须为 1—200 字普通文本")
	}
	allowed := "requestId revision reason "
	switch op {
	case "provider.create":
		allowed += "code name adapterKind region endpoint"
	case "provider.update":
		allowed += "name endpoint"
	case "provider.status", "model.status":
		allowed += "status"
	case "provider.credential":
		allowed += "sourceType secret envName"
	case "model.create":
		allowed += "code name providerId kind visible sort"
	case "model.update":
		allowed += "name visible sort"
	case "model.version":
		allowed += "officialModelId timeoutSeconds capabilities costRule pointRule"
	case "defaults.update":
		allowed += "textModelId imageModelId"
	default:
		return 0, errs.WithCode(400, "模型管理操作无效")
	}
	value := reflect.ValueOf(r)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		key := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if !value.Field(i).IsZero() && !strings.Contains(" "+allowed+" ", " "+key+" ") {
			return 0, errs.WithCode(400, "本操作包含不支持的字段")
		}
	}
	revision := int64(0)
	if op != "provider.create" && op != "model.create" {
		revision, err = PODPositiveID(r.Revision)
		if err != nil {
			return 0, err
		}
		if id <= 0 && op != "defaults.update" {
			return 0, errs.WithCode(400, "记录编号无效")
		}
	} else if r.Revision != "" {
		return 0, errs.WithCode(400, "新增不接受版本号")
	}
	if op == "provider.create" && !catalogCode.MatchString(r.Code) {
		return 0, errs.WithCode(400, "供应商编码须为 3—64 位，以小写字母开头，仅支持小写字母、数字、下划线 (_) 和半角连字符 (-)")
	}
	if op == "model.create" && !catalogModelCode.MatchString(r.Code) {
		return 0, errs.WithCode(400, "模型编码须为 3—64 位，以小写字母开头，仅支持小写字母、数字、下划线 (_)、半角连字符 (-) 和点号 (.)")
	}
	if strings.HasSuffix(op, ".create") || op == "provider.update" || op == "model.update" {
		if !catalogText(r.Name, 100) {
			return 0, errs.WithCode(400, "名称须为 1—100 字普通文本")
		}
	}
	if r.Sort != nil && (*r.Sort < 0 || *r.Sort > 1_000_000) {
		return 0, errs.WithCode(400, "排序须在 0—1000000 之间")
	}
	if strings.HasSuffix(op, ".status") && r.Status != "enabled" && r.Status != "disabled" {
		return 0, errs.WithCode(400, "状态必须为 enabled 或 disabled")
	}
	if op == "provider.create" {
		if err := modelcatalog.ValidateProvider(r.AdapterKind, r.Region, r.Endpoint); err != nil {
			return 0, errs.WithCode(400, err.Error())
		}
	}
	if op == "model.create" {
		if _, err := PODPositiveID(r.ProviderID); err != nil {
			return 0, err
		}
		if r.Kind != "text" && r.Kind != "image" {
			return 0, errs.WithCode(400, "模型类型必须为 text 或 image")
		}
	}
	if op == "provider.credential" {
		if r.SourceType == "encrypted" {
			if !modelcatalog.ValidSecret(r.Secret) || r.EnvName != "" || strings.Contains(r.Reason, r.Secret) {
				return 0, errs.WithCode(400, "加密凭据须为 8—512 位无空格 ASCII，不接受环境引用或将密钥填入原因")
			}
		}
		if r.SourceType == "env" {
			if r.Secret != "" || r.EnvName == "" {
				return 0, errs.WithCode(400, "环境引用不接受明文密钥")
			}
			if _, err := catalogVault.Load().ResolveEnv(r.EnvName); err != nil {
				return 0, errs.WithCode(400, err.Error())
			}
		}
		if r.SourceType != "env" && r.SourceType != "encrypted" {
			return 0, errs.WithCode(400, "凭据来源必须为 encrypted 或 env")
		}
	}
	if op == "model.version" && (r.Capabilities == nil || r.CostRule == nil || r.PointRule == nil) {
		return 0, errs.WithCode(400, "必须填写完整模型版本")
	}
	if op == "defaults.update" {
		for _, raw := range []*string{r.TextModelID, r.ImageModelID} {
			if raw == nil {
				return 0, errs.WithCode(400, "请明确指定两类默认值，空字符串表示清除")
			}
			if *raw != "" {
				if _, err := PODPositiveID(*raw); err != nil {
					return 0, err
				}
			}
		}
	}
	return revision, nil
}
func catalogText(value string, max int) bool {
	if value != strings.TrimSpace(value) || value == "" || utf8.RuneCountInString(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) || c == '<' || c == '>' {
			return false
		}
	}
	return true
}

func PODCatalogWrite(ctx context.Context, user *model.LoginUser, op string, id int64, r model.PODCatalogWrite) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	perm := CatalogPermission(op)
	if perm == "" {
		return nil, errs.WithCode(400, "模型管理操作无效")
	}
	if err := requirePODAdmin(user, perm); err != nil {
		return nil, err
	}
	revision, err := validateCatalogWrite(op, id, r)
	if err != nil {
		if auditErr := repository.PODCatalogRejected(ctx, user.UserID, op, logx.TraceID(ctx)); auditErr != nil {
			return nil, errs.WithCode(500, "模型管理校验未通过且审计不可用")
		}
		return nil, err
	}
	vault := catalogVault.Load()
	if !vault.Ready() {
		if err := repository.PODCatalogRejected(ctx, user.UserID, op, logx.TraceID(ctx)); err != nil {
			return nil, errs.WithCode(500, "模型管理配置与审计不可用")
		}
		return nil, errs.WithCode(503, "模型主密钥尚未配置，管理查询仍可使用")
	}
	hash, err := vault.Fingerprint(op, struct {
		ID   int64
		Body model.PODCatalogWrite
	}{id, r})
	if err != nil {
		return nil, errs.WithCode(503, "模型管理幂等指纹不可用")
	}
	return repository.PODCatalogMutate(ctx, repository.PODCatalogMutation{ActorID: user.UserID, ID: id, Revision: revision, Op: op, Permission: perm, Hash: hash, Trace: logx.TraceID(ctx), Request: r}, vault)
}

func PODCatalogList(ctx context.Context, user *model.LoginUser, providers bool, f model.PODCatalogFilter, pg page.Query) (any, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	perm := "podAdmin:model:list"
	if providers {
		perm = "podAdmin:provider:list"
	}
	if err := requirePODAdmin(user, perm); err != nil {
		return nil, 0, err
	}
	if (f.Status != "" && f.Status != "enabled" && f.Status != "disabled") || (f.Kind != "" && f.Kind != "text" && f.Kind != "image") || utf8.RuneCountInString(f.Name) > 100 || len(f.Code) > 64 {
		return nil, 0, errs.WithCode(400, "查询条件无效")
	}
	if providers {
		return repository.PODCatalogProviders(ctx, f, normalizePODPage(pg))
	}
	return repository.PODCatalogModels(ctx, f, normalizePODPage(pg))
}
func PODCatalogGet(ctx context.Context, user *model.LoginUser, providers bool, id int64) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	perm := "podAdmin:model:query"
	if providers {
		perm = "podAdmin:provider:query"
	}
	if err := requirePODAdmin(user, perm); err != nil {
		return nil, err
	}
	if providers {
		return repository.PODCatalogProvider(ctx, id)
	}
	return repository.PODCatalogModelGet(ctx, id)
}
func PODCatalogVersions(ctx context.Context, user *model.LoginUser, id int64, pg page.Query) ([]model.PODCatalogVersionView, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, "podAdmin:model:query"); err != nil {
		return nil, 0, err
	}
	rows, total, err := repository.PODCatalogVersions(ctx, id, normalizePODPage(pg))
	if err != nil {
		return nil, 0, err
	}
	views := make([]model.PODCatalogVersionView, 0, len(rows))
	for _, r := range rows {
		v := model.PODCatalogVersionView{ModelVersionID: r.ModelVersionID, ModelID: r.ModelID, Version: r.Version, OfficialModelID: r.OfficialModelID, AdapterKind: r.AdapterKind, AdapterVersion: r.AdapterVersion, Endpoint: r.Endpoint, Region: r.Region, TimeoutSeconds: r.TimeoutSeconds, SpecHash: r.SpecHash, CreateTime: r.CreateTime}
		if json.Unmarshal(r.CapabilitiesJSON, &v.Capabilities) != nil || json.Unmarshal(r.CostRuleJSON, &v.CostRule) != nil || json.Unmarshal(r.PointRuleJSON, &v.PointRule) != nil {
			return nil, 0, errors.New("模型版本格式无效")
		}
		views = append(views, v)
	}
	return views, total, nil
}
func PODCatalogGetDefaults(ctx context.Context, user *model.LoginUser) (*model.PODCatalogDefaults, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, "podAdmin:model:query"); err != nil {
		return nil, err
	}
	return repository.PODCatalogDefaults(ctx)
}
func PODCatalogGetAction(ctx context.Context, user *model.LoginUser, request string) (*model.PODCatalogResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, model.PODAdminAccess); err != nil {
		return nil, err
	}
	u, err := uuid.Parse(request)
	if err != nil || u == uuid.Nil || u.String() != request {
		return nil, errs.WithCode(400, "请求编号无效")
	}
	a, err := repository.PODCatalogAction(ctx, user.UserID, request)
	if err != nil {
		return nil, err
	}
	perm := CatalogPermission(a.ActionType)
	if perm == "" {
		return nil, errs.WithCode(404, "未找到本阶段管理操作")
	}
	if err := requirePODAdmin(user, perm); err != nil {
		return nil, err
	}
	var result model.PODCatalogResult
	if a.State != "completed" || json.Unmarshal(a.ResultJSON, &result) != nil || result.RequestID != request || result.ActionType != a.ActionType || result.ResourceID != a.ResourceID || result.Revision <= 0 {
		return nil, errors.New("管理操作结果不可读")
	}
	return &result, nil
}
func PODCatalogCheck(ctx context.Context, user *model.LoginUser, id int64) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := requirePODAdmin(user, "podAdmin:model:check"); err != nil {
		return nil, err
	}
	if err := repository.PODCatalogCheck(ctx, id, catalogVault.Load()); err != nil {
		return nil, err
	}
	return map[string]any{"modelId": strconv.FormatInt(id, 10), "configurationReady": true, "providerCalled": false, "message": "配置和凭据来源检查通过；未调用供应商，不证明密钥有效或模型已开通"}, nil
}
