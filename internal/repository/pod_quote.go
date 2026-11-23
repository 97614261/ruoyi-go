package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/workflow"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/modelcatalog"
)

func lockManagedPlatform(tx *gorm.DB) (model.PODPlatformState, error) {
	var p model.PODPlatformState
	err := tx.Table("pod_platform_state").Where("state_id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&p).Error
	return p, err
}

// Called only after explicit managed configuration passed startup validation.
func EnablePODManaged(ctx context.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		p, err := lockManagedPlatform(tx)
		if err != nil {
			return err
		}
		if p.ManagedEnabled {
			return nil
		}
		return tx.Exec("UPDATE pod_platform_state SET managed_enabled=1,revision=revision+1,legacy_run_max_id=(SELECT COALESCE(MAX(run_id),0) FROM pod_run),legacy_batch_max_id=(SELECT COALESCE(MAX(batch_id),0) FROM pod_batch),cutover_time=CURRENT_TIMESTAMP(3) WHERE state_id=1").Error
	})
}
func managedHash(v any) string { return fmt.Sprintf("%x", sha256.Sum256(podJSON(v))) }
func ManagedQuoteSourceHash(source model.PODQuoteRequest) string {
	return managedHash(ManagedSource(source))
}
func ManagedSource(r model.PODQuoteRequest) model.PODQuoteRequest {
	if r.Run != nil {
		v := *r.Run
		v.RequestID = ""
		v.QuoteID = ""
		v.BatchID = 0
		v.BatchItemNo = 0
		r.Run = &v
	}
	if r.Batch != nil {
		v := *r.Batch
		v.RequestID = ""
		v.QuoteID = ""
		r.Batch = &v
	}
	return r
}
func quoteView(q model.PODExecutionQuote) (*model.PODQuoteView, error) {
	var plan model.PODQuotePlan
	if json.Unmarshal(q.SelectionJSON, &plan) != nil {
		return nil, errors.New("invalid quote snapshot")
	}
	v := &model.PODQuoteView{QuoteID: q.QuoteID, Kind: q.Kind, State: q.State, ExpiresAt: q.ExpiresAt, Points: q.PointsUpperBound, Lines: []model.PODQuoteLine{}, ResourceType: q.ConsumedResourceType, ResourceID: q.ConsumedResourceID}
	for _, b := range plan.Models {
		v.Lines = append(v.Lines, model.PODQuoteLine{ItemNo: b.ItemNo, NodeID: b.NodeID, ModelID: b.ModelID, ModelVersionID: b.ModelVersionID, Name: b.Name, Points: b.Points, Kind: b.Kind, OfficialModelID: b.Version.OfficialModelID, Images: b.ImageCount})
	}
	return v, nil
}
func GetPODQuote(ctx context.Context, uid int64, id string) (*model.PODQuoteView, error) {
	var q model.PODExecutionQuote
	if err := DB(ctx).Where("quote_id=? AND user_id=?", id, uid).Take(&q).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.WithCode(404, "报价不存在或无权访问")
		}
		return nil, err
	}
	return quoteView(q)
}

type catalogSet struct {
	models      map[int64]model.PODCatalogModel
	providers   map[int64]model.PODProvider
	versions    map[int64]model.PODCatalogModelVersion
	grants      map[int64]bool
	credentials map[int64]bool
}

