package repository

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func catalogProviders(tx *gorm.DB, f model.PODCatalogFilter) *gorm.DB {
	q := tx.Table("pod_provider p").Joins("LEFT JOIN pod_provider_credential_version c ON c.credential_version_id=p.current_credential_version_id AND c.provider_id=p.provider_id")
	if f.Code != "" {
		q = q.Where("p.provider_code=?", f.Code)
	}
	if f.Name != "" {
		q = q.Where("p.name LIKE ?", "%"+f.Name+"%")
	}
	if f.Status != "" {
		q = q.Where("p.status=?", f.Status)
	}
	return q
}

const providerFields = "p.provider_id,p.provider_code code,p.name,p.adapter_kind,p.region,p.endpoint,p.status,p.revision,(p.current_credential_version_id IS NOT NULL) credential_configured,COALESCE(c.version,0) credential_version,COALESCE(c.source_type,'') credential_source_type"

func PODCatalogProviders(ctx context.Context, f model.PODCatalogFilter, pg page.Query) ([]model.PODProviderView, int64, error) {
	var total int64
	if err := catalogProviders(DB(ctx), f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODProviderView{}
	err := catalogProviders(DB(ctx), f).Select(providerFields).Order("p.provider_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODCatalogProvider(ctx context.Context, id int64) (*model.PODProviderView, error) {
	var row model.PODProviderView
	err := catalogProviders(DB(ctx), model.PODCatalogFilter{}).Select(providerFields).Where("p.provider_id=?", id).Take(&row).Error
	return &row, catalogMissing(err)
}
func catalogModels(tx *gorm.DB, f model.PODCatalogFilter) *gorm.DB {
	q := tx.Table("pod_model m").Joins("JOIN pod_provider p ON p.provider_id=m.provider_id").Joins("LEFT JOIN pod_model_version v ON v.model_version_id=m.current_version_id AND v.model_id=m.model_id")
	if f.Code != "" {
		q = q.Where("m.model_code=?", f.Code)
	}
	if f.Name != "" {
		q = q.Where("m.name LIKE ?", "%"+f.Name+"%")
	}
	if f.Kind != "" {
		q = q.Where("m.kind=?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("m.status=?", f.Status)
	}
	if f.ProviderID > 0 {
		q = q.Where("m.provider_id=?", f.ProviderID)
	}
	return q
}

const modelFields = "m.model_id,m.model_code code,m.provider_id,p.name provider_name,p.status provider_status,(p.current_credential_version_id IS NOT NULL) provider_credential_configured,p.adapter_kind provider_adapter_kind,p.endpoint provider_endpoint,m.kind,m.name,COALESCE(m.current_version_id,0) current_version_id,COALESCE(v.version,0) current_version,m.status,m.visible,m.sort,m.revision"

func PODCatalogModels(ctx context.Context, f model.PODCatalogFilter, pg page.Query) ([]model.PODCatalogModelView, int64, error) {
	var total int64
	if err := catalogModels(DB(ctx), f).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODCatalogModelView{}
	err := catalogModels(DB(ctx), f).Select(modelFields).Order("m.sort,m.model_id").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	if err == nil {
		err = catalogVerification(ctx, rows)
	}
	return rows, total, err
}
func PODCatalogModelGet(ctx context.Context, id int64) (*model.PODCatalogModelView, error) {
	var row model.PODCatalogModelView
	err := catalogModels(DB(ctx), model.PODCatalogFilter{}).Select(modelFields).Where("m.model_id=?", id).Take(&row).Error
	if err == nil {
		rows := []model.PODCatalogModelView{row}
		err = catalogVerification(ctx, rows)
		row = rows[0]
	}
	return &row, catalogMissing(err)
}

// A successful receipt proves this exact model AND current credential version.
func catalogVerification(ctx context.Context, rows []model.PODCatalogModelView) error {
	ids := []int64{}
	for _, r := range rows {
		if r.CurrentVersionID > 0 {
			ids = append(ids, r.CurrentVersionID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var verified []struct {
		ModelVersionID int64
		VerifiedAt     types.Time
	}
	err := DB(ctx).Table("pod_call_billing b").
		Joins("JOIN pod_provider_call c ON c.submission_key=b.submission_key AND c.state='confirmed'").
		Joins("JOIN pod_model m ON m.current_version_id=b.model_version_id").
		Joins("JOIN pod_provider p ON p.provider_id=m.provider_id AND p.current_credential_version_id=b.credential_version_id").
		Where("b.model_version_id IN ? AND JSON_UNQUOTE(JSON_EXTRACT(c.receipt_json,'$.state'))='succeeded'", ids).
		Select("b.model_version_id,MAX(c.update_time) verified_at").Group("b.model_version_id").Scan(&verified).Error
	if err != nil {
		return err
	}
	for i := range rows {
		for _, v := range verified {
			if v.ModelVersionID == rows[i].CurrentVersionID {
				stamp := v.VerifiedAt
				rows[i].VerifiedAt = &stamp
				break
			}
		}
	}
	return nil
}
func PODCatalogVersions(ctx context.Context, id int64, pg page.Query) ([]model.PODCatalogModelVersion, int64, error) {
	var total int64
	if _, err := PODCatalogModelGet(ctx, id); err != nil {
		return nil, 0, err
	}
	if err := DB(ctx).Model(&model.PODCatalogModelVersion{}).Where("model_id=?", id).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODCatalogModelVersion{}
	err := DB(ctx).Where("model_id=?", id).Order("version DESC,model_version_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func catalogMissing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.WithCode(404, "供应商或模型记录不存在")
	}
	return err
}
func PODCatalogDefaults(ctx context.Context) (*model.PODCatalogDefaults, error) {
	s, err := PODAdminState(ctx)
	if err != nil {
		return nil, err
	}
	v := &model.PODCatalogDefaults{Revision: s.Revision, ManagedEnabled: s.ManagedEnabled}
	if s.DefaultTextModelID != nil {
		v.TextModelID = strconv.FormatInt(*s.DefaultTextModelID, 10)
	}
	if s.DefaultImageModelID != nil {
		v.ImageModelID = strconv.FormatInt(*s.DefaultImageModelID, 10)
	}
	return v, nil
}
func PODCatalogAction(ctx context.Context, actor int64, request string) (*model.PODAdminAction, error) {
	var row model.PODAdminAction
	err := DB(ctx).Where("actor_user_id=? AND request_id=?", actor, request).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.WithCode(404, "尚无已完成记录；请保留原请求编号核对")
	}
	return &row, err
}

type PODCatalogMutation struct {
	ActorID    int64
	ID         int64
	Revision   int64
	Op         string
	Permission string
	Hash       string
	Trace      string
	Request    model.PODCatalogWrite
}

func PODCatalogRejected(ctx context.Context, actor int64, op, trace string) error {
	kind := "catalog"
	if strings.HasPrefix(op, "creator.") || op == "points.adjust" {
		kind = "creator"
	}
	return DB(ctx).Create(&model.PODAdminAudit{ActorUserID: actor, ActionType: op, ResourceType: kind, Reason: "请求校验或管理配置未通过", BeforeJSON: podJSON(model.PODAdminAuditSnapshot{}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{}), TraceID: trace, Outcome: "failure", CreateBy: strconv.FormatInt(actor, 10), CreateTime: types.Now()}).Error
}

func catalogActor(tx *gorm.DB, actor int64, permission string) error {
	var count int64
	err := tx.Table("sys_user").Where("user_id=? AND status='0' AND del_flag='0'", actor).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return errs.WithCode(403, "管理权限已失效")
	}
	if actor == model.AdminUserID {
		return nil
	}
	var permissions []string
	if err := tx.Table("sys_user_role ur").Joins("JOIN sys_role r ON r.role_id=ur.role_id AND r.status='0' AND r.del_flag='0'").Joins("JOIN sys_role_menu rm ON rm.role_id=r.role_id").Joins("JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0'").Where("ur.user_id=?", actor).Pluck("m.perms", &permissions).Error; err != nil {
		return err
	}
	for _, raw := range permissions {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p == permission || p == model.AllPermission {
				return nil
			}
		}
	}
	return errs.WithCode(403, "管理权限已失效")
}
func catalogSnapshot(tx *gorm.DB, resource string, id int64) (model.PODCatalogAuditSnapshot, error) {
	s := model.PODCatalogAuditSnapshot{}
	if id == 0 {
		return s, nil
	}
	if resource == "provider" {
		var p model.PODProvider
		if err := tx.Where("provider_id=?", id).Take(&p).Error; err != nil {
			return s, catalogMissing(err)
		} else {
			s.Code = p.ProviderCode
			s.Endpoint = p.Endpoint
			s.Revision = p.Revision
			s.Status = p.Status
			if p.CurrentCredentialVersionID != nil {
				var c model.PODProviderCredential
				if err := tx.Select("version,source_type").Where("credential_version_id=? AND provider_id=?", *p.CurrentCredentialVersionID, p.ProviderID).Take(&c).Error; err != nil {
					return s, err
				}
				s.Version = c.Version
				s.SourceType = c.SourceType
			}
		}
	}
	if resource == "model" {
		var m model.PODCatalogModel
		if err := tx.Where("model_id=?", id).Take(&m).Error; err != nil {
			return s, catalogMissing(err)
		} else {
			s.Code = m.ModelCode
			s.Revision = m.Revision
			s.Status = m.Status
			if m.CurrentVersionID != nil {
				var v model.PODCatalogModelVersion
				if err := tx.Select("version").Where("model_version_id=? AND model_id=?", *m.CurrentVersionID, m.ModelID).Take(&v).Error; err != nil {
					return s, err
				}
				s.Version = v.Version
			}
		}
	}
	if resource == "platform" {
		var state model.PODPlatformState
		if err := tx.Table("pod_platform_state").Where("state_id=1").Take(&state).Error; err != nil {
			return s, err
		}
		s.Revision = state.Revision
		if state.DefaultTextModelID != nil {
			s.TextModelID = strconv.FormatInt(*state.DefaultTextModelID, 10)
		}
		if state.DefaultImageModelID != nil {
			s.ImageModelID = strconv.FormatInt(*state.DefaultImageModelID, 10)
		}
	}
	return s, nil
}

func PODCatalogMutate(ctx context.Context, w PODCatalogMutation, vault *modelcatalog.Vault) (*model.PODCatalogResult, error) {
	result := &model.PODCatalogResult{RequestID: w.Request.RequestID, ActionType: w.Op, ResourceID: w.ID}
	resource := "model"
	if len(w.Op) >= 9 && w.Op[:9] == "provider." {
		resource = "provider"
	}
	if w.Op == "defaults.update" {
		resource = "platform"
		result.ResourceID = 1
	}
	result.ResourceType = resource
	err := DB(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		var state model.PODPlatformState
		if err := tx.Table("pod_platform_state").Clauses(clause.Locking{Strength: "UPDATE"}).Where("state_id=1").Take(&state).Error; err != nil {
			return err
		}
		if err := catalogActor(tx, w.ActorID, model.PODAdminAccess); err != nil {
			return err
		}
		if err := catalogActor(tx, w.ActorID, w.Permission); err != nil {
			return err
		}
		var action model.PODAdminAction
		err := tx.Where("actor_user_id=? AND request_id=?", w.ActorID, w.Request.RequestID).Take(&action).Error
		if err == nil {
			if action.ActionType != w.Op || !hmac.Equal([]byte(action.RequestHash), []byte(w.Hash)) {
				return errs.WithCode(409, "请求编号已用于不同操作或内容")
			}
			if json.Unmarshal(action.ResultJSON, result) != nil || action.State != "completed" {
				return errors.New("管理操作结果无法确认")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		before, err := catalogSnapshot(tx, resource, result.ResourceID)
		if err != nil {
			return err
		}
		if err := applyCatalogMutation(tx, w, vault, &state, result); err != nil {
			return err
		}
		after, err := catalogSnapshot(tx, resource, result.ResourceID)
		if err != nil {
			return err
		}
		if result.Version > 0 {
			after.Version = result.Version
		}
		if w.Op == "provider.credential" {
			after.SourceType = w.Request.SourceType
		}
		if resource == "platform" {
			after.Revision = result.Revision
			if state.DefaultTextModelID != nil {
				after.TextModelID = strconv.FormatInt(*state.DefaultTextModelID, 10)
			}
			if state.DefaultImageModelID != nil {
				after.ImageModelID = strconv.FormatInt(*state.DefaultImageModelID, 10)
			}
		}
		audit := model.PODAdminAudit{ActorUserID: w.ActorID, ActionType: w.Op, ResourceType: resource, ResourceID: result.ResourceID, Reason: w.Request.Reason, BeforeJSON: podJSON(model.PODAdminAuditSnapshot{Catalog: &before}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{Catalog: &after}), TraceID: w.Trace, Outcome: "success", CreateBy: strconv.FormatInt(w.ActorID, 10), CreateTime: types.Now()}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		action = model.PODAdminAction{ActorUserID: w.ActorID, RequestID: w.Request.RequestID, RequestHash: w.Hash, ActionType: w.Op, ResourceType: resource, ResourceID: result.ResourceID, State: "completed", ResultJSON: podJSON(result), CreateBy: strconv.FormatInt(w.ActorID, 10), CreateTime: types.Now()}
		return tx.Create(&action).Error
	})
	if err != nil {
		// Only typed metadata is recorded. No request serialization or secret digest.
		audit := model.PODAdminAudit{ActorUserID: w.ActorID, ActionType: w.Op, ResourceType: resource, ResourceID: w.ID, Reason: w.Request.Reason, BeforeJSON: podJSON(model.PODAdminAuditSnapshot{}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{}), TraceID: w.Trace, Outcome: "failure", CreateBy: strconv.FormatInt(w.ActorID, 10), CreateTime: types.Now()}
		if auditErr := DB(ctx).Create(&audit).Error; auditErr != nil {
			return nil, errors.New("模型管理失败且审计不可用，请核对原请求编号")
		}
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return nil, errs.WithCode(409, "编码已存在，请使用其他编码")
		}
		return nil, err
	}
	return result, nil
}

func applyCatalogMutation(tx *gorm.DB, w PODCatalogMutation, vault *modelcatalog.Vault, state *model.PODPlatformState, out *model.PODCatalogResult) error {
	r := w.Request
	actor := strconv.FormatInt(w.ActorID, 10)
	if w.Op == "provider.create" {
		p := model.PODProvider{ProviderCode: r.Code, Name: r.Name, AdapterKind: r.AdapterKind, Region: r.Region, Endpoint: r.Endpoint, Status: "disabled", Revision: 1, CreateBy: actor, CreateTime: types.Now()}
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		out.ResourceID = p.ProviderID
		out.Revision = 1
		return nil
	}
	if w.Op == "defaults.update" {
		if state.Revision != w.Revision || state.Revision == math.MaxInt64 {
			return errs.WithCode(409, "平台设置已变化，请重新读取")
		}
		ids := []int64{}
		for _, raw := range []*string{r.TextModelID, r.ImageModelID} {
			if raw != nil && *raw != "" {
				id, _ := strconv.ParseInt(*raw, 10, 64)
				ids = append(ids, id)
			}
		}
		rows := []struct {
			ModelID                        int64
			Kind, Status, ProviderStatus   string
			CurrentVersionID, CredentialID *int64
		}{}
		if len(ids) > 0 {
			if err := tx.Table("pod_model m").Select("m.model_id,m.kind,m.status,m.current_version_id,p.status provider_status,p.current_credential_version_id credential_id").Joins("JOIN pod_provider p ON p.provider_id=m.provider_id").Where("m.model_id IN ?", ids).Scan(&rows).Error; err != nil {
				return err
			}
		}
		valid := map[int64]string{}
		for _, m := range rows {
			if m.Status == "enabled" && m.ProviderStatus == "enabled" && m.CurrentVersionID != nil && m.CredentialID != nil {
				valid[m.ModelID] = m.Kind
			}
		}
		values := []*int64{nil, nil}
		for i, raw := range []*string{r.TextModelID, r.ImageModelID} {
			if raw == nil {
				return errs.WithCode(400, "默认值必须明确指定，空字符串表示清除")
			}
			if *raw != "" {
				id, _ := strconv.ParseInt(*raw, 10, 64)
				kind := "text"
				if i == 1 {
					kind = "image"
				}
				if valid[id] != kind {
					return errs.WithCode(409, "默认模型须为对应类型、模型和供应商已启用，且已配置凭据和版本")
				}
				values[i] = &id
			}
		}
		state.DefaultTextModelID = values[0]
		state.DefaultImageModelID = values[1]
		out.Revision = state.Revision + 1
		return tx.Table("pod_platform_state").Where("state_id=1").Updates(map[string]any{"default_text_model_id": values[0], "default_image_model_id": values[1], "revision": out.Revision, "update_by": actor, "update_time": podNow()}).Error
	}
	if out.ResourceType == "provider" {
		var p model.PODProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id=?", w.ID).Take(&p).Error; err != nil {
			return catalogMissing(err)
		}
		if p.Revision != w.Revision || p.Revision == math.MaxInt64 {
			return errs.WithCode(409, "供应商已变化，请重新读取")
		}
		updates := map[string]any{"revision": p.Revision + 1, "update_by": actor, "update_time": podNow()}
		switch w.Op {
		case "provider.update":
			updates["name"] = r.Name
			if r.Endpoint != "" && r.Endpoint != p.Endpoint {
				if err := catalogActor(tx, w.ActorID, "podAdmin:model:edit"); err != nil {
					return err
				}
				if err := modelcatalog.ValidateEndpointChange(p, r.Endpoint); err != nil {
					return errs.WithCode(400, err.Error())
				}
				if err := updateCatalogEndpointVersions(tx, p, r.Endpoint, actor); err != nil {
					return err
				}
				updates["endpoint"] = r.Endpoint
			}
		case "provider.status":
			if r.Status == "enabled" {
				if _, err := catalogCredential(tx, p, vault); err != nil {
					return err
				}
			}
			updates["status"] = r.Status
		case "provider.credential":
			var current model.PODProviderCredential
			if p.CurrentCredentialVersionID != nil {
				if err := tx.Where("credential_version_id=? AND provider_id=?", *p.CurrentCredentialVersionID, p.ProviderID).Take(&current).Error; err != nil {
					return err
				}
			}
			if current.Version == math.MaxInt64 {
				return errs.WithCode(409, "凭据版本已达上限")
			}
			c := model.PODProviderCredential{ProviderID: p.ProviderID, Version: current.Version + 1, SourceType: r.SourceType, EnvName: r.EnvName, State: "active", CreateBy: actor, CreateTime: types.Now()}
			if r.SourceType == "encrypted" {
				var err error
				c.KeyID, c.Nonce, c.Ciphertext, err = vault.Seal(p.ProviderID, c.Version, r.Secret)
				if err != nil {
					return errs.WithCode(503, "模型凭据加密不可用")
				}
			} else {
				if _, err := vault.ResolveEnv(r.EnvName); err != nil {
					return errs.WithCode(400, err.Error())
				}
			}
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
			updates["current_credential_version_id"] = c.CredentialVersionID
			out.Version = c.Version
		default:
			return errs.WithCode(400, "管理操作无效")
		}
		out.Revision = p.Revision + 1
		return tx.Model(&model.PODProvider{}).Where("provider_id=?", w.ID).Updates(updates).Error
	}
	if w.Op == "model.create" {
		providerID, _ := strconv.ParseInt(r.ProviderID, 10, 64)
		var p model.PODProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id=?", providerID).Take(&p).Error; err != nil {
			return catalogMissing(err)
		}
		m := model.PODCatalogModel{ModelCode: r.Code, Name: r.Name, ProviderID: providerID, Kind: r.Kind, Status: "disabled", Revision: 1, CreateBy: actor, CreateTime: types.Now()}
		if r.Visible != nil {
			m.Visible = *r.Visible
		}
		if r.Sort != nil {
			m.Sort = *r.Sort
		}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		out.ResourceID = m.ModelID
		out.Revision = 1
		return nil
	}
	// The platform lock serializes catalog writers. Provider precedes model in
	// the shared execution lock order, including when a new version is created.
	var m model.PODCatalogModel
	if err := tx.Where("model_id=?", w.ID).Take(&m).Error; err != nil {
		return catalogMissing(err)
	}
	var p model.PODProvider
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id=?", m.ProviderID).Take(&p).Error; err != nil {
		return catalogMissing(err)
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_id=?", w.ID).Take(&m).Error; err != nil {
		return catalogMissing(err)
	}
	if m.Revision != w.Revision || m.Revision == math.MaxInt64 {
		return errs.WithCode(409, "模型已变化，请重新读取")
	}
	updates := map[string]any{"revision": m.Revision + 1, "update_by": actor, "update_time": podNow()}
	switch w.Op {
	case "model.update":
		updates["name"] = r.Name
		if r.Visible != nil {
			updates["visible"] = *r.Visible
		}
		if r.Sort != nil {
			updates["sort"] = *r.Sort
		}
	case "model.status":
		if r.Status == "enabled" {
			if p.Status != "enabled" || m.CurrentVersionID == nil {
				return errs.WithCode(409, "先启用供应商并保存模型版本")
			}
			if _, err := catalogCredential(tx, p, vault); err != nil {
				return err
			}
		}
		updates["status"] = r.Status
	case "model.version":
		if err := modelcatalog.ValidateSpec(m.Kind, r); err != nil {
			return errs.WithCode(400, err.Error())
		}
		var previous model.PODCatalogModelVersion
		if m.CurrentVersionID != nil {
			if err := tx.Where("model_version_id=? AND model_id=?", *m.CurrentVersionID, m.ModelID).Take(&previous).Error; err != nil {
				return err
			}
		}
		if previous.Version == math.MaxInt64 {
			return errs.WithCode(409, "模型版本已达上限")
		}
		v := model.PODCatalogModelVersion{ModelID: m.ModelID, Version: previous.Version + 1, OfficialModelID: r.OfficialModelID, AdapterKind: p.AdapterKind, AdapterVersion: "1", Endpoint: p.Endpoint, Region: p.Region, CapabilitiesJSON: podJSON(r.Capabilities), CostRuleJSON: podJSON(r.CostRule), PointRuleJSON: podJSON(r.PointRule), TimeoutSeconds: r.TimeoutSeconds, CreateBy: actor, CreateTime: types.Now()}
		v.SpecHash = modelcatalog.SpecHash(p.ProviderID, v, r)
		if err := modelcatalog.ValidateVersion(p, m, v); err != nil {
			return errs.WithCode(400, err.Error())
		}
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		updates["current_version_id"] = v.ModelVersionID
		out.Version = v.Version
	default:
		return errs.WithCode(400, "管理操作无效")
	}
	out.Revision = m.Revision + 1
	return tx.Model(&model.PODCatalogModel{}).Where("model_id=?", w.ID).Updates(updates).Error
}

