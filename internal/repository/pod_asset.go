package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strconv"
)

var ErrPODAsset = errors.New("POD asset not found")
var ErrPODAssetReference = errors.New("POD asset is referenced")
var ErrPODAssetGraph = errors.New("POD graph asset unavailable")

const assetOwnerExists = "EXISTS (SELECT 1 FROM pod_workspace w JOIN pod_workspace_member m ON m.workspace_id=w.workspace_id AND m.user_id=? AND m.status='0' WHERE w.workspace_id=pod_asset.workspace_id AND w.owner_user_id=? AND w.status='0')"
const assetSelect = "pod_asset.*, ((SELECT COUNT(*) FROM pod_project_asset_ref r WHERE r.asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)+(SELECT COUNT(*) FROM pod_run_asset_ref r WHERE r.asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)+(SELECT COUNT(*) FROM pod_template_asset_ref r WHERE r.asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)+(SELECT COUNT(*) FROM pod_batch_asset_ref r WHERE r.asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)+(SELECT COUNT(*) FROM pod_product_pack r WHERE r.reference_asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)+(SELECT COUNT(*) FROM pod_product_final r WHERE r.asset_id=pod_asset.asset_id AND r.workspace_id=pod_asset.workspace_id)) AS reference_count"

// Read source classification in one authorized query; choosing it again in imageInput must not erase mockup provenance.
type PODImageInput struct {
	model.PODAssetFile
	Source      string
	AssetRemark string
}

func SelectPODImageInputs(ctx context.Context, userID int64, ids []string) (map[string]PODImageInput, error) {
	var rows []PODImageInput
	err := DB(ctx).Table("pod_asset_file f").Select("f.*,pod_asset.source,pod_asset.remark AS asset_remark").Joins("JOIN pod_asset ON pod_asset.asset_id=f.asset_id").Where("pod_asset.asset_id IN ? AND pod_asset.del_flag='0' AND f.variant='original'", ids).Where(assetOwnerExists, userID, userID).Find(&rows).Error
	result := map[string]PODImageInput{}
	for _, row := range rows {
		result[strconv.FormatInt(row.AssetID, 10)] = row
	}
	return result, err
}

func activePODProductPageProject(tx *gorm.DB, workspaceID, projectID int64) error {
	var job model.PODProductJob
	err := tx.Table("pod_product_job j").Select("j.project_id").
		Joins("JOIN pod_project p ON p.project_id=j.project_id AND p.workspace_id=j.workspace_id").
		Where("j.workspace_id=? AND j.project_id=? AND j.work_kind=? AND p.del_flag='0'", workspaceID, projectID, model.PODProductImages).
		Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPODProject
	}
	return err
}

func productPageAssets(q *gorm.DB, projectID int64) *gorm.DB {
	// For the product-page source, remark records the verified source project.
	return q.Where("pod_asset.source=? AND pod_asset.remark=?", model.PODAssetProductPage, strconv.FormatInt(projectID, 10))
}

