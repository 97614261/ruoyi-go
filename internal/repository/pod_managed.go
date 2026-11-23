package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/modelcatalog"
)

func managedPoints(tx *gorm.DB, uid, workspace, run, batch int64, key, kind string, amount int64) error {
	var a model.PODPointsAccount
	if err := tx.Where("user_id=?", uid).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
		return err
	}
	if amount < 0 || !validPODPoints(a) || a.Revision == math.MaxInt64 {
		return errors.New("invalid managed points account")
	}
	var count int64
	if err := tx.Table("pod_points_event").Where("user_id=? AND event_key=?", uid, key).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return errors.New("duplicate managed accounting event")
	}
	switch kind {
	case "hold":
		if a.Available < amount {
			return errs.WithCode(409, "正式点数不足，请联系管理员发放点数")
		}
		a.Available -= amount
		a.Held += amount
	case "settle":
		if a.Held < amount {
			return errors.New("managed held mismatch")
		}
		a.Held -= amount
		a.Spent += amount
	case "release":
		if a.Held < amount {
			return errors.New("managed held mismatch")
		}
		a.Held -= amount
		a.Available += amount
	case "transfer":
	default:
		return errors.New("invalid managed accounting event")
	}
	if !validPODPoints(a) {
		return errors.New("managed points invariant failed")
	}
	a.Revision++
	if err := tx.Model(&a).Updates(map[string]any{"available": a.Available, "held": a.Held, "spent": a.Spent, "revision": a.Revision, "update_by": "pod-managed", "update_time": podNow()}).Error; err != nil {
		return err
	}
	event := map[string]any{"user_id": uid, "workspace_id": workspace, "event_key": key, "kind": kind, "amount": amount, "available_after": a.Available, "held_after": a.Held, "spent_after": a.Spent, "reason": "正式执行：" + kind, "actor_user_id": uid, "create_by": "pod-managed"}
	if run > 0 {
		event["run_id"] = run
	}
	if batch > 0 {
		event["batch_id"] = batch
	}
	return tx.Table("pod_points_event").Create(event).Error
}
func attachManagedRun(tx *gorm.DB, r *model.PODRun, body model.PODRunCreate, prep *preparedPODRun, prepaid bool) error {
	var bs []model.PODBoundModel
	var snap model.PODExecutionSnapshot
	if prepaid {
		var parent model.PODExecutionSnapshot
		if err := tx.Where("scope_kind='batch' AND scope_id=? AND user_id=?", body.BatchID, r.UserID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&parent).Error; err != nil {
			return err
		}
		var plan model.PODQuotePlan
		if json.Unmarshal(parent.SelectionJSON, &plan) != nil {
			return ErrPODRunInput
		}
		for _, b := range plan.Models {
			if b.ItemNo == body.BatchItemNo {
				bs = append(bs, b)
			}
		}
		amount := int64(0)
		for _, b := range bs {
			amount += b.Points
		}
		if parent.UnallocatedPoints < amount {
			return errors.New("batch hold allocation mismatch")
		}
		if err := validateManagedBindings(tx, r.UserID, bs, false); err != nil {
			return err
		}
		if err := tx.Model(&parent).Update("unallocated_points", parent.UnallocatedPoints-amount).Error; err != nil {
			return err
		}
		snap = model.PODExecutionSnapshot{ScopeKind: "run", ScopeID: r.RunID, WorkspaceID: r.WorkspaceID, UserID: r.UserID, QuoteID: parent.QuoteID, BillingScheme: "managedPointsV1", PolicyRevision: parent.PolicyRevision, SelectionJSON: podJSON(model.PODQuotePlan{Models: bs}), HoldPoints: amount, UnallocatedPoints: amount, CreateTime: podNow()}
		if err := managedPoints(tx, r.UserID, r.WorkspaceID, r.RunID, body.BatchID, fmt.Sprintf("transfer:%d:%d", body.BatchID, body.BatchItemNo), "transfer", amount); err != nil {
			return err
		}
	} else {
		source := model.PODQuoteRequest{Kind: "run", Run: &body}
		q, plan, err := consumeManagedQuote(tx, r.UserID, body.QuoteID, "run", body.RequestID, r.RunID, source)
		if err != nil {
			return err
		}
		bs = plan.Models
		var platform model.PODPlatformState
		if err := tx.Table("pod_platform_state").Where("state_id=1").Take(&platform).Error; err != nil {
			return err
		}
		if err := validateManagedBase(tx, r.BaseRunID, r.UserID, platform, prep); err != nil {
			return err
		}
		snap = model.PODExecutionSnapshot{ScopeKind: "run", ScopeID: r.RunID, WorkspaceID: r.WorkspaceID, UserID: r.UserID, QuoteID: q.QuoteID, BillingScheme: "managedPointsV1", PolicyRevision: q.PolicyRevision, SelectionJSON: q.SelectionJSON, HoldPoints: q.PointsUpperBound, UnallocatedPoints: q.PointsUpperBound, CreateTime: podNow()}
		if err := managedPoints(tx, r.UserID, r.WorkspaceID, r.RunID, 0, "hold:"+q.QuoteID, "hold", q.PointsUpperBound); err != nil {
			return err
		}
	}
	if err := tx.Create(&snap).Error; err != nil {
		return err
	}
	for _, b := range bs {
		row := model.PODRunModelBinding{RunID: r.RunID, NodeID: b.NodeID, WorkspaceID: r.WorkspaceID, UserID: r.UserID, ModelID: b.ModelID, ModelVersionID: b.ModelVersionID, ProviderID: b.ProviderID, SelectionJSON: podJSON(b)}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		result := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND node_id=?", r.RunID, b.NodeID).Updates(map[string]any{"provider": fmt.Sprintf("managed:%d", b.ProviderID), "provider_model": b.Version.OfficialModelID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("managed node binding missing")
		}
	}
	return nil
}
func attachManagedBatch(tx *gorm.DB, b *model.PODBatch) error {
	var q model.PODExecutionQuote
	if err := tx.Where("quote_id=? AND user_id=?", b.QuoteID, b.UserID).Take(&q).Error; err != nil {
		return err
	}
	var p model.PODQuotePlan
	if json.Unmarshal(q.SelectionJSON, &p) != nil || q.SourceHash != b.SourceHash {
		return errs.WithCode(409, "批次输入与报价不一致，请重新报价")
	}
	qptr, _, err := consumeManagedQuote(tx, b.UserID, b.QuoteID, "batch", b.RequestID, b.BatchID, p.Source)
	if err != nil {
		return err
	}
	s := model.PODExecutionSnapshot{ScopeKind: "batch", ScopeID: b.BatchID, WorkspaceID: b.WorkspaceID, UserID: b.UserID, QuoteID: q.QuoteID, BillingScheme: "managedPointsV1", PolicyRevision: q.PolicyRevision, SelectionJSON: q.SelectionJSON, HoldPoints: q.PointsUpperBound, UnallocatedPoints: q.PointsUpperBound, CreateTime: podNow()}
	if err := managedPoints(tx, b.UserID, b.WorkspaceID, 0, b.BatchID, "hold:"+qptr.QuoteID, "hold", qptr.PointsUpperBound); err != nil {
		return err
	}
	return tx.Create(&s).Error
}

