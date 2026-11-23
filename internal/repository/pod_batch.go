package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var ErrPODBatch = errors.New("POD batch not found")

const podBatchOwner = "EXISTS (SELECT 1 FROM pod_workspace w JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0' WHERE w.workspace_id=pod_batch.workspace_id AND w.owner_user_id=? AND w.status='0')"

func FindPODBatchRequest(ctx context.Context, userID int64, requestID, hash string) (*model.PODBatch, error) {
	var row model.PODBatch
	err := DB(ctx).Where("request_id=?", requestID).Where(podBatchOwner, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestHash != hash {
		return nil, ErrPODRequest
	}
	return &row, nil
}

func CreatePODBatch(ctx context.Context, user *model.SysUser, batch *model.PODBatch, items []model.PODBatchItem, refs []model.PODProjectAssetRef, limits model.PODExecutionLimits) (*model.PODBatch, error) {
	var result model.PODBatch
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if batch.ExecutionMode == "managed" {
			if _, e := lockManagedPlatform(tx); e != nil {
				return e
			}
		}
		w, err := lockedPODWorkspace(tx, user.UserID)
		if err != nil {
			return err
		}
		err = tx.Where("workspace_id=? AND request_id=?", w.WorkspaceID, batch.RequestID).Take(&result).Error
		if err == nil {
			if result.RequestHash != batch.RequestHash {
				return ErrPODRequest
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var source model.PODTemplateVersion
		if err = podTemplateQuery(tx, user.UserID).Where("t.template_id=? AND t.del_flag='0'", batch.TemplateID).Select("t.template_id").Take(&model.PODTemplate{}).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODTemplate
		} else if err != nil {
			return err
		}
		if err = tx.Table("pod_template_version").Where("template_id=? AND version=? AND workspace_id=?", batch.TemplateID, batch.TemplateVersion, w.WorkspaceID).Take(&source).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODTemplate
		} else if err != nil {
			return err
		}
		var counts struct {
			Total int64
			Own   int64
		}
		if err = tx.Table("pod_batch_item").Select("COUNT(*) AS total, COALESCE(SUM(workspace_id=?),0) AS own", w.WorkspaceID).Where("state='waiting'").Scan(&counts).Error; err != nil {
			return err
		}
		if counts.Total+int64(len(items)) > 200 || counts.Own+int64(len(items)) > 20 {
			return ErrPODQueueFull
		}
		if err = validateTemplateRefs(tx, w.WorkspaceID, refs); err != nil {
			return err
		}
		p, err := checkCreator(tx, user.UserID, true, true)
		if err != nil {
			return err
		}
		var active int64
		if err = tx.Model(&model.PODRun{}).Where("user_id=? AND state IN ?", user.UserID, podActiveRunStates).Count(&active).Error; err != nil {
			return err
		}
		if active+counts.Own+int64(len(items)) > int64(min(p.MaxPendingRuns, limits.MaxUserPending)) {
			return ErrPODQueueFull
		}
		if err = acceptCreatorSubmissions(tx, user.UserID, len(items)); err != nil {
			return err
		}
		batch.WorkspaceID = w.WorkspaceID
		batch.UserID = user.UserID
		if err = tx.Create(batch).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].BatchID = batch.BatchID
			items[i].WorkspaceID = w.WorkspaceID
		}
		if err = tx.Create(&items).Error; err != nil {
			return err
		}
		unique := map[int64]bool{}
		assetRefs := []model.PODBatchAssetRef{}
		for _, ref := range refs {
			if !unique[ref.AssetID] {
				unique[ref.AssetID] = true
				assetRefs = append(assetRefs, model.PODBatchAssetRef{BatchID: batch.BatchID, AssetID: ref.AssetID, WorkspaceID: w.WorkspaceID})
			}
		}
		if len(assetRefs) > 0 {
			if err = tx.CreateInBatches(&assetRefs, 200).Error; err != nil {
				return err
			}
		}
		if batch.ExecutionMode == "managed" {
			if err := attachManagedBatch(tx, batch); err != nil {
				return err
			}
		}
		result = *batch
		return nil
	})
	return &result, err
}

