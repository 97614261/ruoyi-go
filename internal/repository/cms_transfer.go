package repository

import (
	"context"
	"ruoyi-go/internal/model"
)

func CMSUsedSlugs(ctx context.Context, kind string, slugs []string) (map[string]bool, error) {
	rows := []string{}
	if len(slugs) > 0 {
		if e := cmsDB(ctx).Model(&model.CMSContent{}).Where("kind=? AND slug IN ?", kind, slugs).Pluck("slug", &rows).Error; e != nil {
			return nil, e
		}
	}
	out := map[string]bool{}
	for _, s := range rows {
		out[s] = true
	}
	return out, nil
}
func CMSTransferDrafts(ctx context.Context, kind string, ids []int64) ([]model.CMSInspectionRow, error) {
	out := []model.CMSInspectionRow{}
	e := cmsDB(ctx).Table("cms_content c").Joins("JOIN cms_content_revision r ON r.content_id=c.content_id AND r.revision_id=c.working_revision_id").Select("r.*,c.slug,c.version,c.kind").Where("c.kind=? AND c.content_id IN ? AND c.is_deleted=0", kind, ids).Find(&out).Error
	return out, e
}