func releaseManagedBatchItem(tx *gorm.DB, b *model.PODBatch, item int) error {
	var s model.PODExecutionSnapshot
	if err := tx.Where("scope_kind='batch' AND scope_id=?", b.BatchID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&s).Error; err != nil {
		return err
	}
	var plan model.PODQuotePlan
	if json.Unmarshal(s.SelectionJSON, &plan) != nil {
		return ErrPODRunInput
	}
	var amount int64
	for _, binding := range plan.Models {
		if binding.ItemNo == item {
			amount += binding.Points
		}
	}
	if s.UnallocatedPoints < amount {
		return errors.New("batch item hold mismatch")
	}
	if err := managedPoints(tx, s.UserID, s.WorkspaceID, 0, b.BatchID, fmt.Sprintf("batch-failed:%d:%d", b.BatchID, item), "release", amount); err != nil {
		return err
	}
	return tx.Model(&s).Update("unallocated_points", s.UnallocatedPoints-amount).Error
}
func releaseManagedSnapshot(tx *gorm.DB, kind string, id int64, key string) error {
	var s model.PODExecutionSnapshot
	if err := tx.Where("scope_kind=? AND scope_id=?", kind, id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&s).Error; err != nil {
		return err
	}
	if s.UnallocatedPoints == 0 {
		return nil
	}
	run, batch := int64(0), int64(0)
	if kind == "run" {
		run = id
	} else {
		batch = id
	}
	if err := managedPoints(tx, s.UserID, s.WorkspaceID, run, batch, key, "release", s.UnallocatedPoints); err != nil {
		return err
	}
	return tx.Model(&s).Update("unallocated_points", 0).Error
}
func settleManagedRun(tx *gorm.DB, r *model.PODRun) error {
	var held int64
	if err := tx.Model(&model.PODCallBilling{}).Where("run_id=? AND state='held'", r.RunID).Count(&held).Error; err != nil {
		return err
	}
	if held > 0 {
		return errors.New("cannot settle an uncertain managed call")
	}
	if err := releaseManagedSnapshot(tx, "run", r.RunID, fmt.Sprintf("run-release:%d:%d", r.RunID, r.Attempt)); err != nil {
		return err
	}
	r.Settled = true
	r.Reserved = 0
	return nil
}
func PODManagedBilling(ctx context.Context, uid int64, kind string, id int64) (*model.PODBillingView, error) {
	v := &model.PODBillingView{Scheme: "managedPointsV1"}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var s model.PODExecutionSnapshot
		if err := tx.Where("scope_kind=? AND scope_id=? AND user_id=?", kind, id, uid).Take(&s).Error; err != nil {
			return err
		}
		v.Held = s.UnallocatedPoints
		q := tx.Model(&model.PODCallBilling{}).Where("user_id=?", uid)
		if kind == "batch" {
			var pending int64
			if err := tx.Table("pod_execution_snapshot s").Joins("JOIN pod_batch_item i ON i.run_id=s.scope_id AND i.batch_id=?", id).Where("s.scope_kind='run' AND s.user_id=?", uid).Select("COALESCE(SUM(s.unallocated_points),0)").Scan(&pending).Error; err != nil {
				return err
			}
			v.Held += pending
			q = q.Where("run_id IN (SELECT run_id FROM pod_batch_item WHERE batch_id=? AND workspace_id=?)", id, s.WorkspaceID)
		} else {
			q = q.Where("run_id=?", id)
		}
		var totals struct{ Held, Spent int64 }
		if err := q.Select("COALESCE(SUM(CASE WHEN state='held' THEN held_points ELSE 0 END),0) held,COALESCE(SUM(charged_points),0) spent").Scan(&totals).Error; err != nil {
			return err
		}
		v.Held += totals.Held
		v.Spent = totals.Spent
		v.Released = s.HoldPoints - v.Held - v.Spent
		// A retry of a dispatched item owns a separate hold; batch history includes that explicit addition.
		if kind == "batch" {
			var additional int64
			var parent model.PODQuotePlan
			if json.Unmarshal(s.SelectionJSON, &parent) != nil {
				return ErrPODRunInput
			}
			if err := tx.Table("pod_execution_snapshot s").Joins("JOIN pod_batch_item i ON i.run_id=s.scope_id AND i.batch_id=?", id).Where("s.scope_kind='run' AND s.user_id=?", uid).Select("COALESCE(SUM(s.hold_points),0)").Scan(&additional).Error; err != nil {
				return err
			}
			var unassigned int64
			var plan model.PODQuotePlan
			_ = json.Unmarshal(s.SelectionJSON, &plan)
			var waiting []int
			if err := tx.Table("pod_batch_item").Where("batch_id=? AND run_id=0", id).Pluck("item_no", &waiting).Error; err != nil {
				return err
			}
			for _, b := range plan.Models {
				for _, item := range waiting {
					if b.ItemNo == item {
						unassigned += b.Points
						break
					}
				}
			}
			v.Released = additional + unassigned - v.Held - v.Spent
		}
		if v.Released < 0 {
			return errors.New("managed billing summary mismatch")
		}
		return nil
	})
	return v, err
}
func ManagedBound(ctx context.Context, c *PODClaim) (model.PODBoundModel, *model.PODProviderCredential, error) {
	var row model.PODRunModelBinding
	var b model.PODBoundModel
	if err := DB(ctx).Where("run_id=? AND node_id=? AND user_id=?", c.Run.RunID, c.Node.NodeID, c.Run.UserID).Take(&row).Error; err != nil {
		return b, nil, err
	}
	if json.Unmarshal(row.SelectionJSON, &b) != nil {
		return b, nil, ErrPODRunInput
	}
	var p model.PODProvider
	if err := DB(ctx).Where("provider_id=?", row.ProviderID).Take(&p).Error; err != nil {
		return b, nil, err
	}
	if p.CurrentCredentialVersionID == nil {
		return b, nil, errs.WithCode(409, "模型凭据未配置")
	}
	var cr model.PODProviderCredential
	if err := DB(ctx).Where("credential_version_id=? AND provider_id=? AND state='active'", *p.CurrentCredentialVersionID, p.ProviderID).Take(&cr).Error; err != nil {
		return b, nil, errs.WithCode(409, "模型凭据不可用")
	}
	return b, &cr, nil
}