func SelectPODBatch(ctx context.Context, userID, id int64) (*model.PODBatch, []model.PODBatchItemView, error) {
	var row model.PODBatch
	err := DB(ctx).Where("batch_id=?", id).Where(podBatchOwner, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrPODBatch
	}
	if err != nil {
		return nil, nil, err
	}
	items := []model.PODBatchItemView{}
	err = DB(ctx).Table("pod_batch_item i").Select("i.item_no,i.name,CASE WHEN r.run_id IS NOT NULL THEN r.state ELSE i.state END AS state,i.project_id,i.run_id,i.error_code").Joins("LEFT JOIN pod_run r ON r.run_id=i.run_id AND r.workspace_id=i.workspace_id").Where("i.batch_id=? AND i.workspace_id=?", id, row.WorkspaceID).Order("i.item_no ASC").Limit(model.PODBatchMaxItems).Scan(&items).Error
	return &row, items, err
}
func SelectPODBatchPage(ctx context.Context, userID int64, pg page.Query) ([]model.PODBatch, int64, error) {
	q := DB(ctx).Model(&model.PODBatch{}).Where(podBatchOwner, userID, userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODBatch{}
	err := q.Order("create_time DESC,batch_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, total, err
}
func StopPODBatch(ctx context.Context, userID, id int64, operator string) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		w, err := lockedPODWorkspaceAccess(tx, userID, false)
		if err != nil {
			return err
		}
		var row model.PODBatch
		if err = tx.Where("batch_id=? AND workspace_id=?", id, w.WorkspaceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODBatch
		} else if err != nil {
			return err
		}
		now := types.Now()
		if row.ExecutionMode == "managed" {
			if err := releaseManagedSnapshot(tx, "batch", id, fmt.Sprintf("batch-stop:%d", id)); err != nil {
				return err
			}
		}
		if err = tx.Model(&model.PODBatchItem{}).Where("batch_id=? AND workspace_id=? AND state='waiting'", id, w.WorkspaceID).Updates(map[string]any{"state": "cancelled", "error_code": "", "update_by": operator, "update_time": now}).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"state": "stopped", "update_by": operator, "update_time": now}).Error
	})
}

