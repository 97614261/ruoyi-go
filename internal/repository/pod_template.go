package repository

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var ErrPODTemplate = errors.New("POD template not found")
var ErrPODTemplateRequest = errors.New("POD template request conflict")

func podTemplateQuery(tx *gorm.DB, userID int64) *gorm.DB {
	return tx.Table("pod_template t").
		Joins("JOIN pod_workspace w ON w.workspace_id=t.workspace_id AND w.owner_user_id=? AND w.status='0'", userID).
		Joins("JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0'", userID)
}

func SelectPODTemplatePage(ctx context.Context, userID int64, deleted bool, pg page.Query) ([]model.PODTemplate, int64, error) {
	state := "0"
	if deleted {
		state = "2"
	}
	q := podTemplateQuery(DB(ctx), userID).Where("t.del_flag=?", state)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODTemplate{}
	err := q.Select("t.*").Order("t.update_time DESC,t.template_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}

func SelectPODTemplate(ctx context.Context, userID, id int64) (*model.PODTemplate, error) {
	var row model.PODTemplate
	err := podTemplateQuery(DB(ctx), userID).Select("t.*").Where("t.template_id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrPODTemplate
	}
	return &row, err
}

func SelectPODTemplateVersions(ctx context.Context, userID, id int64, pg page.Query) ([]model.PODTemplateVersion, int64, error) {
	q := podTemplateQuery(DB(ctx), userID).Joins("JOIN pod_template_version v ON v.template_id=t.template_id AND v.workspace_id=t.workspace_id").Where("t.template_id=?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODTemplateVersion{}
	err := q.Select("v.template_id,v.version,v.create_time").Order("v.version DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}

func SelectPODTemplateVersion(ctx context.Context, userID, id, version int64) (*model.PODTemplateVersion, error) {
	var row model.PODTemplateVersion
	err := podTemplateQuery(DB(ctx), userID).Joins("JOIN pod_template_version v ON v.template_id=t.template_id AND v.workspace_id=t.workspace_id").
		Select("v.*").Where("t.template_id=? AND v.version=?", id, version).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrPODTemplate
	}
	return &row, err
}

func templateAction(tx *gorm.DB, workspaceID int64, request, action, hash string) (*model.PODTemplateAction, error) {
	var row model.PODTemplateAction
	err := tx.Table("pod_template_action").Where("workspace_id=? AND request_id=?", workspaceID, request).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Action != action || row.RequestHash != hash {
		return nil, ErrPODTemplateRequest
	}
	return &row, nil
}

// Replays can be found before re-reading a source that may have since changed.
func FindPODTemplateAction(ctx context.Context, userID int64, request, action, hash string) (*model.PODTemplateAction, error) {
	w, err := PODPersonalWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrPODWorkspace
	}
	return templateAction(DB(ctx), w.WorkspaceID, request, action, hash)
}

type PODTemplateWrite struct {
	TargetID        int64
	Revision        int64
	ProjectID       int64
	ProjectRevision int64
	SourceID        int64
	SourceVersion   int64
	Template        model.PODTemplate
	Version         model.PODTemplateVersion
	Refs            []model.PODProjectAssetRef
	Action          model.PODTemplateAction
}

func validateTemplateRefs(tx *gorm.DB, workspaceID int64, refs []model.PODProjectAssetRef) error {
	ids := []int64{}
	seen := map[int64]bool{}
	for _, ref := range refs {
		if !seen[ref.AssetID] {
			seen[ref.AssetID] = true
			ids = append(ids, ref.AssetID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var count int64
	if err := tx.Table("pod_asset").Where("workspace_id=? AND del_flag='0' AND asset_id IN ?", workspaceID, ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return ErrPODAssetGraph
	}
	return nil
}

func WritePODTemplate(ctx context.Context, userID int64, in PODTemplateWrite) (*model.PODTemplateAction, error) {
	var result *model.PODTemplateAction
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		old, err := templateAction(tx, w.WorkspaceID, in.Action.RequestID, in.Action.Action, in.Action.RequestHash)
		if err != nil {
			return err
		}
		if old != nil {
			result = old
			return nil
		}
		if in.ProjectID > 0 {
			var p model.PODProject
			err = tx.Table("pod_project").Where("workspace_id=? AND project_id=? AND del_flag='0'", w.WorkspaceID, in.ProjectID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&p).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPODProject
			}
			if err != nil {
				return err
			}
			if p.Revision != in.ProjectRevision {
				return ErrPODRevision
			}
		} else {
			var count int64
			if err = tx.Table("pod_template t").Joins("JOIN pod_template_version v ON v.template_id=t.template_id AND v.workspace_id=t.workspace_id").
				Where("t.workspace_id=? AND t.template_id=? AND t.del_flag='0' AND v.version=?", w.WorkspaceID, in.SourceID, in.SourceVersion).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrPODTemplate
			}
		}
		if err = validateTemplateRefs(tx, w.WorkspaceID, in.Refs); err != nil {
			return err
		}
		row := in.Template
		row.WorkspaceID = w.WorkspaceID
		if in.TargetID == 0 {
			row.Revision = 1
			row.CurrentVersion = 1
			row.DelFlag = "0"
			if err = tx.Table("pod_template").Create(&row).Error; err != nil {
				return err
			}
		} else {
			var current model.PODTemplate
			err = tx.Table("pod_template").Where("workspace_id=? AND template_id=? AND del_flag='0'", w.WorkspaceID, in.TargetID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPODTemplate
			}
			if err != nil {
				return err
			}
			if current.Revision != in.Revision || current.Revision == math.MaxInt64 || current.CurrentVersion == math.MaxInt64 {
				return ErrPODRevision
			}
			current.Revision++
			current.CurrentVersion++
			if err = tx.Table("pod_template").Where("template_id=? AND workspace_id=?", in.TargetID, w.WorkspaceID).
				Updates(map[string]any{"name": row.Name, "description": row.Description, "revision": current.Revision, "current_version": current.CurrentVersion, "update_by": row.UpdateBy, "update_time": row.UpdateTime}).Error; err != nil {
				return err
			}
			row = current
		}
		version := in.Version
		version.TemplateID = row.TemplateID
		version.WorkspaceID = w.WorkspaceID
		version.Version = row.CurrentVersion
		if err = tx.Table("pod_template_version").Create(&version).Error; err != nil {
			return err
		}
		refs := make([]model.PODTemplateAssetRef, 0, len(in.Refs))
		for _, ref := range in.Refs {
			refs = append(refs, model.PODTemplateAssetRef{TemplateID: row.TemplateID, Version: version.Version, WorkspaceID: w.WorkspaceID, NodeID: ref.NodeID, AssetID: ref.AssetID, CreateBy: version.CreateBy, UpdateBy: version.UpdateBy, CreateTime: version.CreateTime, UpdateTime: version.UpdateTime})
		}
		if len(refs) > 0 {
			if err = tx.Table("pod_template_asset_ref").CreateInBatches(&refs, 500).Error; err != nil {
				return err
			}
		}
		action := in.Action
		action.WorkspaceID = w.WorkspaceID
		action.TemplateID = row.TemplateID
		action.Version = version.Version
		if err = tx.Table("pod_template_action").Create(&action).Error; err != nil {
			return err
		}
		result = &action
		return nil
	})
	return result, err
}

func UpdatePODTemplate(ctx context.Context, userID, id, revision int64, from string, updates map[string]any) (*model.PODTemplate, error) {
	var row model.PODTemplate
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		err = tx.Table("pod_template").Where("template_id=? AND workspace_id=? AND del_flag=?", id, w.WorkspaceID, from).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODTemplate
		}
		if err != nil {
			return err
		}
		if row.Revision != revision || revision == math.MaxInt64 {
			return ErrPODRevision
		}
		updates["revision"] = revision + 1
		updates["update_time"] = types.Now()
		if err = tx.Table("pod_template").Where("template_id=? AND workspace_id=?", id, w.WorkspaceID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Table("pod_template").Where("template_id=? AND workspace_id=?", id, w.WorkspaceID).Take(&row).Error
	})
	return &row, err
}