func loadManagedCatalog(tx *gorm.DB, uid int64, ids, versionIDs []int64) (catalogSet, error) {
	c := catalogSet{map[int64]model.PODCatalogModel{}, map[int64]model.PODProvider{}, map[int64]model.PODCatalogModelVersion{}, map[int64]bool{}, map[int64]bool{}}
	if len(ids) == 0 {
		return c, nil
	}
	var ms []model.PODCatalogModel
	if err := tx.Where("model_id IN ?", ids).Find(&ms).Error; err != nil {
		return c, err
	}
	ps := []int64{}
	vs := append([]int64(nil), versionIDs...)
	for _, m := range ms {
		c.models[m.ModelID] = m
		ps = append(ps, m.ProviderID)
		if m.CurrentVersionID != nil {
			vs = append(vs, *m.CurrentVersionID)
		}
	}
	var providers []model.PODProvider
	if len(ps) > 0 {
		if err := tx.Where("provider_id IN ?", ps).Find(&providers).Error; err != nil {
			return c, err
		}
	}
	cs := []int64{}
	for _, p := range providers {
		c.providers[p.ProviderID] = p
		if p.CurrentCredentialVersionID != nil {
			cs = append(cs, *p.CurrentCredentialVersionID)
		}
	}
	var versions []model.PODCatalogModelVersion
	if len(vs) > 0 {
		if err := tx.Where("model_version_id IN ?", vs).Find(&versions).Error; err != nil {
			return c, err
		}
	}
	for _, v := range versions {
		c.versions[v.ModelVersionID] = v
	}
	var grants []int64
	if err := tx.Table("pod_user_model_grant").Where("user_id=? AND model_id IN ? AND enabled=1", uid, ids).Pluck("model_id", &grants).Error; err != nil {
		return c, err
	}
	for _, id := range grants {
		c.grants[id] = true
	}
	var creds []model.PODProviderCredential
	if len(cs) > 0 {
		if err := tx.Where("credential_version_id IN ? AND state='active'", cs).Find(&creds).Error; err != nil {
			return c, err
		}
	}
	for _, cr := range creds {
		if p, ok := c.providers[cr.ProviderID]; ok && p.CurrentCredentialVersionID != nil && *p.CurrentCredentialVersionID == cr.CredentialVersionID {
			c.credentials[cr.ProviderID] = true
		}
	}
	return c, nil
}
func (c catalogSet) usable(id, version int64, current bool) (model.PODCatalogModel, model.PODCatalogModelVersion, error) {
	m, ok := c.models[id]
	p := c.providers[m.ProviderID]
	v := c.versions[version]
	if !ok || !c.grants[id] || !m.Visible || m.Status != "enabled" || p.Status != "enabled" || m.CurrentVersionID == nil || !c.credentials[m.ProviderID] {
		return m, v, errs.WithCode(409, "所选模型未授权、已停用或配置不完整，请联系管理员")
	}
	if current && *m.CurrentVersionID != version {
		return m, v, errs.WithCode(409, "模型版本已变化，请重新报价")
	}
	if err := modelcatalog.ValidateHistoricalVersion(p, m, v); err != nil {
		return m, v, errs.WithCode(409, "模型版本校验失败，暂不能执行")
	}
	return m, v, nil
}
func creatorModel(m model.PODCatalogModel, v model.PODCatalogModelVersion, p model.PODPlatformState) (model.PODCreatorModel, error) {
	b := model.PODCreatorModel{ModelID: m.ModelID, ModelVersionID: v.ModelVersionID, Name: m.Name, Kind: m.Kind, OfficialModelID: v.OfficialModelID}
	if json.Unmarshal(v.CapabilitiesJSON, &b.Capabilities) != nil || json.Unmarshal(v.PointRuleJSON, &b.PointRule) != nil {
		return b, errors.New("invalid creator model")
	}
	b.Default = (p.DefaultTextModelID != nil && *p.DefaultTextModelID == m.ModelID) || (p.DefaultImageModelID != nil && *p.DefaultImageModelID == m.ModelID)
	return b, nil
}
func PODManagedModels(ctx context.Context, uid int64, kind string) ([]model.PODCreatorModel, error) {
	out := []model.PODCreatorModel{}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := checkCreator(tx, uid, false, false); err != nil {
			return err
		}
		var p model.PODPlatformState
		if err := tx.Table("pod_platform_state").Where("state_id=1").Take(&p).Error; err != nil {
			return err
		}
		ids := []int64{}
		q := tx.Table("pod_model m").Joins("JOIN pod_user_model_grant g ON g.model_id=m.model_id AND g.user_id=? AND g.enabled=1", uid).Where("m.visible=1 AND m.status='enabled'")
		if kind != "" {
			q = q.Where("m.kind=?", kind)
		}
		if err := q.Order("m.sort,m.model_id").Limit(100).Pluck("m.model_id", &ids).Error; err != nil {
			return err
		}
		c, err := loadManagedCatalog(tx, uid, ids, nil)
		if err != nil {
			return err
		}
		for _, id := range ids {
			m := c.models[id]
			if m.CurrentVersionID == nil {
				continue
			}
			m, v, err := c.usable(id, *m.CurrentVersionID, true)
			if err != nil {
				continue
			}
			view, err := creatorModel(m, v, p)
			if err != nil {
				return err
			}
			out = append(out, view)
		}
		return nil
	})
	return out, err
}

