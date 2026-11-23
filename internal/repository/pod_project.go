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

var ErrPODWorkspace = errors.New("POD workspace access denied")
var ErrPODProject = errors.New("POD project not found")
var ErrPODRevision = errors.New("POD project revision conflict")

const podOwnerExists = "EXISTS (SELECT 1 FROM pod_workspace w JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0' WHERE w.workspace_id=pod_project.workspace_id AND w.owner_user_id=? AND w.status='0')"

func lockedPODWorkspace(tx *gorm.DB, userID int64) (*model.PODWorkspace, error) {
	return lockedPODWorkspaceAccess(tx, userID, true)
}
func lockedPODWorkspaceAccess(tx *gorm.DB, userID int64, write bool) (*model.PODWorkspace, error) {
	w, err := lockedPODWorkspaceRaw(tx, userID)
	if err != nil {
		return nil, err
	}
	if _, err = checkCreator(tx, userID, write, true); err != nil {
		return nil, err
	}
	return w, nil
}
func lockedPODWorkspaceRaw(tx *gorm.DB, userID int64) (*model.PODWorkspace, error) {
	var w model.PODWorkspace
	err := tx.Table("pod_workspace w").Select("w.workspace_id,w.name").
		Joins("JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0'", userID).
		Where("w.owner_user_id=? AND w.status='0'", userID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPODWorkspace
	}
	return &w, err
}

// Explicit initialization never reactivates a disabled space or revoked member.
func EnsurePODPersonalWorkspace(ctx context.Context, userID int64, name, operator string) (*model.PODWorkspace, error) {
	var result *model.PODWorkspace
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var identity struct{ UserID int64 }
		if err := tx.Table("sys_user").Select("user_id").Where("user_id=? AND status='0' AND del_flag='0'", userID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&identity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPODWorkspace
			}
			return err
		}
		var count int64
		if err := tx.Table("pod_workspace").Where("owner_user_id=?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			var err error
			result, err = lockedPODWorkspace(tx, userID)
			return err
		}
		row := map[string]any{"owner_user_id": userID, "name": name, "create_by": operator, "update_by": operator}
		if err := tx.Table("pod_workspace").Create(row).Error; err != nil {
			return err
		}
		var workspace model.PODWorkspace
		if err := tx.Table("pod_workspace").Where("owner_user_id=?", userID).Take(&workspace).Error; err != nil {
			return err
		}
		if err := tx.Table("pod_workspace_member").Create(map[string]any{"workspace_id": workspace.WorkspaceID, "user_id": userID, "create_by": operator, "update_by": operator}).Error; err != nil {
			return err
		}
		if err := initCreatorAccounts(tx, userID, workspace.WorkspaceID, "active", operator); err != nil {
			return err
		}
		result = &workspace
		return nil
	})
	return result, err
}

func SelectPODProjectPage(ctx context.Context, userID int64, deleted bool, pg page.Query) ([]model.PODProject, int64, error) {
	state := "0"
	if deleted {
		state = "2"
	}
	query := DB(ctx).Table("pod_project").Where("del_flag=?", state).Where(podOwnerExists, userID, userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	list := []model.PODProject{}
	err := query.Select("project_id,workspace_id,name,revision,del_flag,create_time,update_time").Order("update_time DESC,project_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error
	return list, total, err
}

func SelectPODProjectByID(ctx context.Context, userID, id int64) (*model.PODProject, error) {
	var row model.PODProject
	err := DB(ctx).Table("pod_project").Where("project_id=? AND del_flag='0'", id).Where(podOwnerExists, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPODProject
	}
	return &row, err
}

func InsertPODProject(ctx context.Context, userID int64, row *model.PODProject) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		row.WorkspaceID = w.WorkspaceID
		return tx.Table("pod_project").Create(row).Error
	})
}

// Membership locks and compare-and-swap are in the same transaction as writes.
func UpdatePODProject(ctx context.Context, userID, id, revision int64, fromState string, updates map[string]any, refs *[]model.PODProjectAssetRef) (*model.PODProject, error) {
	var row model.PODProject
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		err = tx.Table("pod_project").Where("project_id=? AND workspace_id=? AND del_flag=?", id, w.WorkspaceID, fromState).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODProject
		}
		if err != nil {
			return err
		}
		if row.Revision != revision || revision == math.MaxInt64 {
			return ErrPODRevision
		}
		if refs != nil {
			if err := syncPODProjectAssetRefs(tx, w.WorkspaceID, id, *refs, updates["update_by"].(string)); err != nil {
				return err
			}
		}
		updates["revision"] = gorm.Expr("revision+1")
		updates["update_time"] = types.Now()
		write := tx.Table("pod_project").Where("project_id=? AND workspace_id=? AND revision=? AND del_flag=?", id, w.WorkspaceID, revision, fromState).Updates(updates)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return ErrPODRevision
		}
		return tx.Table("pod_project").Where("project_id=? AND workspace_id=?", id, w.WorkspaceID).Take(&row).Error
	})
	return &row, err
}