// SelectManagedBBRootImageCredential only resolves the original credential for a durable receipt.
func SelectManagedBBRootImageCredential(ctx context.Context, c *PODClaim) (*model.PODProviderCredential, error) {
	var credential model.PODProviderCredential
	err := DB(ctx).Table("pod_provider_call pc").Select("cr.*").
		Joins("JOIN pod_call_billing cb ON cb.submission_key=pc.submission_key AND cb.run_id=pc.run_id AND cb.workspace_id=pc.workspace_id").
		Joins("JOIN pod_run_model_binding b ON b.run_id=cb.run_id AND b.node_id=cb.node_id AND b.user_id=cb.user_id AND b.workspace_id=cb.workspace_id AND b.model_version_id=cb.model_version_id").
		Joins("JOIN pod_model_version v ON v.model_version_id=b.model_version_id AND v.model_id=b.model_id").
		Joins("JOIN pod_provider_credential_version cr ON cr.credential_version_id=cb.credential_version_id AND cr.provider_id=b.provider_id AND cr.state='active'").
		Where("pc.submission_key=? AND pc.run_id=? AND pc.workspace_id=? AND pc.node_id=? AND pc.state='confirmed' AND cb.node_id=? AND cb.user_id=?", c.Node.SubmissionKey, c.Run.RunID, c.Run.WorkspaceID, c.Node.NodeID, c.Node.NodeID, c.Run.UserID).
		Where("v.adapter_kind='openai-compatible-v1' AND v.endpoint=?", executor.BBRootImageAPIEndpoint).
		Take(&credential).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &credential, err
}

