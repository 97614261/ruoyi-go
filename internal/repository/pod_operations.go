package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func operationMissing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.WithCode(404, "记录不存在或没有访问权限")
	}
	return err
}
func adminTasks(tx *gorm.DB, f model.PODTaskFilter, batch bool) *gorm.DB {
	table, id := "pod_run", "run_id"
	if batch {
		table, id = "pod_batch", "batch_id"
	}
	q := tx.Table(table+" r").Where("r.create_time>=? AND r.create_time<?", f.Begin, f.End)
	if f.UserID > 0 {
		q = q.Where("r.user_id=?", f.UserID)
	}
	if f.State != "" {
		q = q.Where("r.state=?", f.State)
	}
	if f.ModelID > 0 {
		if batch {
			q = q.Where("EXISTS (SELECT 1 FROM pod_batch_item i JOIN pod_run_model_binding m ON m.run_id=i.run_id WHERE i.batch_id=r."+id+" AND m.model_id=?)", f.ModelID)
		} else {
			q = q.Where("EXISTS (SELECT 1 FROM pod_run_model_binding m WHERE m.run_id=r.run_id AND m.model_id=?)", f.ModelID)
		}
	}
	return q
}
func PODAdminTasks(ctx context.Context, f model.PODTaskFilter, p page.Query) ([]model.PODTaskView, int64, error) {
	var total int64
	if err := adminTasks(DB(ctx), f, false).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODTaskView{}
	err := adminTasks(DB(ctx), f, false).Select("r.run_id,r.user_id,r.project_id,r.execution_mode,r.mode,r.state,r.attempt,r.create_time,r.update_time").Order("r.create_time DESC,r.run_id DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODAdminTask(ctx context.Context, id int64) (*model.PODTaskView, error) {
	var row model.PODTaskView
	err := DB(ctx).Table("pod_run").Select("run_id,user_id,project_id,execution_mode,mode,state,attempt,create_time,update_time").Where("run_id=?", id).Take(&row).Error
	return &row, operationMissing(err)
}
func PODAdminTaskNodes(ctx context.Context, id int64, p page.Query) ([]model.PODTaskNodeView, int64, error) {
	if _, err := PODAdminTask(ctx, id); err != nil {
		return nil, 0, err
	}
	var total int64
	q := DB(ctx).Table("pod_node_run n").Where("n.run_id=?", id)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODTaskNodeView{}
	err := q.Joins("LEFT JOIN pod_run_model_binding b ON b.run_id=n.run_id AND b.node_id=n.node_id").Select("n.node_id,n.node_type,n.state,n.attempt,COALESCE(b.model_id,0) model_id,COALESCE(b.model_version_id,0) model_version_id,n.error_code").Order("n.node_id").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODAdminBatches(ctx context.Context, f model.PODTaskFilter, p page.Query) ([]model.PODAdminBatchView, int64, error) {
	var total int64
	if err := adminTasks(DB(ctx), f, true).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODAdminBatchView{}
	err := adminTasks(DB(ctx), f, true).Select("r.batch_id,r.user_id,r.state,r.execution_mode,r.create_time").Order("r.create_time DESC,r.batch_id DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODAdminBatch(ctx context.Context, id int64) (*model.PODAdminBatchView, []model.PODAdminBatchItem, error) {
	var row model.PODAdminBatchView
	if err := DB(ctx).Table("pod_batch").Select("batch_id,user_id,state,execution_mode,create_time").Where("batch_id=?", id).Take(&row).Error; err != nil {
		return nil, nil, operationMissing(err)
	}
	items := []model.PODAdminBatchItem{}
	err := DB(ctx).Table("pod_batch_item").Select("item_no,state,run_id,error_code").Where("batch_id=?", id).Order("item_no").Limit(10).Scan(&items).Error
	return &row, items, err
}
func PODAdminUsage(ctx context.Context, f model.PODTaskFilter, p page.Query) ([]model.PODUsageGroup, int64, error) {
	q := DB(ctx).Table("pod_provider_call c").Joins("JOIN pod_run r ON r.run_id=c.run_id").Joins("LEFT JOIN pod_run_model_binding b ON b.run_id=c.run_id AND b.node_id=c.node_id").Joins("LEFT JOIN pod_call_billing p ON p.submission_key=c.submission_key").Where("c.create_time>=? AND c.create_time<?", f.Begin, f.End)
	if f.UserID > 0 {
		q = q.Where("r.user_id=?", f.UserID)
	}
	if f.ModelID > 0 {
		q = q.Where("b.model_id=?", f.ModelID)
	}
	group := "r.user_id,COALESCE(b.model_id,0),c.model"
	var total int64
	if err := DB(ctx).Table("(?) groups_count", q.Session(&gorm.Session{}).Select(group).Group(group)).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODUsageGroup{}
	err := q.Select("r.user_id,COALESCE(b.model_id,0) model_id,c.model,COUNT(*) calls,COALESCE(SUM(c.charged_micros),0) charged_micros,COALESCE(SUM(c.reserved_micros),0) reserved_micros,COALESCE(SUM(p.charged_points),0) points_spent,COALESCE(SUM(CASE WHEN p.state='held' THEN p.held_points-p.charged_points-p.released_points ELSE 0 END),0) points_held").Group(group).Order("r.user_id,model_id,c.model").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error
	return rows, total, err
}
func PODPrivateGrants(ctx context.Context, p page.Query) ([]model.PODPrivateGrant, int64, error) {
	rows := []model.PODPrivateGrant{}
	var total int64
	q := DB(ctx).Model(&model.PODPrivateGrant{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("grant_id DESC").Offset(p.Offset()).Limit(p.PageSize).Find(&rows).Error
	return rows, total, err
}
func privateOwner(tx *gorm.DB, kind string, id int64) (int64, error) {
	var owner int64
	q := tx.Table("pod_run").Select("user_id").Where("run_id=?", id)
	if kind == "asset" {
		q = tx.Table("pod_asset a").Joins("JOIN pod_workspace w ON w.workspace_id=a.workspace_id").Select("w.owner_user_id").Where("a.asset_id=? AND a.del_flag='0'", id)
	}
	if err := q.Take(&owner).Error; err != nil {
		return 0, operationMissing(err)
	}
	return owner, nil
}
func operationAudit(tx *gorm.DB, actor, owner, id int64, op, kind, reason, trace string, s model.PODOperationAudit) error {
	return tx.Create(&model.PODAdminAudit{ActorUserID: actor, OwnerUserID: owner, ActionType: op, ResourceType: kind, ResourceID: id, Reason: reason, BeforeJSON: podJSON(model.PODAdminAuditSnapshot{}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{Operation: &s}), TraceID: trace, Outcome: "success", CreateBy: strconv.FormatInt(actor, 10), CreateTime: types.Now()}).Error
}

// Execute an admin action under the same gate as Worker dispatch and billing.
// No owner impersonation, external I/O or terminal-state rewriting occurs here.
func PODOperationWrite(ctx context.Context, in model.PODOperationMutation) (*model.PODCatalogResult, error) {
	var result model.PODCatalogResult
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		for _, p := range []string{model.PODAdminAccess, in.Permission} {
			if err := catalogActor(tx, in.ActorID, p); err != nil {
				return err
			}
		}
		var old model.PODAdminAction
		err := tx.Where("actor_user_id=? AND request_id=?", in.ActorID, in.Request.RequestID).Take(&old).Error
		if err == nil {
			if old.ActionType != in.Op || old.RequestHash != in.Hash {
				return errs.WithCode(409, "请求编号已用于其他操作")
			}
			return json.Unmarshal(old.ResultJSON, &result)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		kind := in.Request.ResourceType
		id, owner, revision := in.ID, int64(0), int64(1)
		snapshot := model.PODOperationAudit{}
		switch in.Op {
		case "task.cancel":
			var r model.PODRun
			if err := tx.Select("run_id,user_id,workspace_id,state").Where("run_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&r).Error; err != nil {
				return operationMissing(err)
			}
			kind, owner = "run", r.UserID
			if !terminalAdminRun(r.State) {
				if err := tx.Model(&model.PODRun{}).Where("run_id=?", id).Updates(map[string]any{"state": model.PODRunCancelRequested, "update_by": strconv.FormatInt(in.ActorID, 10), "update_time": podNow()}).Error; err != nil {
					return err
				}
				if err := tx.Model(&model.PODOutbox{}).Where("run_id=?", id).Updates(map[string]any{"active": true, "next_delivery_at": podNow(), "update_time": podNow()}).Error; err != nil {
					return err
				}
				r.State = model.PODRunCancelRequested
			}
			snapshot.State = string(r.State)
		case "batch.stop":
			var b model.PODBatch
			if err := tx.Where("batch_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&b).Error; err != nil {
				return operationMissing(err)
			}
			kind, owner = "batch", b.UserID
			if b.ExecutionMode == "managed" {
				if err := releaseManagedSnapshot(tx, "batch", id, "batch-stop:"+strconv.FormatInt(id, 10)); err != nil {
					return err
				}
			}
			if err := tx.Model(&model.PODBatchItem{}).Where("batch_id=? AND state='waiting'", id).Updates(map[string]any{"state": "cancelled", "error_code": "", "update_time": types.Now(), "update_by": strconv.FormatInt(in.ActorID, 10)}).Error; err != nil {
				return err
			}
			if err := tx.Model(&b).Updates(map[string]any{"state": "stopped", "update_time": types.Now(), "update_by": strconv.FormatInt(in.ActorID, 10)}).Error; err != nil {
				return err
			}
			snapshot.State = "stopped"
		case "private.grant":
			g := in.Grant
			actual, err := privateOwner(tx, g.ResourceType, g.ResourceID)
			if err != nil {
				return err
			}
			if actual != g.OwnerUserID {
				return errs.WithCode(404, "资源与所有者不匹配")
			}
			for _, p := range []string{model.PODAdminAccess, "podAdmin:private:read"} {
				if err := catalogActor(tx, g.OperatorUserID, p); err != nil {
					return err
				}
			}
			if !g.ExpiresAt.After(time.Now()) || g.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
				return errs.WithCode(400, "授权有效期须在未来 24 小时内")
			}
			g.Revision = 1
			g.CreateBy = strconv.FormatInt(in.ActorID, 10)
			g.CreateTime = types.Now()
			if err := tx.Create(&g).Error; err != nil {
				return err
			}
			kind, id, owner = "privateGrant", g.GrantID, g.OwnerUserID
			snapshot = model.PODOperationAudit{OperatorUserID: g.OperatorUserID, ResourceType: g.ResourceType, ResourceID: g.ResourceID, ExpiresAt: &g.ExpiresAt, Revision: 1}
		case "private.revoke":
			var g model.PODPrivateGrant
			if err := tx.Where("grant_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&g).Error; err != nil {
				return operationMissing(err)
			}
			if g.Revision != in.Revision {
				return ErrPODRevision
			}
			kind, owner, revision = "privateGrant", g.OwnerUserID, g.Revision+1
			if err := tx.Model(&g).Updates(map[string]any{"revoked": true, "revision": revision, "update_by": strconv.FormatInt(in.ActorID, 10), "update_time": types.Now()}).Error; err != nil {
				return err
			}
			snapshot.State = "revoked"
			snapshot.Revision = revision
		default:
			return errs.WithCode(400, "不支持的管理操作")
		}
		result = model.PODCatalogResult{RequestID: in.Request.RequestID, ActionType: in.Op, ResourceType: kind, ResourceID: id, Revision: revision}
		if err := operationAudit(tx, in.ActorID, owner, id, in.Op, kind, in.Request.Reason, in.Trace, snapshot); err != nil {
			return err
		}
		return tx.Create(&model.PODAdminAction{ActorUserID: in.ActorID, RequestID: in.Request.RequestID, RequestHash: in.Hash, ActionType: in.Op, ResourceType: kind, ResourceID: id, State: "completed", ResultJSON: podJSON(result), CreateBy: strconv.FormatInt(in.ActorID, 10), CreateTime: types.Now()}).Error
	})
	return &result, err
}
func terminalAdminRun(s model.PODRunState) bool {
	return s == model.PODRunSucceeded || s == model.PODRunFailed || s == model.PODRunCancelled
}

func privateRead(tx *gorm.DB, actor, id int64, kind, trace string) error {
	for _, p := range []string{model.PODAdminAccess, "podAdmin:private:read"} {
		if err := catalogActor(tx, actor, p); err != nil {
			return err
		}
	}
	owner, err := privateOwner(tx, kind, id)
	if err != nil {
		return err
	}
	var g model.PODPrivateGrant
	err = tx.Where("operator_user_id=? AND owner_user_id=? AND resource_type=? AND resource_id=? AND revoked=0 AND expires_at>CURRENT_TIMESTAMP(3)", actor, owner, kind, id).Order("grant_id DESC").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.WithCode(403, "需要该资源的有效私有访问授权")
	}
	if err != nil {
		return err
	}
	return operationAudit(tx, actor, owner, id, "private.read", kind, g.Purpose, trace, model.PODOperationAudit{ResourceType: kind, ResourceID: id, Revision: g.Revision})
}
func PODAdminPrivateRun(ctx context.Context, actor, id int64, trace string) (*model.PODPrivateRun, error) {
	out := &model.PODPrivateRun{RunID: id, Nodes: []model.PODPrivateNode{}}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := privateRead(tx, actor, id, "run", trace); err != nil {
			return err
		}
		var r model.PODRun
		if err := tx.Select("run_id,user_id,graph_json").Where("run_id=?", id).Take(&r).Error; err != nil {
			return err
		}
		out.UserID = r.UserID
		if err := json.Unmarshal(r.GraphJSON, &out.Graph); err != nil {
			return err
		}
		nodes := []model.PODNodeRun{}
		if err := tx.Select("node_id,output_json").Where("run_id=?", id).Order("node_id").Limit(100).Find(&nodes).Error; err != nil {
			return err
		}
		for _, n := range nodes {
			v := model.PODPrivateNode{NodeID: n.NodeID}
			if err := json.Unmarshal(n.OutputJSON, &v.Output); err != nil {
				return err
			}
			out.Nodes = append(out.Nodes, v)
		}
		return nil
	})
	if err != nil {
		return nil, recordPrivateReadDenial(ctx, actor, id, "run", trace, err)
	}
	return out, nil
}
func PODAdminPrivateAsset(ctx context.Context, actor, id int64, variant, trace string) (*model.PODAssetFile, string, error) {
	var f model.PODAssetFile
	var name string
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := privateRead(tx, actor, id, "asset", trace); err != nil {
			return err
		}
		if err := tx.Table("pod_asset").Select("original_name").Where("asset_id=?", id).Take(&name).Error; err != nil {
			return err
		}
		return tx.Table("pod_asset_file").Where("asset_id=? AND variant=?", id, variant).Take(&f).Error
	})
	if err != nil {
		return nil, "", recordPrivateReadDenial(ctx, actor, id, "asset", trace, operationMissing(err))
	}
	return &f, name, nil
}

func recordPrivateReadDenial(ctx context.Context, actor, id int64, kind, trace string, cause error) error {
	b := errs.As(cause)
	if b == nil || (b.Code != 403 && b.Code != 404) {
		return cause
	}
	row := model.PODAdminAudit{ActorUserID: actor, ActionType: "private.read", ResourceType: kind, ResourceID: id, Reason: "未满足资源访问授权", BeforeJSON: podJSON(model.PODAdminAuditSnapshot{}), AfterJSON: podJSON(model.PODAdminAuditSnapshot{}), TraceID: trace, Outcome: "denied", CreateBy: strconv.FormatInt(actor, 10), CreateTime: types.Now()}
	if err := DB(ctx).Create(&row).Error; err != nil {
		return errs.WithCode(500, "私有访问审计暂不可用")
	}
	return cause
}
