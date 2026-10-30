package repository

import (
	"context"
	"errors"
	mysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	portalsql "ruoyi-go/sql/portal"
	"strings"
)

func cmsLocked(ctx context.Context, action func(*gorm.DB) error) error {
	return cmsDB(ctx).Transaction(func(tx *gorm.DB) error {
		var marker struct{ Version int }
		if err := tx.Table("portal_schema_version").Clauses(clause.Locking{Strength: "UPDATE"}).Select("version").Where("version=?", portalsql.CurrentVersion).Take(&marker).Error; err != nil {
			return err
		}
		return action(tx)
	})
}
func cmsDuplicate(err error) error {
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return errs.New("同类型栏目地址已存在")
	}
	return err
}
func channelView(db *gorm.DB) *gorm.DB {
	return db.Table("cms_channel AS c").Select("c.*,COALESCE(p.name,'顶级栏目') AS parent_name,COALESCE(a.name,'') AS cover_name").Joins("LEFT JOIN cms_channel AS p ON p.channel_id=c.parent_id").Joins("LEFT JOIN cms_asset AS a ON a.asset_id=c.cover_asset_id AND a.is_deleted=0")
}
func CMSChannelPage(ctx context.Context, q model.CMSChannelQuery, pg page.Query) ([]model.CmsChannel, int64, error) {
	db := cmsDB(ctx).Table("cms_channel AS c")
	if q.ParentId != nil {
		db = db.Where("c.parent_id=?", *q.ParentId)
	}
	if q.Kind != "" {
		db = db.Where("c.kind=?", q.Kind)
	}
	if q.Name != "" {
		db = db.Where("c.name LIKE ?", strings.TrimSpace(q.Name)+"%")
	}
	if q.Status != "" {
		db = db.Where("c.status=?", q.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CmsChannel{}
	err := channelView(db).Order("c.order_num ASC,c.channel_id ASC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func CMSChannelByID(ctx context.Context, id int64) (*model.CmsChannel, error) {
	var row model.CmsChannel
	err := channelView(cmsDB(ctx)).Where("c.channel_id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.New("栏目不存在")
	}
	return &row, err
}

func CMSSaveChannel(ctx context.Context, in model.CMSChannelInput, operator string, validate func([]model.CmsChannel, model.CMSChannelInput) error) (int64, error) {
	var id int64
	err := cmsLocked(ctx, func(tx *gorm.DB) error {
		rows := []model.CmsChannel{}
		if err := tx.Order("channel_id").Limit(2001).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 2000 || (in.ChannelId == 0 && len(rows) >= 2000) {
			return errs.New("基础版最多支持2000个栏目")
		}
		if in.ChannelId > 0 {
			found := false
			for _, r := range rows {
				if r.ChannelId == in.ChannelId {
					found = true
					if r.Kind != in.Kind {
						var used int64
						if err := tx.Model(&model.CMSRevision{}).Where("channel_id=?", r.ChannelId).Count(&used).Error; err != nil {
							return err
						}
						if used > 0 {
							return errs.New("栏目已被内容历史版本引用，不能修改类型")
						}
					}
					if r.Version != in.Version {
						return errs.New("栏目已变化，请刷新后重试")
					}
				}
			}
			if !found {
				return errs.New("栏目不存在")
			}
		}
		if err := validate(rows, in); err != nil {
			return err
		}
		var cover *int64
		if in.CoverAssetId > 0 {
			var asset model.CmsAsset
			if err := tx.Where("asset_id=? AND is_deleted=0", in.CoverAssetId).Take(&asset).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.New("封面素材不存在或已删除")
			} else if err != nil {
				return err
			}
			cover = &in.CoverAssetId
		}
		now := types.Now()
		if in.ChannelId == 0 {
			row := model.CmsChannel{ParentId: in.ParentId, Kind: in.Kind, Name: in.Name, Slug: in.Slug, OrderNum: in.OrderNum, Status: in.Status, CoverAssetId: cover, SeoTitle: in.SeoTitle, SeoDescription: in.SeoDescription, Remark: in.Remark, Version: 1, CreateBy: operator, CreateTime: now}
			if err := tx.Create(&row).Error; err != nil {
				return cmsDuplicate(err)
			}
			id = row.ChannelId
		} else {
			result := tx.Model(&model.CmsChannel{}).Where("channel_id=? AND version=?", in.ChannelId, in.Version).Updates(map[string]any{"parent_id": in.ParentId, "kind": in.Kind, "name": in.Name, "slug": in.Slug, "order_num": in.OrderNum, "status": in.Status, "cover_asset_id": cover, "seo_title": in.SeoTitle, "seo_description": in.SeoDescription, "remark": in.Remark, "version": in.Version + 1, "update_by": operator, "update_time": now})
			if result.Error != nil {
				return cmsDuplicate(result.Error)
			}
			if result.RowsAffected != 1 {
				return errs.New("栏目已变化，请刷新后重试")
			}
			id = in.ChannelId
		}
		if err := tx.Table("cms_asset_ref").Where("owner_type='CHANNEL' AND owner_id=?", id).Delete(nil).Error; err != nil {
			return err
		}
		if cover != nil {
			return tx.Table("cms_asset_ref").Create(map[string]any{"asset_id": *cover, "owner_type": "CHANNEL", "owner_id": id, "create_by": operator, "create_time": now, "update_by": "", "remark": "栏目封面"}).Error
		}
		return nil
	})
	return id, err
}
func CMSDeleteChannel(ctx context.Context, id, version int64) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var row model.CmsChannel
		if err := tx.Where("channel_id=?", id).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.New("栏目不存在")
		} else if err != nil {
			return err
		}
		if row.Version != version {
			return errs.New("栏目已变化，请刷新后重试")
		}
		var children int64
		if err := tx.Model(&model.CmsChannel{}).Where("parent_id=?", id).Count(&children).Error; err != nil {
			return err
		}
		var used int64
		if err := tx.Model(&model.CMSRevision{}).Where("channel_id=?", id).Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return errs.New("栏目被内容历史版本引用，不能删除；可停用")
		}
		if children > 0 {
			return errs.New("存在子栏目，请先删除子栏目")
		}
		if err := tx.Table("cms_asset_ref").Where("owner_type='CHANNEL' AND owner_id=?", id).Delete(nil).Error; err != nil {
			return err
		}
		return tx.Where("channel_id=?", id).Delete(&model.CmsChannel{}).Error
	})
}

