package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

func CMSAssetsByIDs(ctx context.Context, ids []int64) ([]model.CmsAsset, error) {
	out := []model.CmsAsset{}
	if len(ids) == 0 {
		return out, nil
	}
	err := cmsDB(ctx).Where("asset_id IN ? AND is_deleted=0", ids).Find(&out).Error
	return out, err
}
func CMSInspectionChannels(ctx context.Context) ([]model.CmsChannel, error) {
	rows := []model.CmsChannel{}
	err := cmsDB(ctx).Select("channel_id,parent_id,kind,status,slug").Limit(2001).Find(&rows).Error
	if len(rows) > 2000 {
		return nil, errs.New("栏目超过2000，巡检需扩展分页策略")
	}
	return rows, err
}
func CMSInspectionPage(ctx context.Context, pg page.Query) ([]model.CMSInspectionRow, int64, error) {
	q := cmsDB(ctx).Table("cms_content c").Joins("JOIN cms_content_revision r ON r.content_id=c.content_id AND r.revision_id=c.published_revision_id").Where("c.is_published=1 AND c.is_deleted=0")
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSInspectionRow{}
	err := q.Select("c.kind,c.slug,c.version,r.*").Order("c.content_id").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	if err == nil && len(rows) > 0 {
		ids := []int64{}
		for _, v := range rows {
			ids = append(ids, v.RevisionId)
		}
		var ops []model.CMSRevisionOps
		if err = cmsDB(ctx).Where("revision_id IN ?", ids).Find(&ops).Error; err != nil {
			return nil, 0, err
		}
		by := map[int64]*model.CMSContentOps{}
		for _, o := range ops {
			v := &model.CMSContentOps{}
			if err = json.Unmarshal([]byte(o.Payload), v); err != nil {
				return nil, 0, err
			}
			by[o.RevisionId] = v
		}
		for i := range rows {
			rows[i].Ops = by[rows[i].RevisionId]
		}
	}
	return rows, total, err
}
func CMSInspectionURLs(ctx context.Context) (map[string]bool, error) {
	tx := cmsDB(ctx)
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ContentId  int64
		Kind, Slug string
	}
	if err = cmsPublished(tx, active).Select("c.content_id,c.kind,c.slug").Limit(5001).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 5000 {
		return nil, errs.New("公开内容超过5000，首版巡检需要扩展后再执行，不返回不完整结果")
	}
	out := map[string]bool{"/": true, "/news": true, "/products": true, "/search": true, "/sitemap.xml": true, "/robots.txt": true}
	for _, row := range rows {
		out[portalURL(row.Kind, row.Slug)] = true
		out[fmt.Sprintf("content:%d", row.ContentId)] = true
	}
	return out, nil
}