func bindManagedPlan(tx *gorm.DB, uid int64, p model.PODPlatformState, g model.PODGraph, nodes []model.PODNode, item int) ([]model.PODBoundModel, error) {
	selected := map[string]int64{}
	ids := []int64{}
	for _, n := range nodes {
		if n.Type != "textGenerate" && n.Type != "imageGenerate" {
			continue
		}
		var conf struct {
			ModelID string `json:"modelId"`
		}
		if json.Unmarshal(n.Config, &conf) != nil {
			return nil, ErrPODRunInput
		}
		id := int64(0)
		if conf.ModelID != "" {
			id, _ = strconv.ParseInt(conf.ModelID, 10, 64)
		} else if n.Type == "textGenerate" && p.DefaultTextModelID != nil {
			id = *p.DefaultTextModelID
		} else if n.Type == "imageGenerate" && p.DefaultImageModelID != nil {
			id = *p.DefaultImageModelID
		}
		if id < 1 {
			return nil, errs.WithCode(409, "请为 AI 节点选择模型，或请管理员设置已授权的默认模型")
		}
		selected[n.ID] = id
		ids = append(ids, id)
	}
	c, err := loadManagedCatalog(tx, uid, ids, nil)
	if err != nil {
		return nil, err
	}
	out := []model.PODBoundModel{}
	for _, n := range nodes {
		id, ok := selected[n.ID]
		if !ok {
			continue
		}
		m := c.models[id]
		if m.CurrentVersionID == nil {
			return nil, errs.WithCode(409, "模型尚无有效版本")
		}
		m, v, err := c.usable(id, *m.CurrentVersionID, true)
		if err != nil {
			return nil, err
		}
		b, err := modelcatalog.Bind(m, v, n, item)
		if err != nil {
			return nil, errs.WithCode(400, err.Error())
		}
		out = append(out, b)
	}
	return out, nil
}
func validateManagedBindings(tx *gorm.DB, uid int64, bs []model.PODBoundModel, current bool) error {
	ids, vs := []int64{}, []int64{}
	for _, b := range bs {
		ids = append(ids, b.ModelID)
		vs = append(vs, b.ModelVersionID)
	}
	c, err := loadManagedCatalog(tx, uid, ids, vs)
	if err != nil {
		return err
	}
	for _, b := range bs {
		_, v, err := c.usable(b.ModelID, b.ModelVersionID, current)
		if err != nil {
			return err
		}
		if v.SpecHash != b.Version.SpecHash || c.models[b.ModelID].ProviderID != b.ProviderID {
			return errs.WithCode(409, "模型规格已变化，请重新报价")
		}
		if err := verifyManagedBound(b, v, c.models[b.ModelID], c.providers[b.ProviderID]); err != nil {
			return errs.WithCode(409, "模型能力或计价快照校验失败")
		}
	}
	return nil
}

