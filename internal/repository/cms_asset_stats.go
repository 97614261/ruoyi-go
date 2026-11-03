package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"time"
)

func cmsAssetFilter(ctx context.Context, q model.CMSAssetQuery) *gorm.DB {
	d := cmsDB(ctx).Model(&model.CmsAsset{}).Where("is_deleted=0")
	if q.Name != "" {
		d = d.Where("name LIKE ?", escapeLike(q.Name)+"%")
	}
	if q.GroupName != nil {
		d = d.Where("group_name=?", *q.GroupName)
	}
	if q.MimeType != "" {
		d = d.Where("mime_type=?", q.MimeType)
	}
	if q.MinWidth > 0 {
		d = d.Where("width>=?", q.MinWidth)
	}
	if q.MaxWidth > 0 {
		d = d.Where("width<=?", q.MaxWidth)
	}
	if q.MinBytes > 0 {
		d = d.Where("byte_size>=?", q.MinBytes)
	}
	if q.MaxBytes > 0 {
		d = d.Where("byte_size<=?", q.MaxBytes)
	}
	return d
}
func CMSAssetStatistics(ctx context.Context, q model.CMSAssetQuery) (model.CMSAssetStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var out model.CMSAssetStats
	err := cmsAssetFilter(ctx, q).Select("COUNT(*) AS total,COALESCE(SUM(byte_size),0) AS byte_size,COALESCE(MAX(width),0) AS max_width,COALESCE(MAX(height),0) AS max_height").Scan(&out).Error
	return out, err
}