func BeginManagedCall(ctx context.Context, c *PODClaim, credential, budget int64) (bool, error) {
	fresh := false
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		p, err := lockManagedPlatform(tx)
		if err != nil {
			return err
		}
		if !p.ManagedEnabled {
			return errs.WithCode(409, "正式执行已停用")
		}
		if _, err := lockedPODWorkspace(tx, c.Run.UserID); err != nil {
			return err
		}
		if err := checkPODClaim(tx, c); err != nil {
			return err
		}
		var old model.PODProviderCall
		err = tx.Where("submission_key=?", c.Node.SubmissionKey).Take(&old).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var row model.PODRunModelBinding
		if err := tx.Where("run_id=? AND node_id=? AND user_id=?", c.Run.RunID, c.Node.NodeID, c.Run.UserID).Take(&row).Error; err != nil {
			return err
		}
		var b model.PODBoundModel
		if json.Unmarshal(row.SelectionJSON, &b) != nil {
			return ErrPODRunInput
		}
		if err := validateManagedBindings(tx, c.Run.UserID, []model.PODBoundModel{b}, false); err != nil {
			return err
		}
		var provider model.PODProvider
		if err := tx.Where("provider_id=?", b.ProviderID).Take(&provider).Error; err != nil {
			return err
		}
		if provider.CurrentCredentialVersionID == nil || *provider.CurrentCredentialVersionID != credential {
			return errs.WithCode(409, "供应商凭据已变化，本次尚未调用")
		}
		var used int64
		if err := tx.Model(&model.PODProviderCall{}).Select("COALESCE(SUM(reserved_micros+charged_micros),0)").Scan(&used).Error; err != nil {
			return err
		}
		if budget < 0 || b.CostMicros < 0 || budget < b.CostMicros || used > budget-b.CostMicros {
			return ErrPODBudget
		}
		var snap model.PODExecutionSnapshot
		if err := tx.Where("scope_kind='run' AND scope_id=? AND user_id=?", c.Run.RunID, c.Run.UserID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&snap).Error; err != nil {
			return err
		}
		if snap.UnallocatedPoints < b.Points {
			return errors.New("managed call has no prepaid allocation")
		}
		if err := tx.Model(&snap).Update("unallocated_points", snap.UnallocatedPoints-b.Points).Error; err != nil {
			return err
		}
		billing := model.PODCallBilling{SubmissionKey: c.Node.SubmissionKey, RunID: c.Run.RunID, NodeID: c.Node.NodeID, UserID: c.Run.UserID, WorkspaceID: c.Run.WorkspaceID, ModelVersionID: b.ModelVersionID, CredentialVersionID: credential, HeldPoints: b.Points, State: "held", UsageJSON: json.RawMessage(`{}`), CreateTime: podNow()}
		if err := tx.Create(&billing).Error; err != nil {
			return err
		}
		call := model.PODProviderCall{SubmissionKey: c.Node.SubmissionKey, RunID: c.Run.RunID, NodeID: c.Node.NodeID, WorkspaceID: c.Run.WorkspaceID, Model: b.Version.OfficialModelID, State: "submitted", ReservedMicros: b.CostMicros, ReceiptJSON: json.RawMessage(`{}`), CreateBy: "pod-managed", UpdateBy: "pod-managed", CreateTime: podNow(), UpdateTime: podNow()}
		if err := tx.Create(&call).Error; err != nil {
			return err
		}
		fresh = true
		return nil
	})
	return fresh && err == nil, err
}
func SaveManagedReceipt(ctx context.Context, c *PODClaim, receipt executor.QwenReceipt) error {
	for _, ref := range receipt.ImageURLs {
		if strings.HasPrefix(ref, "data:") {
			return errors.New("inline images must be staged before saving a managed receipt")
		}
	}
	// Receipts can contain private text or signed URLs.
	return DB(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if _, err := lockManagedPlatform(tx); err != nil {
			return err
		}
		if _, err := lockedPODWorkspaceRaw(tx, c.Run.UserID); err != nil && !errors.Is(err, ErrPODWorkspace) {
			return err
		}
		if err := checkPODClaim(tx, c); err != nil {
			return err
		}
		var call model.PODProviderCall
		if err := tx.Where("submission_key=? AND run_id=?", c.Node.SubmissionKey, c.Run.RunID).Take(&call).Error; err != nil {
			return err
		}
		if call.State == "confirmed" {
			return nil
		}
		updates := map[string]any{"receipt_json": string(podJSON(receipt)), "update_time": podNow()}
		if receipt.State == "awaitingReconciliation" {
			return tx.Model(&call).Updates(updates).Error
		}
		if receipt.State != "succeeded" && receipt.State != "failed" {
			return errors.New("invalid managed receipt state")
		}
		var binding model.PODRunModelBinding
		if err := tx.Where("run_id=? AND node_id=?", c.Run.RunID, c.Node.NodeID).Take(&binding).Error; err != nil {
			return err
		}
		var b model.PODBoundModel
		if json.Unmarshal(binding.SelectionJSON, &b) != nil {
			return ErrPODRunInput
		}
		cost, points, exceeded, err := modelcatalog.UsagePrice(b, receipt.Usage)
		if err != nil {
			return err
		}
		receipt.Usage.EstimatedCostMicros = cost
		updates["receipt_json"] = string(podJSON(receipt))
		var billing model.PODCallBilling
		if err := tx.Where("submission_key=? AND run_id=? AND state='held'", c.Node.SubmissionKey, c.Run.RunID).Take(&billing).Error; err != nil {
			return err
		}
		points = min(points, billing.HeldPoints)
		release := billing.HeldPoints - points
		if err := managedPoints(tx, c.Run.UserID, c.Run.WorkspaceID, c.Run.RunID, 0, "settle:"+c.Node.SubmissionKey, "settle", points); err != nil {
			return err
		}
		if err := managedPoints(tx, c.Run.UserID, c.Run.WorkspaceID, c.Run.RunID, 0, "release:"+c.Node.SubmissionKey, "release", release); err != nil {
			return err
		}
		if err := tx.Model(&billing).Updates(map[string]any{"charged_points": points, "released_points": release, "state": "settled", "usage_json": string(podJSON(receipt.Usage))}).Error; err != nil {
			return err
		}
		updates["state"] = "confirmed"
		updates["reserved_micros"] = 0
		updates["charged_micros"] = cost
		if err := tx.Model(&call).Updates(updates).Error; err != nil {
			return err
		}
		if exceeded {
			if err := tx.Table("pod_model").Where("model_id=?", b.ModelID).Updates(map[string]any{"status": "disabled", "revision": gorm.Expr("revision+1"), "update_by": "pod-bound-guard", "update_time": podNow()}).Error; err != nil {
				return err
			}
			if err := tx.Table("pod_platform_state").Where("state_id=1").Update("revision", gorm.Expr("revision+1")).Error; err != nil {
				return err
			}
			slog.Error("POD 模型用量超过预占上界，已记录实际成本并停用模型", "modelId", b.ModelID, "runId", c.Run.RunID)
		}
		return nil
	})
}