func CreatePODQuote(ctx context.Context, user *model.SysUser, source model.PODQuoteRequest, batchItems []model.PODBatchItem, l model.PODExecutionLimits, gl model.PODGraphLimits) (*model.PODQuoteView, error) {
	var quote model.PODExecutionQuote
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		platform, err := lockManagedPlatform(tx)
		if err != nil {
			return err
		}
		if !platform.ManagedEnabled {
			return errs.WithCode(409, "正式执行尚未启用")
		}
		w, err := lockedPODWorkspace(tx, user.UserID)
		if err != nil {
			return err
		}
		policy, err := checkCreator(tx, user.UserID, true, true)
		if err != nil {
			return err
		}
		plan := model.PODQuotePlan{Source: ManagedSource(source), Models: []model.PODBoundModel{}}
		switch source.Kind {
		case "run":
			b := *source.Run
			pid, _ := strconv.ParseInt(b.ProjectID, 10, 64)
			rev, _ := strconv.ParseInt(b.Revision, 10, 64)
			base, _ := strconv.ParseInt(b.BaseRunID, 10, 64)
			prep, err := preparePODRun(tx, w, b, pid, rev, base, l, gl)
			if err != nil {
				return err
			}
			plan.Models, err = bindManagedPlan(tx, user.UserID, platform, prep.Graph, prep.Plan, 0)
			if err != nil {
				return err
			}
			if err = validateManagedBase(tx, base, user.UserID, platform, prep); err != nil {
				return err
			}
		case "batch":
			if len(batchItems) < 1 || len(batchItems) > model.PODBatchMaxItems {
				return errs.WithCode(400, "批次份数无效")
			}
			for _, item := range batchItems {
				var g model.PODGraph
				if json.Unmarshal(item.GraphJSON, &g) != nil {
					return ErrPODRunInput
				}
				nodes, err := workflow.ExecutionPlanMode(g, gl, "", nil, "managed")
				if err != nil {
					return err
				}
				bs, err := bindManagedPlan(tx, user.UserID, platform, g, nodes, item.ItemNo)
				if err != nil {
					return err
				}
				plan.Models = append(plan.Models, bs...)
			}
		case "retry":
			id, _ := strconv.ParseInt(source.RunID, 10, 64)
			var r model.PODRun
			if err := tx.Where("run_id=? AND user_id=? AND workspace_id=? AND execution_mode='managed'", id, user.UserID, w.WorkspaceID).Take(&r).Error; err != nil {
				return ErrPODRun
			}
			if r.State != model.PODRunFailed || !r.Settled || r.Attempt >= 5 {
				return ErrPODRunState
			}
			plan.RunAttempt = r.Attempt
			var nodes []model.PODNodeRun
			if err := tx.Where("run_id=? AND state<>'succeeded'", id).Find(&nodes).Error; err != nil {
				return err
			}
			wanted := map[string]bool{}
			for _, n := range nodes {
				wanted[n.NodeID] = true
			}
			var bindings []model.PODRunModelBinding
			if err := tx.Where("run_id=?", id).Order("node_id").Find(&bindings).Error; err != nil {
				return err
			}
			for _, row := range bindings {
				if !wanted[row.NodeID] {
					continue
				}
				var b model.PODBoundModel
				if json.Unmarshal(row.SelectionJSON, &b) != nil {
					return ErrPODRunInput
				}
				plan.Models = append(plan.Models, b)
			}
			if err := validateManagedBindings(tx, user.UserID, plan.Models, false); err != nil {
				return err
			}
		default:
			return errs.WithCode(400, "报价类型无效")
		}
		var now time.Time
		if err := tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
			return err
		}
		// Only expired, unused quotes are disposable. Never purge a consumed quote or its accounting evidence.
		if err := tx.Exec("DELETE FROM pod_execution_quote WHERE user_id=? AND state='ready' AND expires_at<=? ORDER BY expires_at,quote_id LIMIT 100", user.UserID, now).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.PODExecutionQuote{}).Where("user_id=? AND state='ready' AND expires_at>?", user.UserID, now).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return errs.WithCode(429, "待使用报价最多 20 个，请使用已有报价或稍后再试")
		}
		quote = model.PODExecutionQuote{QuoteID: uuid.NewString(), UserID: user.UserID, WorkspaceID: w.WorkspaceID, Kind: source.Kind, SourceHash: managedHash(plan.Source), SelectionJSON: podJSON(plan), PolicyRevision: policy.Revision, CatalogHash: managedHash(plan.Models), State: "ready", ExpiresAt: now.Add(10 * time.Minute), CreateTime: now}
		for _, b := range plan.Models {
			quote.PointsUpperBound += b.Points
			quote.CostUpperMicros += b.CostMicros
		}
		return tx.Create(&quote).Error
	})
	if err != nil {
		return nil, err
	}
	return quoteView(quote)
}