func findPODProductPageAsset(tx *gorm.DB, workspaceID, projectID int64, name, digest string) (*model.PODAsset, error) {
	var row model.PODAsset
	q := tx.Table("pod_asset").Select("pod_asset.*").
		Joins("JOIN pod_asset_file f ON f.asset_id=pod_asset.asset_id AND f.variant='original'").
		Where("pod_asset.workspace_id=? AND pod_asset.del_flag='0' AND CAST(pod_asset.original_name AS BINARY)=? AND f.sha256=?", workspaceID, name, digest)
	err := productPageAssets(q, projectID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func FindPODProductPageAsset(ctx context.Context, workspaceID, projectID int64, name, digest string) (*model.PODAsset, error) {
	tx := DB(ctx)
	if err := activePODProductPageProject(tx, workspaceID, projectID); err != nil {
		return nil, err
	}
	return findPODProductPageAsset(tx, workspaceID, projectID, name, digest)
}

func SelectPODAssetPage(ctx context.Context, userID, workspaceID, projectID int64, deleted bool, pg page.Query) ([]model.PODAsset, int64, error) {
	state := "0"
	if deleted {
		state = "2"
	}
	tx := DB(ctx)
	q := tx.Table("pod_asset").Where("workspace_id=? AND del_flag=?", workspaceID, state).Where(assetOwnerExists, userID, userID)
	if projectID != 0 {
		if err := activePODProductPageProject(tx, workspaceID, projectID); err != nil {
			return nil, 0, err
		}
		q = productPageAssets(q, projectID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.PODAsset{}
	err := q.Select(assetSelect).Order("create_time DESC,asset_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func SelectPODAsset(ctx context.Context, userID, id int64) (*model.PODAsset, error) {
	var row model.PODAsset
	err := DB(ctx).Table("pod_asset").Select(assetSelect).Where("asset_id=? AND del_flag='0'", id).Where(assetOwnerExists, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPODAsset
	}
	return &row, err
}
func SelectPODAssetFile(ctx context.Context, userID, id int64, variant string) (*model.PODAssetFile, string, error) {
	var row struct {
		model.PODAssetFile
		OriginalName string
	}
	err := DB(ctx).Table("pod_asset_file f").Select("f.*,pod_asset.original_name").Joins("JOIN pod_asset ON pod_asset.asset_id=f.asset_id").Where("pod_asset.asset_id=? AND pod_asset.del_flag='0' AND f.variant=?", id, variant).Where(assetOwnerExists, userID, userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrPODAsset
	}
	return &row.PODAssetFile, row.OriginalName, err
}
func InsertPODAsset(ctx context.Context, userID, projectID int64, asset *model.PODAsset, files []model.PODAssetFile, reservation string) (*model.PODAsset, error) {
	saved := asset
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		asset.WorkspaceID = w.WorkspaceID
		if projectID != 0 {
			if err := activePODProductPageProject(tx, w.WorkspaceID, projectID); err != nil {
				return err
			}
			existing, err := findPODProductPageAsset(tx, w.WorkspaceID, projectID, asset.OriginalName, files[0].SHA256)
			if err != nil {
				return err
			}
			if existing != nil {
				saved = existing
				return nil
			}
			asset.Source = model.PODAssetProductPage
			asset.Remark = strconv.FormatInt(projectID, 10)
		}
		if err := tx.Table("pod_asset").Create(asset).Error; err != nil {
			return err
		}
		for i := range files {
			files[i].AssetID = asset.AssetID
		}
		if err := tx.Table("pod_asset_file").Create(&files).Error; err != nil {
			return err
		}
		return commitPODStorage(tx, userID, reservation, asset.AssetID, files)
	})
	return saved, err
}
func ChangePODAssetDeletion(ctx context.Context, userID, id, revision int64, restore bool, operator string) (*model.PODAsset, error) {
	from, to := "0", "2"
	if restore {
		from, to = to, from
	}
	var row model.PODAsset
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockedPODWorkspace(tx, userID)
		if err != nil {
			return err
		}
		err = tx.Table("pod_asset").Where("asset_id=? AND workspace_id=? AND del_flag=?", id, w.WorkspaceID, from).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPODAsset
		}
		if err != nil {
			return err
		}
		if row.Revision != revision || revision == math.MaxInt64 {
			return ErrPODRevision
		}
		if !restore {
			used, err := officialCopyUsesAsset(tx, w.WorkspaceID, id)
			if err != nil {
				return err
			}
			if used {
				return ErrPODAssetReference
			}
			var count int64
			if err := tx.Table("pod_batch_asset_ref").Where("asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
			if err := tx.Table("pod_project_asset_ref").Where("asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
			if err := tx.Table("pod_run_asset_ref").Where("asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
			if err := tx.Table("pod_template_asset_ref").Where("asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
			if err := tx.Table("pod_product_pack").Where("reference_asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
			if err := tx.Table("pod_product_final").Where("asset_id=? AND workspace_id=?", id, w.WorkspaceID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrPODAssetReference
			}
		}
		result := tx.Table("pod_asset").Where("asset_id=? AND revision=? AND del_flag=?", id, revision, from).Updates(map[string]any{"revision": revision + 1, "del_flag": to, "update_by": operator, "update_time": types.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrPODRevision
		}
		row.Revision++
		row.DelFlag = to
		return nil
	})
	return &row, err
}

// Caller holds the workspace lock, shared with asset deletion, for the entire project write.
func syncPODProjectAssetRefs(tx *gorm.DB, workspaceID, projectID int64, refs []model.PODProjectAssetRef, operator string) error {
	ids := []int64{}
	seen := map[int64]bool{}
	for _, ref := range refs {
		if !seen[ref.AssetID] {
			seen[ref.AssetID] = true
			ids = append(ids, ref.AssetID)
		}
	}
	if len(ids) > 0 {
		var count int64
		if err := tx.Table("pod_asset").Where("workspace_id=? AND del_flag='0' AND asset_id IN ?", workspaceID, ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return ErrPODAssetGraph
		}
	}
	if err := tx.Table("pod_project_asset_ref").Where("project_id=? AND workspace_id=?", projectID, workspaceID).Delete(&model.PODProjectAssetRef{}).Error; err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	now := types.Now()
	for i := range refs {
		refs[i].ProjectID = projectID
		refs[i].WorkspaceID = workspaceID
		refs[i].CreateBy = operator
		refs[i].UpdateBy = operator
		refs[i].CreateTime = now
		refs[i].UpdateTime = now
	}
	return tx.Table("pod_project_asset_ref").CreateInBatches(&refs, 200).Error
}