// At most one item per tick. All processes serialize on the same gate as ordinary runs.
func DispatchPODBatch(ctx context.Context, limits model.PODExecutionLimits, graphLimits model.PODGraphLimits) (bool, error) {
	var candidate struct {
		BatchID int64
		ItemNo  int
		UserID  int64
	}
	err := DB(ctx).Table("pod_batch_item i").Select("i.batch_id,i.item_no,b.user_id").Joins("JOIN pod_batch b ON b.batch_id=i.batch_id AND b.state='active' AND b.execution_mode=?", limits.Mode).Joins("JOIN sys_user u ON u.user_id=b.user_id AND u.status='0' AND u.del_flag='0'").Where("i.state='waiting'").Order("i.checked_at ASC,i.batch_id ASC,i.item_no ASC").Take(&candidate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	dispatched := false
	err = DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if limits.Mode == "managed" {
			if _, e := lockManagedPlatform(tx); e != nil {
				return e
			}
		}
		w, err := lockedPODWorkspace(tx, candidate.UserID)
		if errors.Is(err, ErrPODWorkspace) || creatorDenied(err) {
			return tx.Model(&model.PODBatchItem{}).Where("batch_id=? AND item_no=? AND state='waiting'", candidate.BatchID, candidate.ItemNo).Update("checked_at", types.Now()).Error
		}
		if err != nil {
			return err
		}
		var batch model.PODBatch
		if err = tx.Where("batch_id=? AND workspace_id=?", candidate.BatchID, w.WorkspaceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&batch).Error; err != nil {
			return err
		}
		var item model.PODBatchItem
		if err = tx.Where("batch_id=? AND item_no=?", candidate.BatchID, candidate.ItemNo).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&item).Error; err != nil {
			return err
		}
		if item.State != "waiting" || batch.State != "active" || batch.ExecutionMode != limits.Mode {
			return nil
		}
		var user model.SysUser
		if err = tx.Select("user_id,user_name").Where("user_id=? AND status='0' AND del_flag='0'", batch.UserID).Take(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		now := types.Now()
		update := map[string]any{"checked_at": now, "update_time": now, "update_by": "pod-worker"}
		if err = checkPODPending(tx, w.WorkspaceID, limits, true); errors.Is(err, ErrPODQueueFull) {
			update["error_code"] = "queueFull"
			return tx.Model(&item).Updates(update).Error
		} else if err != nil {
			return err
		}
		if err = tx.SavePoint("batch_dispatch").Error; err != nil {
			return err
		}
		var graph model.PODGraph
		if err = json.Unmarshal(item.GraphJSON, &graph); err != nil {
			return err
		}
		project := model.PODProject{WorkspaceID: w.WorkspaceID, Name: item.Name, GraphJSON: item.GraphJSON, SchemaVersion: graph.SchemaVersion, Revision: 1, DelFlag: "0", CreateBy: user.UserName, UpdateBy: user.UserName, CreateTime: now, UpdateTime: now}
		if err = tx.Table("pod_project").Create(&project).Error; err != nil {
			return err
		}
		if err = syncPODProjectAssetRefs(tx, w.WorkspaceID, project.ProjectID, batchGraphRefs(graph), user.UserName); err != nil {
			return err
		}
		body := model.PODRunCreate{BatchID: batch.BatchID, BatchItemNo: item.ItemNo, ExecutionMode: limits.Mode, ProjectID: strconv.FormatInt(project.ProjectID, 10), Revision: "1", RequestID: fmt.Sprintf("pod-batch-%d-item-%d", batch.BatchID, item.ItemNo), Mode: "workflow"}
		if limits.Mode == "simulation" {
			body.TestScenario = "success"
		}
		raw, _ := json.Marshal(body)
		run, err := createPODRunTx(tx, &user, body, project.ProjectID, 1, 0, fmt.Sprintf("%x", sha256.Sum256(raw)), limits, graphLimits, true)
		if err != nil {
			if !errors.Is(err, ErrPODRunInput) && !errors.Is(err, ErrPODCredit) && errs.As(err) == nil {
				return err
			}
			if rollbackErr := tx.RollbackTo("batch_dispatch").Error; rollbackErr != nil {
				return rollbackErr
			}
			update["state"] = "dispatchFailed"
			update["error_code"] = "configurationChanged"
			if batch.ExecutionMode == "managed" {
				if e := releaseManagedBatchItem(tx, &batch, item.ItemNo); e != nil {
					return e
				}
			}
		} else {
			update["state"] = "dispatched"
			update["project_id"] = project.ProjectID
			update["run_id"] = run.RunID
			update["error_code"] = ""
			dispatched = true
		}
		if err = tx.Model(&item).Updates(update).Error; err != nil {
			return err
		}
		var remaining int64
		if err = tx.Model(&model.PODBatchItem{}).Where("batch_id=? AND state='waiting'", batch.BatchID).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			return tx.Model(&batch).Updates(map[string]any{"state": "dispatched", "update_time": now, "update_by": "pod-worker"}).Error
		}
		return nil
	})
	return dispatched, err
}

func batchGraphRefs(g model.PODGraph) []model.PODProjectAssetRef {
	refs := []model.PODProjectAssetRef{}
	for _, n := range g.Nodes {
		if n.Type == "imageInput" {
			var c struct {
				AssetIDs []string `json:"assetIds"`
			}
			_ = json.Unmarshal(n.Config, &c)
			for _, s := range c.AssetIDs {
				id, _ := strconv.ParseInt(s, 10, 64)
				refs = append(refs, model.PODProjectAssetRef{NodeID: n.ID, AssetID: id})
			}
		}
	}
	return refs
}

// Output metadata is read once for the authorized batch, never from client paths.
func SelectPODBatchOutputs(ctx context.Context, userID, id int64) ([]model.PODNodeRun, error) {
	row, _, err := SelectPODBatch(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	out := []model.PODNodeRun{}
	err = DB(ctx).Table("pod_node_run n").Select("n.*").Joins("JOIN pod_batch_item i ON i.run_id=n.run_id AND i.workspace_id=n.workspace_id").Where("i.batch_id=? AND i.workspace_id=? AND n.state='succeeded'", id, row.WorkspaceID).Order("i.item_no,n.node_id").Find(&out).Error
	return out, err
}
