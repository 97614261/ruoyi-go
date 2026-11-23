package repository

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

var ErrPODProductFinalChanged = errors.New("product final changed")
var ErrPODProductResult = errors.New("product result is unavailable")

func productResultQuery(tx *gorm.DB, workspaceID, projectID int64, nodeID string) *gorm.DB {
	return tx.Table("pod_run r").
		Joins("JOIN pod_node_run n ON n.run_id=r.run_id AND n.workspace_id=r.workspace_id AND n.node_id=? AND n.node_type='imageGenerate'", nodeID).
		Where("r.workspace_id=? AND r.project_id=? AND r.mode IN ?", workspaceID, projectID, []string{"workflow", "branch", "node"})
}

const podProductArchivedImages = "n.state='succeeded' AND JSON_LENGTH(n.output_json, '$.assetIds')>0"

func productJobAccess(tx *gorm.DB, workspaceID, projectID int64, nodeID string) error {
	var job model.PODProductJob
	err := tx.Table("pod_product_job j").
		Joins("JOIN pod_project p ON p.project_id=j.project_id AND p.workspace_id=j.workspace_id AND p.del_flag='0'").
		Select("j.project_id,j.output_mask").Where("j.workspace_id=? AND j.project_id=?", workspaceID, projectID).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPODProject
	}
	if err == nil && job.OutputMask&model.PODProductOutputMask(nodeID) == 0 {
		return ErrPODProject
	}
	return err
}

func SelectPODProductHistory(ctx context.Context, workspaceID, projectID int64, nodeID string, pg page.Query) ([]model.PODProductRun, int64, error) {
	rows := []model.PODProductRun{}
	var total int64
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := productJobAccess(tx, workspaceID, projectID, nodeID); err != nil {
			return err
		}
		query := productResultQuery(tx, workspaceID, projectID, nodeID).Where(podProductArchivedImages)
		if err := query.Count(&total).Error; err != nil {
			return err
		}
		return scanPODProductRuns(query.Select(podProductRunColumns).Order("r.run_id DESC").Offset(pg.Offset()).Limit(pg.PageSize), &rows)
	})
	return rows, total, err
}

func selectPODProductFinal(tx *gorm.DB, workspaceID, projectID int64, nodeID string) (*model.PODProductFinalView, error) {
	var result model.PODProductFinalView
	err := tx.Table("pod_product_final f").
		Joins("JOIN pod_run r ON r.run_id=f.run_id AND r.workspace_id=f.workspace_id AND r.project_id=f.project_id").
		Where("f.workspace_id=? AND f.project_id=? AND f.node_id=?", workspaceID, projectID, nodeID).
		Select("f.run_id,f.asset_id,r.project_revision,r.execution_mode='simulation' AS test_only").Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &result, err
}

func SetPODProductFinal(ctx context.Context, userID int64, next, expected model.PODProductFinal) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		workspace, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		next.WorkspaceID = workspace.WorkspaceID
		if err := productJobAccess(tx, next.WorkspaceID, next.ProjectID, next.NodeID); err != nil {
			return err
		}
		var existing model.PODProductFinal
		err = tx.Where("workspace_id=? AND project_id=? AND node_id=?", next.WorkspaceID, next.ProjectID, next.NodeID).Take(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if existing.RunID == next.RunID && existing.AssetID == next.AssetID {
			return nil
		}
		if existing.RunID != expected.RunID || existing.AssetID != expected.AssetID {
			return ErrPODProductFinalChanged
		}
		var candidates []model.PODProductRun
		query := productResultQuery(tx, next.WorkspaceID, next.ProjectID, next.NodeID).
			Where(podProductArchivedImages).Where("r.run_id=?", next.RunID).Select(podProductRunColumns)
		if err = scanPODProductRuns(query, &candidates); err != nil {
			return err
		}
		if len(candidates) != 1 || !slices.Contains(candidates[0].Output.AssetIDs, strconv.FormatInt(next.AssetID, 10)) {
			return ErrPODProductResult
		}
		var valid int64
		if err := tx.Table("pod_asset a").Joins("JOIN pod_asset_file f ON f.asset_id=a.asset_id AND f.variant='original'").
			Where("a.asset_id=? AND a.workspace_id=? AND a.del_flag='0'", next.AssetID, next.WorkspaceID).Count(&valid).Error; err != nil {
			return err
		}
		if valid != 1 {
			return ErrPODProductResult
		}
		if existing.RunID == 0 {
			return tx.Create(&next).Error
		}
		return tx.Model(&existing).Where("workspace_id=?", next.WorkspaceID).Updates(map[string]any{"run_id": next.RunID, "asset_id": next.AssetID}).Error
	})
}