func updateCatalogEndpointVersions(tx *gorm.DB, p model.PODProvider, endpoint, actor string) error {
	var models []model.PODCatalogModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider_id=? AND current_version_id IS NOT NULL", p.ProviderID).Order("model_id").Find(&models).Error; err != nil {
		return err
	}
	for _, m := range models {
		var current model.PODCatalogModelVersion
		if err := tx.Where("model_version_id=? AND model_id=?", *m.CurrentVersionID, m.ModelID).Take(&current).Error; err != nil {
			return err
		}
		if m.Revision == math.MaxInt64 || current.Version == math.MaxInt64 {
			return errs.WithCode(409, "模型版本或修订号已达上限，地址未修改")
		}
		next, err := modelcatalog.VersionWithEndpoint(p, m, current, endpoint)
		if err != nil {
			return errs.WithCode(409, "供应商下有模型版本配置无效，地址未修改")
		}
		next.ModelVersionID, next.Version = 0, current.Version+1
		next.CreateBy, next.CreateTime = actor, types.Now()
		next.UpdateBy, next.UpdateTime, next.Remark = "", types.Time{}, ""
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		if err := tx.Model(&m).Updates(map[string]any{"current_version_id": next.ModelVersionID, "revision": m.Revision + 1, "update_by": actor, "update_time": podNow()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func catalogCredential(tx *gorm.DB, p model.PODProvider, vault *modelcatalog.Vault) (string, error) {
	if p.CurrentCredentialVersionID == nil {
		return "", errs.WithCode(409, "供应商尚未配置凭据")
	}
	var c model.PODProviderCredential
	if err := tx.Where("credential_version_id=? AND provider_id=?", *p.CurrentCredentialVersionID, p.ProviderID).Take(&c).Error; err != nil {
		return "", err
	}
	if c.State != "active" {
		return "", errs.WithCode(409, "凭据版本不可用")
	}
	var secret string
	var err error
	if c.SourceType == "env" {
		secret, err = vault.ResolveEnv(c.EnvName)
	} else if c.SourceType == "encrypted" {
		secret, err = vault.Open(c.ProviderID, c.Version, c.KeyID, c.Nonce, c.Ciphertext)
	} else {
		err = errors.New("invalid source")
	}
	if err != nil || !modelcatalog.ValidSecret(secret) {
		return "", errs.WithCode(409, "凭据来源不可用，请核对部署密钥或轮换凭据")
	}
	return secret, nil
}

func PODCatalogCheck(ctx context.Context, id int64, vault *modelcatalog.Vault) error {
	return DB(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		var m model.PODCatalogModel
		if err := tx.Where("model_id=?", id).Take(&m).Error; err != nil {
			return catalogMissing(err)
		}
		var p model.PODProvider
		if err := tx.Where("provider_id=?", m.ProviderID).Take(&p).Error; err != nil {
			return catalogMissing(err)
		}
		if err := modelcatalog.ValidateProvider(p.AdapterKind, p.Region, p.Endpoint); err != nil {
			return errs.WithCode(400, err.Error())
		}
		if m.CurrentVersionID == nil {
			return errs.WithCode(409, "模型尚无有效版本")
		}
		var v model.PODCatalogModelVersion
		if err := tx.Where("model_version_id=? AND model_id=?", *m.CurrentVersionID, m.ModelID).Take(&v).Error; err != nil {
			return err
		}
		if err := modelcatalog.ValidateVersion(p, m, v); err != nil {
			return errs.WithCode(400, err.Error())
		}
		_, err := catalogCredential(tx, p, vault)
		return err
	})
}

// Offline restore verification includes historical versions, not only active pointers.
func PODCatalogVerifyRestore(ctx context.Context, vault *modelcatalog.Vault) (int, int, error) {
	tx := DB(ctx).Session(&gorm.Session{Logger: logger.Discard})
	var providers []model.PODProvider
	var models []model.PODCatalogModel
	var versions []model.PODCatalogModelVersion
	var credentials []model.PODProviderCredential
	for _, value := range []any{&providers, &models, &versions, &credentials} {
		if err := tx.Find(value).Error; err != nil {
			return 0, 0, errors.New("恢复模型目录不可读")
		}
	}
	ps := map[int64]model.PODProvider{}
	ms := map[int64]model.PODCatalogModel{}
	vs := map[int64]int64{}
	cs := map[int64]int64{}
	for _, p := range providers {
		ps[p.ProviderID] = p
	}
	for _, m := range models {
		if _, ok := ps[m.ProviderID]; !ok {
			return 0, 0, errors.New("恢复模型缺少供应商")
		}
		ms[m.ModelID] = m
	}
	for _, v := range versions {
		m, ok := ms[v.ModelID]
		if !ok || modelcatalog.ValidateHistoricalVersion(ps[m.ProviderID], m, v) != nil {
			return 0, 0, errors.New("恢复历史模型版本不完整")
		}
		vs[v.ModelVersionID] = v.ModelID
	}
	decrypted := 0
	for _, c := range credentials {
		if _, ok := ps[c.ProviderID]; !ok {
			return 0, 0, errors.New("恢复凭据缺少供应商")
		}
		if c.SourceType == "encrypted" {
			secret, err := vault.Open(c.ProviderID, c.Version, c.KeyID, c.Nonce, c.Ciphertext)
			if err != nil || !modelcatalog.ValidSecret(secret) {
				return 0, 0, errors.New("恢复凭据解密失败，请另行提供配套主密钥")
			}
			decrypted++
		} else if c.SourceType != "env" || !vault.EnvAllowed(c.EnvName) {
			return 0, 0, errors.New("恢复环境凭据不在部署白名单")
		}
		cs[c.CredentialVersionID] = c.ProviderID
	}
	for _, p := range providers {
		if p.CurrentCredentialVersionID != nil && cs[*p.CurrentCredentialVersionID] != p.ProviderID {
			return 0, 0, errors.New("恢复供应商凭据指针不一致")
		}
	}
	for _, m := range models {
		if m.CurrentVersionID != nil && vs[*m.CurrentVersionID] != m.ModelID {
			return 0, 0, errors.New("恢复当前模型版本指针不一致")
		}
	}
	state, err := PODAdminState(ctx)
	if err != nil {
		return 0, 0, err
	}
	for i, ptr := range []*int64{state.DefaultTextModelID, state.DefaultImageModelID} {
		if ptr != nil {
			kind := "text"
			if i == 1 {
				kind = "image"
			}
			m, ok := ms[*ptr]
			if !ok || m.Kind != kind {
				return 0, 0, errors.New("恢复默认模型指针不一致")
			}
		}
	}
	return len(versions), decrypted, nil
}