func UsePODTemplate(ctx context.Context, userID int64, project *model.PODProject, refs []model.PODProjectAssetRef, action model.PODTemplateAction) (*model.PODTemplateAction, error) {
	var result *model.PODTemplateAction
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		old, err := templateAction(tx, w.WorkspaceID, action.RequestID, action.Action, action.RequestHash)
		if err != nil {
			return err
		}
		if old != nil {
			result = old
			return nil
		}
		var count int64
		if err = tx.Table("pod_template t").Joins("JOIN pod_template_version v ON v.template_id=t.template_id AND v.workspace_id=t.workspace_id").
			Where("t.workspace_id=? AND t.template_id=? AND t.del_flag='0' AND v.version=?", w.WorkspaceID, action.TemplateID, action.Version).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrPODTemplate
		}
		project.WorkspaceID = w.WorkspaceID
		if err = tx.Table("pod_project").Create(project).Error; err != nil {
			return err
		}
		if err = syncPODProjectAssetRefs(tx, w.WorkspaceID, project.ProjectID, refs, project.CreateBy); err != nil {
			return err
		}
		action.WorkspaceID = w.WorkspaceID
		action.ProjectID = project.ProjectID
		if err = tx.Table("pod_template_action").Create(&action).Error; err != nil {
			return err
		}
		result = &action
		return nil
	})
	return result, err
}