func validateManagedBase(tx *gorm.DB, base, uid int64, p model.PODPlatformState, prep *preparedPODRun) error {
	if base == 0 {
		return nil
	}
	needed := []model.PODNode{}
	for _, n := range prep.Graph.Nodes {
		if _, ok := prep.Base[n.ID]; ok && (n.Type == "textGenerate" || n.Type == "imageGenerate") {
			needed = append(needed, n)
		}
	}
	bs, err := bindManagedPlan(tx, uid, p, prep.Graph, needed, 0)
	if err != nil {
		return err
	}
	rows := []model.PODRunModelBinding{}
	if len(bs) > 0 {
		if err := tx.Where("run_id=?", base).Find(&rows).Error; err != nil {
			return err
		}
	}
	versions := map[string]int64{}
	for _, r := range rows {
		versions[r.NodeID] = r.ModelVersionID
	}
	for _, b := range bs {
		if versions[b.NodeID] != b.ModelVersionID {
			return errs.WithCode(409, "历史上游的模型版本不一致，请重新整条执行")
		}
	}
	return nil
}

func consumeManagedQuote(tx *gorm.DB, uid int64, id, kind, request string, resource int64, source model.PODQuoteRequest) (*model.PODExecutionQuote, model.PODQuotePlan, error) {
	var q model.PODExecutionQuote
	var p model.PODQuotePlan
	if err := tx.Where("quote_id=? AND user_id=?", id, uid).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&q).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, p, errs.WithCode(404, "报价不存在或无权使用")
		}
		return nil, p, err
	}
	if q.Kind != kind || q.SourceHash != managedHash(ManagedSource(source)) || json.Unmarshal(q.SelectionJSON, &p) != nil {
		return nil, p, errs.WithCode(409, "执行内容与报价不一致，请重新报价")
	}
	if q.State != "ready" {
		return nil, p, errs.WithCode(409, "报价已使用，请核对原操作")
	}
	var now time.Time
	if err := tx.Raw("SELECT CURRENT_TIMESTAMP(3)").Scan(&now).Error; err != nil {
		return nil, p, err
	}
	if !now.Before(q.ExpiresAt) {
		return nil, p, errs.WithCode(409, "报价已过期，请重新报价")
	}
	policy, err := checkCreator(tx, uid, true, true)
	if err != nil {
		return nil, p, err
	}
	if policy.Revision != q.PolicyRevision {
		return nil, p, errs.WithCode(409, "用户权益已变化，请重新报价")
	}
	if err := validateManagedBindings(tx, uid, p.Models, kind != "retry"); err != nil {
		return nil, p, err
	}
	q.State = "consumed"
	q.ConsumedResourceType = kind
	if kind == "retry" {
		q.ConsumedResourceType = "run"
	}
	q.ConsumedResourceID = resource
	q.ConsumedRequestID = &request
	if err := tx.Save(&q).Error; err != nil {
		return nil, p, err
	}
	return &q, p, nil
}
