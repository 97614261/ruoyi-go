package repository

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/modelcatalog"
)

// Restore verification must account for uncertain calls as held, never as refundable expiry.
func PODVerifyManagedAccounts(ctx context.Context) error {
	checks := []string{
		`SELECT COUNT(*) FROM pod_points_account a WHERE a.held<>
		 (SELECT COALESCE(SUM(s.unallocated_points),0) FROM pod_execution_snapshot s WHERE s.user_id=a.user_id)+
		 (SELECT COALESCE(SUM(b.held_points),0) FROM pod_call_billing b WHERE b.user_id=a.user_id AND b.state='held')
		 OR a.spent<>(SELECT COALESCE(SUM(b.charged_points),0) FROM pod_call_billing b WHERE b.user_id=a.user_id)`,
		`SELECT COUNT(*) FROM pod_execution_snapshot s LEFT JOIN pod_creator_policy p ON p.user_id=s.user_id AND p.workspace_id=s.workspace_id
		 WHERE p.user_id IS NULL OR s.billing_scheme<>'managedPointsV1' OR s.hold_points<0 OR s.unallocated_points<0 OR s.unallocated_points>s.hold_points
		 OR s.scope_kind NOT IN ('run','batch')
		 OR (s.scope_kind='run' AND NOT EXISTS(SELECT 1 FROM pod_run r WHERE r.run_id=s.scope_id AND r.user_id=s.user_id AND r.workspace_id=s.workspace_id AND r.execution_mode='managed'))
		 OR (s.scope_kind='batch' AND NOT EXISTS(SELECT 1 FROM pod_batch b WHERE b.batch_id=s.scope_id AND b.user_id=s.user_id AND b.workspace_id=s.workspace_id AND b.execution_mode='managed'))`,
		`SELECT COUNT(*) FROM pod_run r WHERE r.execution_mode='managed' AND NOT EXISTS(SELECT 1 FROM pod_execution_snapshot s WHERE s.scope_kind='run' AND s.scope_id=r.run_id)`,
		`SELECT COUNT(*) FROM pod_batch b WHERE b.execution_mode='managed' AND NOT EXISTS(SELECT 1 FROM pod_execution_snapshot s WHERE s.scope_kind='batch' AND s.scope_id=b.batch_id)`,
		`SELECT COUNT(*) FROM pod_call_billing b LEFT JOIN pod_provider_call c ON c.submission_key=b.submission_key AND c.run_id=b.run_id AND c.node_id=b.node_id
		 LEFT JOIN pod_run_model_binding m ON m.run_id=b.run_id AND m.node_id=b.node_id AND m.user_id=b.user_id AND m.workspace_id=b.workspace_id AND m.model_version_id=b.model_version_id
		 LEFT JOIN pod_provider_credential_version k ON k.credential_version_id=b.credential_version_id AND k.provider_id=m.provider_id
		 WHERE c.submission_key IS NULL OR m.run_id IS NULL OR k.credential_version_id IS NULL OR b.held_points<0 OR b.charged_points<0 OR b.released_points<0
		 OR b.charged_points+b.released_points>b.held_points OR b.state NOT IN ('held','settled')
		 OR (b.state='held' AND (c.state<>'submitted' OR b.charged_points<>0 OR b.released_points<>0))
		 OR (b.state='settled' AND (c.state<>'confirmed' OR b.charged_points+b.released_points<>b.held_points))`,
		`SELECT COUNT(*) FROM pod_provider_call c JOIN pod_run r ON r.run_id=c.run_id AND r.execution_mode='managed'
		 WHERE NOT EXISTS(SELECT 1 FROM pod_call_billing b WHERE b.submission_key=c.submission_key)`,
		`SELECT COUNT(*) FROM pod_points_account a WHERE
		 a.held<>(SELECT COALESCE(SUM(CASE kind WHEN 'hold' THEN amount WHEN 'settle' THEN -amount WHEN 'release' THEN -amount ELSE 0 END),0) FROM pod_points_event e WHERE e.user_id=a.user_id)
		 OR a.spent<>(SELECT COALESCE(SUM(amount),0) FROM pod_points_event e WHERE e.user_id=a.user_id AND e.kind='settle')`,
	}
	for _, query := range checks {
		var invalid int64
		if err := DB(ctx).Raw(query).Scan(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return errors.New("正式执行账户或调用关联校验失败")
		}
	}
	var versions []model.PODCatalogModelVersion
	var models []model.PODCatalogModel
	var providers []model.PODProvider
	if err := DB(ctx).Find(&versions).Error; err != nil {
		return err
	}
	if err := DB(ctx).Find(&models).Error; err != nil {
		return err
	}
	if err := DB(ctx).Find(&providers).Error; err != nil {
		return err
	}
	vm := map[int64]model.PODCatalogModelVersion{}
	mm := map[int64]model.PODCatalogModel{}
	pm := map[int64]model.PODProvider{}
	for _, v := range versions {
		vm[v.ModelVersionID] = v
	}
	for _, m := range models {
		mm[m.ModelID] = m
	}
	for _, p := range providers {
		pm[p.ProviderID] = p
	}
	verify := func(b model.PODBoundModel) error {
		return verifyManagedBound(b, vm[b.ModelVersionID], mm[b.ModelID], pm[b.ProviderID])
	}
	var bindings []model.PODRunModelBinding
	if err := DB(ctx).Find(&bindings).Error; err != nil {
		return err
	}
	for _, row := range bindings {
		var bound model.PODBoundModel
		if json.Unmarshal(row.SelectionJSON, &bound) != nil || bound.NodeID != row.NodeID || bound.ModelID != row.ModelID || bound.ModelVersionID != row.ModelVersionID || bound.ProviderID != row.ProviderID {
			return errors.New("节点模型绑定快照校验失败")
		}
		if err := verify(bound); err != nil {
			return err
		}
	}
	var quotes []model.PODExecutionQuote
	if err := DB(ctx).Find(&quotes).Error; err != nil {
		return err
	}
	for _, q := range quotes {
		var plan model.PODQuotePlan
		if json.Unmarshal(q.SelectionJSON, &plan) != nil || q.SourceHash != ManagedQuoteSourceHash(plan.Source) || q.CatalogHash != managedHash(plan.Models) {
			return errors.New("报价快照摘要校验失败")
		}
		var points, cost int64
		for _, b := range plan.Models {
			if err := verify(b); err != nil {
				return err
			}
			points += b.Points
			cost += b.CostMicros
		}
		if points != q.PointsUpperBound || cost != q.CostUpperMicros {
			return errors.New("报价汇总校验失败")
		}
	}
	return nil
}

