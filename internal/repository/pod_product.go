package repository

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

var ErrPODProductBusy = errors.New("POD product output already in progress")

var podProductFinishedNodes = []string{"succeeded", "failed", "blocked", "cancelled"}

func ListPODProductPacks(ctx context.Context, workspaceID int64, pg page.Query) ([]model.PODProductPack, int64, error) {
	query := DB(ctx).Model(&model.PODProductPack{}).Where("workspace_id=?", workspaceID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	packs := []model.PODProductPack{}
	err := query.Order("pack_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&packs).Error
	return packs, total, err
}

func ListPODProductJobs(ctx context.Context, workspaceID int64, pg page.Query) ([]model.PODProductRecent, int64, error) {
	query := DB(ctx).Table("pod_product_job j").Joins("JOIN pod_project p ON p.project_id=j.project_id AND p.workspace_id=j.workspace_id").
		Where("j.workspace_id=? AND p.del_flag='0'", workspaceID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODProductRecent{}
	err := query.Select("j.project_id,p.name,j.work_kind").Order("j.project_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}

func FindPODProductJobRequest(ctx context.Context, workspaceID int64, key, hash string) (*model.PODProductJob, error) {
	return findPODProductJobRequest(DB(ctx), workspaceID, key, hash)
}

func findPODProductJobRequest(db *gorm.DB, workspaceID int64, key, hash string) (*model.PODProductJob, error) {
	var job model.PODProductJob
	err := db.Where("workspace_id=? AND request_id=?", workspaceID, key).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err == nil && job.RequestHash != hash {
		return nil, ErrPODRequest
	}
	return &job, err
}

// Project, material pack, asset reference and submission marker commit together.
func CreatePODProductJob(ctx context.Context, user *model.SysUser, job *model.PODProductJob, pack *model.PODProductPack, project *model.PODProject) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, user.UserID)
		if err != nil {
			return err
		}
		if w.WorkspaceID != pack.WorkspaceID {
			return ErrPODWorkspace
		}
		existing, err := findPODProductJobRequest(tx, w.WorkspaceID, job.RequestID, job.RequestHash)
		if err != nil {
			return err
		}
		if existing != nil {
			*job = *existing
			return nil
		}
		project.WorkspaceID = w.WorkspaceID
		if err = tx.Table("pod_project").Create(project).Error; err != nil {
			return err
		}
		refs := []model.PODProjectAssetRef{{NodeID: model.PODProductReference, AssetID: pack.ReferenceAssetID}}
		if err = syncPODProjectAssetRefs(tx, w.WorkspaceID, project.ProjectID, refs, user.UserName); err != nil {
			return err
		}
		var saved model.PODProductPack
		err = tx.Where("workspace_id=? AND fingerprint=?", w.WorkspaceID, pack.Fingerprint).Take(&saved).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Create(pack).Error
		} else if err == nil {
			pack.PackID = saved.PackID
		}
		if err != nil {
			return err
		}
		job.WorkspaceID, job.ProjectID, job.PackID = w.WorkspaceID, project.ProjectID, pack.PackID
		if job.WorkKind == "" {
			job.WorkKind, job.OutputMask = model.PODProductImages, model.PODProductDefaultOutputs
		}
		return tx.Create(job).Error
	})
}

func SelectPODProductJob(ctx context.Context, workspaceID, projectID int64) (model.PODProject, model.PODProductPack, []model.PODProductSlot, string, error) {
	var project model.PODProject
	var pack model.PODProductPack
	var job model.PODProductJob
	slots := []model.PODProductSlot{}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("pod_project").Where("workspace_id=? AND project_id=? AND del_flag='0'", workspaceID, projectID).Take(&project).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id=? AND project_id=?", workspaceID, projectID).Take(&job).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id=? AND pack_id=?", workspaceID, job.PackID).Take(&pack).Error; err != nil {
			return err
		}
		for _, output := range model.PODProductOutputs(job.OutputMask) {
			nodeID := output.ID
			slot, err := podProductSlot(tx, workspaceID, projectID, nodeID)
			if err != nil {
				return err
			}
			slot.Final, err = selectPODProductFinal(tx, workspaceID, projectID, nodeID)
			if err != nil {
				return err
			}
			slots = append(slots, slot)
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrPODProject
	}
	return project, pack, slots, job.WorkKind, err
}

const podProductRunColumns = "r.run_id,r.project_revision,r.state AS run_state,r.execution_mode='simulation' AS test_only,n.state,n.error_code,n.error_message,n.output_json"

func scanPODProductRuns(query *gorm.DB, runs *[]model.PODProductRun) error {
	var rows []struct {
		model.PODProductRun
		OutputJSON json.RawMessage
	}
	if err := query.Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := json.Unmarshal(row.OutputJSON, &row.Output); err != nil {
			return err
		}
		*runs = append(*runs, row.PODProductRun)
	}
	return nil
}

func podProductSlot(tx *gorm.DB, workspaceID, projectID int64, nodeID string) (model.PODProductSlot, error) {
	slot := model.PODProductSlot{NodeID: nodeID}
	query := func() *gorm.DB {
		return productResultQuery(tx, workspaceID, projectID, nodeID).
			Select(podProductRunColumns).
			Order("r.run_id DESC").Limit(1)
	}
	read := func(db *gorm.DB) (*model.PODProductRun, error) {
		var rows []model.PODProductRun
		if err := scanPODProductRuns(db, &rows); err != nil || len(rows) == 0 {
			return nil, err
		}
		return &rows[0], nil
	}
	latest, err := read(query())
	if err != nil {
		return slot, err
	}
	slot.Latest = latest
	if latest == nil {
		return slot, nil
	}
	// An older in-flight or uncertain run still owns this output.
	if active, err := read(query().Where("r.state IN ? AND n.state NOT IN ?", podActiveRunStates, podProductFinishedNodes)); err != nil {
		return slot, err
	} else if active != nil {
		slot.Latest = active
	}
	if latest.State == "succeeded" && len(latest.Output.AssetIDs) > 0 {
		slot.Result = latest
		return slot, nil
	}
	slot.Result, err = read(query().Where(podProductArchivedImages))
	return slot, err
}

// Submission and recovery hold the execution gate and workspace lock.
func checkPODProductRunOverlap(tx *gorm.DB, workspaceID, projectID int64, plan []model.PODNode) error {
	var outputs []string
	for _, node := range plan {
		if node.Type == "imageGenerate" && model.PODProductOutputMask(node.ID) != 0 {
			outputs = append(outputs, node.ID)
		}
	}
	if len(outputs) == 0 {
		return nil
	}
	var count int64
	err := tx.Table("pod_product_job j").
		Joins("JOIN pod_run r ON r.project_id=j.project_id AND r.workspace_id=j.workspace_id").
		Joins("JOIN pod_node_run n ON n.run_id=r.run_id AND n.workspace_id=r.workspace_id").
		Where("j.workspace_id=? AND j.project_id=? AND n.node_id IN ? AND n.node_type='imageGenerate'", workspaceID, projectID, outputs).
		Where("r.state IN ? AND n.state NOT IN ?", podActiveRunStates, podProductFinishedNodes).
		Count(&count).Error
	if err == nil && count > 0 {
		return ErrPODProductBusy
	}
	return err
}
