package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"strings"
)

// Only metadata already readable in the corresponding list; never draft bodies or files.
func CMSSiteReferences(ctx context.Context, q model.CMSSiteReferenceQuery, kinds []string, ids []int64, pg page.Query) ([]model.CMSSiteReference, int64, error) {
	d := cmsDB(ctx)
	column, title := "c.content_id", "r.title"
	if q.Type == "channel" {
		d = d.Table("cms_channel c").Where("c.kind IN ?", kinds)
		column, title = "c.channel_id", "c.name"
	} else {
		d = d.Table("cms_content c").Joins("JOIN cms_content_revision r ON r.revision_id=c.working_revision_id AND r.content_id=c.content_id").Where("c.is_deleted=0 AND c.kind IN ?", kinds)
	}
	if len(ids) > 0 {
		d = d.Where(column+" IN ?", ids)
	}
	if q.Name != "" {
		d = d.Where(title+" LIKE ?", escapeLike(strings.TrimSpace(q.Name))+"%")
	}
	var total int64
	if e := d.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	enabled := "c.is_published"
	if q.Type == "channel" {
		enabled = "(c.status='0')"
	}
	rows := []model.CMSSiteReference{}
	if e := d.Select(column + " AS ref_id," + title + " AS title,c.kind,c.slug," + enabled + " AS enabled").Order(column + " DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error; e != nil {
		return nil, 0, e
	}
	for i := range rows {
		v := &rows[i]
		v.URL = portalURL(v.Kind, v.Slug)
		if q.Type == "channel" {
			prefix := "/news/c/"
			if v.Kind == "PRODUCT" {
				prefix = "/products/c/"
			}
			v.URL = prefix + v.Slug
		}
	}
	return rows, total, nil
}