func verifyManagedBound(b model.PODBoundModel, v model.PODCatalogModelVersion, m model.PODCatalogModel, p model.PODProvider) error {
	if v.ModelVersionID != b.ModelVersionID || m.ModelID != b.ModelID || m.ProviderID != b.ProviderID || p.ProviderID != b.ProviderID {
		return errors.New("模型绑定身份校验失败")
	}
	if err := modelcatalog.ValidateHistoricalVersion(p, m, v); err != nil {
		return err
	}
	if err := modelcatalog.ValidateHistoricalVersion(p, m, b.Version); err != nil {
		return err
	}
	var caps model.PODModelCapabilities
	var cost, points model.PODModelRate
	if json.Unmarshal(v.CapabilitiesJSON, &caps) != nil || json.Unmarshal(v.CostRuleJSON, &cost) != nil || json.Unmarshal(v.PointRuleJSON, &points) != nil || b.Version.SpecHash != v.SpecHash || b.Version.ModelVersionID != v.ModelVersionID || b.Kind != m.Kind || !reflect.DeepEqual(caps, b.Capabilities) || cost != b.CostRule || points != b.PointRule {
		return errors.New("模型能力或价格快照校验失败")
	}
	input, output, images := int64(0), int64(0), int64(0)
	if b.Kind == "text" {
		input = int64(caps.MaxInputBytes) + 256
		output = int64(caps.MaxOutputTokens)
		if b.InputTokenBound != input {
			return errors.New("文字预占边界校验失败")
		}
	} else {
		images = int64(b.ImageCount)
		if images < 1 || images > int64(caps.MaxImages) {
			return errors.New("图片预占边界校验失败")
		}
	}
	price, err := modelcatalog.Price(points, input, output, images)
	if err != nil {
		return err
	}
	charge, err := modelcatalog.Price(cost, input, output, images)
	if err != nil {
		return err
	}
	if price != b.Points || charge != b.CostMicros {
		return errors.New("固定计价校验失败")
	}
	return nil
}