func CMSAssetByID(ctx context.Context, id int64) (*model.CmsAsset, error) {
	var row model.CmsAsset
	err := cmsDB(ctx).Where("asset_id=? AND is_deleted=0", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.New("素材不存在或已删除")
	}
	if err != nil {
		return nil, err
	}
	if err = cmsDB(ctx).Table("cms_asset_ref").Where("asset_id=?", id).Count(&row.ReferenceCount).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
func CMSAssetPage(ctx context.Context, q model.CMSAssetQuery, pg page.Query) ([]model.CmsAsset, int64, error) {
	db := cmsDB(ctx).Model(&model.CmsAsset{}).Where("is_deleted=0")
	if q.Name != "" {
		db = db.Where("name LIKE ?", strings.TrimSpace(q.Name)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CmsAsset{}
	if err := db.Order("asset_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.AssetId)
	}
	if len(ids) > 0 {
		var counts []struct {
			AssetId int64
			Total   int64
		}
		if err := cmsDB(ctx).Table("cms_asset_ref").Select("asset_id,COUNT(*) AS total").Where("asset_id IN ?", ids).Group("asset_id").Scan(&counts).Error; err != nil {
			return nil, 0, err
		}
		byID := map[int64]int64{}
		for _, c := range counts {
			byID[c.AssetId] = c.Total
		}
		for i := range rows {
			rows[i].ReferenceCount = byID[rows[i].AssetId]
		}
	}
	return rows, total, nil
}
func CMSInsertAsset(ctx context.Context, row *model.CmsAsset) error {
	return cmsDB(ctx).Create(row).Error
}
func CMSUpdateAsset(ctx context.Context, id int64, in model.CMSAssetInput, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&model.CmsAsset{}).Where("asset_id=? AND version=? AND is_deleted=0", id, in.Version).Updates(map[string]any{"name": in.Name, "alt_text": in.AltText, "remark": in.Remark, "version": in.Version + 1, "update_by": operator, "update_time": types.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errs.New("素材已变化或已删除，请刷新后重试")
		}
		return nil
	})
}
func CMSDeleteAsset(ctx context.Context, id, version int64, operator string) (string, error) {
	var key string
	err := cmsLocked(ctx, func(tx *gorm.DB) error {
		var row model.CmsAsset
		if err := tx.Where("asset_id=?", id).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.New("素材不存在")
		} else if err != nil {
			return err
		}
		if row.IsDeleted != 0 {
			key = row.StorageKey
			return nil
		}
		if row.Version != version {
			return errs.New("素材已变化，请刷新后重试")
		}
		var count int64
		if err := tx.Table("cms_asset_ref").Where("asset_id=?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errs.New("素材仍被引用，请先移除引用")
		}
		result := tx.Model(&model.CmsAsset{}).Where("asset_id=? AND version=?", id, version).Updates(map[string]any{"is_deleted": 1, "version": version + 1, "update_by": operator, "update_time": types.Now()})
		key = row.StorageKey
		return result.Error
	})
	return key, err
}
func CMSAssetReferences(ctx context.Context, id int64, pg page.Query) ([]model.CMSAssetReference, int64, error) {
	var total int64
	query := cmsDB(ctx).Table("cms_asset_ref AS r").Where("r.asset_id=?", id)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSAssetReference{}
	err := query.Select("r.ref_id,r.owner_type,r.owner_id,COALESCE(c.name,CONCAT('内容历史修订 #',r.owner_id),'其他引用') AS owner_name").Joins("LEFT JOIN cms_channel c ON r.owner_type='CHANNEL' AND c.channel_id=r.owner_id").Order("r.ref_id ASC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
