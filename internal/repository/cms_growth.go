package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func CMSTags(ctx context.Context, q model.CMSTagQuery, pg page.Query) ([]model.CMSTag, int64, error) {
	db := cmsDB(ctx).Model(&model.CMSTag{})
	if q.Name != "" {
		db = db.Where("name LIKE ?", escapeLike(q.Name)+"%")
	}
	if q.Status != "" {
		db = db.Where("status=?", q.Status)
	}
	var n int64
	if e := db.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSTag{}
	e := db.Order("tag_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func CMSSaveTag(ctx context.Context, in model.CMSTag, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		if in.TagId == 0 {
			in.Version = 1
			in.CreateBy = operator
			in.CreateTime = types.Now()
			in.UpdateBy = ""
			in.UpdateTime = types.Time{}
			return cmsDuplicate(tx.Create(&in).Error)
		}
		if in.TagId < 1 || in.Version < 1 {
			return errs.New("标签编号或版本错误")
		}
		res := tx.Model(&model.CMSTag{}).Where("tag_id=? AND version=?", in.TagId, in.Version).Updates(map[string]any{"name": in.Name, "status": in.Status, "remark": in.Remark, "version": in.Version + 1, "update_by": operator, "update_time": types.Now()})
		if res.Error != nil {
			return cmsDuplicate(res.Error)
		}
		if res.RowsAffected != 1 {
			return errs.WithCode(409, "标签已变化，请刷新")
		}
		return nil
	})
}
func cmsAttachOps(tx *gorm.DB, r *model.CMSRevision) error {
	if r.Ops != nil {
		return nil
	}
	r.Ops = &model.CMSContentOps{}
	if r.RevisionId < 1 {
		return nil
	}
	var row model.CMSRevisionOps
	e := tx.Where("revision_id=?", r.RevisionId).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	return json.Unmarshal([]byte(row.Payload), r.Ops)
}
func opsAsset(ids []int64, o *model.CMSContentOps) []int64 {
	if o == nil || o.ShareAssetId < 1 {
		return ids
	}
	for _, id := range ids {
		if id == o.ShareAssetId {
			return ids
		}
	}
	return append(ids, o.ShareAssetId)
}
func cmsStoreOps(tx *gorm.DB, r model.CMSRevision, operator string) error {
	if r.Ops == nil {
		return nil
	}
	b, e := json.Marshal(r.Ops)
	if e != nil {
		return e
	}
	row := model.CMSRevisionOps{RevisionId: r.RevisionId, Featured: r.Ops.Featured, PinOrder: r.Ops.PinOrder, Payload: string(b), CreateBy: operator, CreateTime: types.Now()}
	if e = tx.Create(&row).Error; e != nil {
		return e
	}
	if len(r.Ops.TagIds) > 0 {
		rows := []map[string]any{}
		for _, id := range r.Ops.TagIds {
			rows = append(rows, map[string]any{"revision_id": r.RevisionId, "tag_id": id, "create_by": operator, "create_time": types.Now(), "update_by": "", "remark": "内容版本标签"})
		}
		return tx.Table("cms_revision_tag").Create(rows).Error
	}
	return nil
}
func CMSGetOps(ctx context.Context, kind string, id int64) (*model.CMSOpsView, error) {
	d, e := CMSContentDetail(ctx, kind, id)
	if e != nil {
		return nil, e
	}
	if e = cmsAttachOps(cmsDB(ctx), &d.Draft); e != nil {
		return nil, e
	}
	return &model.CMSOpsView{ContentId: id, Version: d.Version, Title: d.Draft.Title, Summary: d.Draft.Summary, Slug: d.Slug, SeoTitle: d.Draft.SeoTitle, SeoDescription: d.Draft.SeoDescription, Settings: *d.Draft.Ops}, nil
}
func CMSSetOps(ctx context.Context, kind string, id int64, in model.CMSOpsInput, ids []int64, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		c, e := cmsFindContent(tx, kind, id)
		if e != nil {
			return e
		}
		if c.Version != in.Version {
			return errs.WithCode(409, "内容已变化，请刷新运营设置")
		}
		var r model.CMSRevision
		if c.WorkingRevisionId == nil {
			return errs.New("草稿缺失")
		}
		if e = tx.Where("revision_id=? AND content_id=?", *c.WorkingRevisionId, id).Take(&r).Error; e != nil {
			return e
		}
		in.Settings.Tags = nil
		if len(in.Settings.TagIds) > 0 {
			tags := []model.CMSTag{}
			if e = tx.Where("tag_id IN ? AND status='0'", in.Settings.TagIds).Find(&tags).Error; e != nil {
				return e
			}
			if len(tags) != len(in.Settings.TagIds) {
				return errs.New("标签不存在或已停用，请重新选择")
			}
			by := map[int64]string{}
			for _, t := range tags {
				by[t.TagId] = t.Name
			}
			for _, tag := range in.Settings.TagIds {
				in.Settings.Tags = append(in.Settings.Tags, model.CMSTagSnapshot{TagId: tag, Name: by[tag]})
			}
		}
		if len(in.Settings.RelatedIds) > 0 {
			var n int64
			if e = tx.Model(&model.CMSContent{}).Where("content_id IN ? AND content_id<>? AND is_deleted=0 AND kind IN ('ARTICLE','PRODUCT')", in.Settings.RelatedIds, id).Count(&n).Error; e != nil {
				return e
			}
			if n != int64(len(in.Settings.RelatedIds)) {
				return errs.New("关联内容不存在、重复、自引用或不是文章/产品")
			}
		}
		r.Ops = &in.Settings
		ids = opsAsset(ids, r.Ops)
		if e = cmsRevisionValid(tx, kind, r, ids, false); e != nil {
			return e
		}
		return cmsAppendRevision(tx, &c, r, ids, operator)
	})
}
func CMSRecoverContent(ctx context.Context, kind string, id, version int64, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var c model.CMSContent
		if e := tx.Where("content_id=? AND kind=? AND is_deleted=1", id, kind).Take(&c).Error; e != nil {
			return errs.New("回收站内容不存在")
		}
		if c.Version != version {
			return errs.WithCode(409, "回收站内容已变化，请刷新")
		}
		res := tx.Model(&model.CMSContent{}).Where("content_id=? AND version=? AND is_deleted=1", id, version).Updates(map[string]any{"is_deleted": 0, "is_published": 0, "published_revision_id": nil, "version": version + 1, "update_by": operator, "update_time": types.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.WithCode(409, "内容已变化")
		}
		return nil
	})
}
func CMSRelated(ctx context.Context, ids []int64) ([]model.PortalCard, error) {
	out := []model.PortalCard{}
	if len(ids) == 0 {
		return out, nil
	}
	tx := cmsDB(ctx)
	active, e := cmsActiveChannels(tx)
	if e != nil {
		return nil, e
	}
	rows := []model.PortalCard{}
	if e = cmsPublished(tx, active).Select("c.content_id,c.kind,c.slug,r.title,r.summary,r.cover_asset_id").Where("c.content_id IN ?", ids).Scan(&rows).Error; e != nil {
		return nil, e
	}
	by := map[int64]model.PortalCard{}
	for _, v := range rows {
		v.URL = portalURL(v.Kind, v.Slug)
		by[v.ContentId] = v
	}
	for _, id := range ids {
		if v, ok := by[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// A successful HTML GET is a page view, not a unique person. Never persist IP/UA/cookies/raw referrer.
func CMSRecordVisit(ctx context.Context, rows []model.CMSVisit) error {
	if len(rows) == 0 {
		return nil
	}
	tx := cmsDB(ctx)
	return tx.Transaction(func(db *gorm.DB) error {
		// Serialize only this day's analytics budget, independently of the content publication lock.
		if e := db.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"hits": gorm.Expr("hits+1"), "update_time": types.Now()})}).Create(&rows[0]).Error; e != nil {
			return e
		}
		if len(rows) == 1 {
			return nil
		}
		var n int64
		if e := db.Model(&model.CMSVisit{}).Where("visit_day=?", rows[0].VisitDay).Count(&n).Error; e != nil {
			return e
		}
		// Preserve total views; once the 2000-dimension daily budget is reached, stop optional detail writes.
		if n+int64(len(rows)-1) > 2000 {
			return nil
		}
		return db.Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"hits": gorm.Expr("hits+1"), "update_time": types.Now()})}).Create(rows[1:]).Error
	})
}
func CMSVisits(ctx context.Context, q model.CMSVisitQuery, pg page.Query) ([]model.CMSVisitRow, int64, error) {
	db := cmsDB(ctx).Model(&model.CMSVisit{}).Where("visit_day BETWEEN ? AND ?", q.From, q.To)
	dimension := q.Dimension
	if dimension == "day" {
		dimension = "total"
	}
	db = db.Where("dimension=?", dimension)
	key := "dimension_value"
	if dimension == "total" {
		key = "DATE_FORMAT(visit_day,'%Y-%m-%d')"
	}
	group := db.Select(key + " AS value,SUM(hits) AS hits").Group(key)
	var n int64
	if e := cmsDB(ctx).Table("(?) t", group).Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSVisitRow{}
	order := "hits DESC,value"
	if dimension == "total" {
		order = "value DESC"
	}
	e := group.Order(order).Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, e
}
func CMSCleanupVisits(ctx context.Context, days int) error {
	if days == 0 {
		days = 30
	}
	if days < 1 || days > 90 {
		return errs.New("统计保留期错误")
	}
	return cmsDB(ctx).Where("visit_day<?", time.Now().In(types.Location).AddDate(0, 0, -days).Format("2006-01-02")).Order("visit_day").Limit(500).Delete(&model.CMSVisit{}).Error
}

func CMSGetTag(ctx context.Context, id int64) (*model.CMSTag, error) {
	var row model.CMSTag
	e := cmsDB(ctx).Where("tag_id=?", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, errs.New("标签不存在")
	}
	return &row, e
}