func RetryPODManagedRun(ctx context.Context, user *model.SysUser, id int64, key, quote string, l model.PODExecutionLimits) (*model.PODRun, error) {
	var r model.PODRun
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		p, err := lockManagedPlatform(tx)
		if err != nil {
			return err
		}
		if !p.ManagedEnabled {
			return errs.WithCode(409, "正式执行尚未启用")
		}
		w, err := lockedPODWorkspace(tx, user.UserID)
		if err != nil {
			return err
		}
		if err := tx.Where("run_id=? AND workspace_id=? AND execution_mode='managed'", id, w.WorkspaceID).Take(&r).Error; err != nil {
			return ErrPODRun
		}
		var prior model.PODRetryRequest
		err = tx.Where("workspace_id=? AND request_id=?", w.WorkspaceID, key).Take(&prior).Error
		if err == nil {
			var consumed model.PODExecutionQuote
			if prior.RunID != id {
				return ErrPODRequest
			}
			if err := tx.Where("quote_id=? AND user_id=? AND kind='retry' AND state='consumed' AND consumed_resource_id=? AND consumed_request_id=?", quote, user.UserID, id, key).Take(&consumed).Error; err != nil {
				return ErrPODRequest
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if r.State != model.PODRunFailed || !r.Settled || r.Attempt >= 5 {
			return ErrPODRunState
		}
		q, plan, err := consumeManagedQuote(tx, user.UserID, quote, "retry", key, id, model.PODQuoteRequest{Kind: "retry", RunID: strconv.FormatInt(id, 10)})
		if err != nil {
			return err
		}
		if plan.RunAttempt != r.Attempt {
			return errs.WithCode(409, "运行已变化，请重新报价")
		}
		retryNodes := make([]model.PODNode, 0, len(plan.Models))
		for _, binding := range plan.Models {
			if binding.Kind == "image" {
				retryNodes = append(retryNodes, model.PODNode{ID: binding.NodeID, Type: "imageGenerate"})
			}
		}
		if err := checkPODProductRunOverlap(tx, w.WorkspaceID, r.ProjectID, retryNodes); err != nil {
			return err
		}
		if err := checkPODPending(tx, w.WorkspaceID, l); err != nil {
			return err
		}
		if err := acceptCreatorSubmissions(tx, user.UserID, 1); err != nil {
			return err
		}
		if err := managedPoints(tx, r.UserID, r.WorkspaceID, r.RunID, 0, "hold:"+q.QuoteID, "hold", q.PointsUpperBound); err != nil {
			return err
		}
		updated := tx.Model(&model.PODExecutionSnapshot{}).Where("scope_kind='run' AND scope_id=? AND unallocated_points=0", id).Updates(map[string]any{"hold_points": gorm.Expr("hold_points+?", q.PointsUpperBound), "unallocated_points": q.PointsUpperBound, "quote_id": q.QuoteID})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("managed retry snapshot mismatch")
		}
		r.Attempt++
		r.State = model.PODRunQueued
		r.Settled = false
		if err := tx.Model(&r).Updates(map[string]any{"attempt": r.Attempt, "state": r.State, "settled": false, "update_time": podNow()}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND state<>'succeeded'", id).Updates(map[string]any{"state": "waiting", "lease_until": nil, "lease_token": "", "submission_key": "", "error_code": "", "error_message": "", "update_time": podNow()}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PODRetryRequest{WorkspaceID: w.WorkspaceID, RequestID: key, RunID: id, Attempt: r.Attempt, CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: podNow(), UpdateTime: podNow()}).Error; err != nil {
			return err
		}
		return tx.Model(&model.PODOutbox{}).Where("run_id=?", id).Updates(map[string]any{"active": true, "next_delivery_at": podNow()}).Error
	})
	return &r, err
}
