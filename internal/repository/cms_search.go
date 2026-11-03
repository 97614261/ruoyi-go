package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalsearch"
	"ruoyi-go/pkg/types"
)

func cmsQueueSearch(tx *gorm.DB, id int64) error {
	return tx.Exec(`INSERT INTO cms_search_task(content_id,desired_revision_id,status,attempts,error_message,version,create_by,create_time,update_by,update_time,remark) SELECT content_id,CASE WHEN is_deleted=0 AND is_published=1 THEN published_revision_id ELSE NULL END,'PENDING',0,'',1,'publication',NOW(),'publication',NOW(),'' FROM cms_content WHERE content_id=? ON DUPLICATE KEY UPDATE desired_revision_id=VALUES(desired_revision_id),status='PENDING',attempts=0,error_message='',version=cms_search_task.version+1,update_by='publication',update_time=NOW()`, id).Error
}
func cmsSearchActor(tx *gorm.DB, edit bool) (cmsAccess, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return a, e
	}
	if !a.has("cms:search:list") || (edit && !a.has("cms:search:edit")) {
		return a, errs.ErrForbidden
	}
	return a, nil
}
func CMSRebuildSearch(ctx context.Context) (int64, error) {
	var n int64
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsSearchActor(tx, true)
		if e != nil {
			return e
		}
		kinds := cmsSourceKinds(a, "query")
		predicate := "c.kind IN ? AND ((c.is_deleted=0 AND c.is_published=1) OR EXISTS(SELECT 1 FROM cms_search_document d WHERE d.content_id=c.content_id))"
		if e = tx.Table("cms_content c").Where(predicate, kinds).Count(&n).Error; e != nil {
			return e
		}
		return tx.Exec(`INSERT INTO cms_search_task(content_id,desired_revision_id,status,attempts,error_message,version,create_by,create_time,update_by,update_time,remark) SELECT c.content_id,CASE WHEN c.is_deleted=0 AND c.is_published=1 THEN c.published_revision_id ELSE NULL END,'PENDING',0,'',1,?,NOW(),?,NOW(),'' FROM cms_content c WHERE `+predicate+` ON DUPLICATE KEY UPDATE desired_revision_id=VALUES(desired_revision_id),status='PENDING',attempts=0,error_message='',version=cms_search_task.version+1,update_by=VALUES(update_by),update_time=NOW()`, a.Name, a.Name, kinds).Error
	})
	return n, e
}
func CMSSearchStatus(ctx context.Context) (model.CMSSearchOverview, error) {
	out := model.CMSSearchOverview{MaxAttempts: 3}
	tx := cmsDB(ctx)
	a, e := cmsSearchActor(tx, false)
	if e != nil {
		return out, e
	}
	var rows []struct {
		Status string
		Total  int64
	}
	e = tx.Table("cms_search_task t").Select("t.status,COUNT(*) AS total").Joins("JOIN cms_content c ON c.content_id=t.content_id").Where("c.kind IN ?", cmsSourceKinds(a, "query")).Group("t.status").Scan(&rows).Error
	for _, r := range rows {
		switch r.Status {
		case "READY":
			out.Ready = r.Total
		case "FAILED":
			out.Failed = r.Total
		case "PENDING":
			out.Pending = r.Total
		}
	}
	return out, e
}
func CMSSearchTasks(ctx context.Context, in model.CMSSearchQuery, pg page.Query) ([]model.CMSSearchTaskView, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsSearchActor(tx, false)
	if e != nil {
		return nil, 0, e
	}
	q := tx.Table("cms_search_task t").Joins("JOIN cms_content c ON c.content_id=t.content_id").Joins("LEFT JOIN cms_content_revision r ON r.revision_id=c.published_revision_id AND r.content_id=c.content_id").Where("c.kind IN ?", cmsSourceKinds(a, "query"))
	if in.Status != "" {
		q = q.Where("t.status=?", in.Status)
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSSearchTaskView{}
	e = q.Select("t.*,c.kind,CASE WHEN c.is_deleted=1 THEN '已删除' WHEN c.is_published=0 THEN '已下架' ELSE COALESCE(r.title,'无公开版本') END AS title").Order("t.content_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, e
}
func CMSFetchSearchJobs(ctx context.Context) ([]model.CMSSearchJob, error) {
	rows := []model.CMSSearchJob{}
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		if e := tx.Model(&model.CMSSearchTask{}).Where("status='PENDING' AND attempts>=3").Updates(map[string]any{"status": "FAILED", "error_message": "连续索引尝试失败，请检查后重建", "update_time": types.Now(), "update_by": "indexer"}).Error; e != nil {
			return e
		}
		if e := tx.Table("cms_search_task t").Select("t.*,c.is_published,c.is_deleted,c.published_revision_id,r.title,r.summary,r.body_html,r.product_params,r.product_code").Joins("JOIN cms_content c ON c.content_id=t.content_id").Joins("LEFT JOIN cms_content_revision r ON r.revision_id=c.published_revision_id AND r.content_id=c.content_id").Where("t.status='PENDING' AND t.attempts<3").Order("t.content_id").Limit(2).Scan(&rows).Error; e != nil {
			return e
		}
		ids := []int64{}
		for _, r := range rows {
			ids = append(ids, r.ContentId)
		}
		if len(ids) > 0 {
			return tx.Model(&model.CMSSearchTask{}).Where("content_id IN ?", ids).Update("attempts", gorm.Expr("attempts+1")).Error
		}
		return nil
	})
	return rows, e
}
func cmsSameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func CMSCommitSearch(ctx context.Context, built []model.CMSSearchBuilt) error {
	if len(built) == 0 {
		return nil
	}
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		ids := []int64{}
		for _, b := range built {
			ids = append(ids, b.Job.ContentId)
		}
		var current []struct {
			model.CMSSearchTask
			IsPublished         int
			IsDeleted           int
			PublishedRevisionId *int64
		}
		if e := tx.Table("cms_search_task t").Select("t.*,c.is_published,c.is_deleted,c.published_revision_id").Joins("JOIN cms_content c ON c.content_id=t.content_id").Where("t.content_id IN ?", ids).Scan(&current).Error; e != nil {
			return e
		}
		by := map[int64]int{}
		for i, v := range current {
			by[v.ContentId] = i
		}
		valid, ready, failed := []int64{}, []int64{}, []int64{}
		docs := []model.CMSSearchDocument{}
		postings := []model.CMSSearchPosting{}
		for _, b := range built {
			i, ok := by[b.Job.ContentId]
			if !ok {
				continue
			}
			c := current[i]
			j := b.Job
			if c.Version != j.Version || c.Status != "PENDING" || c.IsPublished != j.IsPublished || c.IsDeleted != j.IsDeleted || !cmsSameID(c.PublishedRevisionId, j.PublishedRevisionId) {
				continue
			}
			valid = append(valid, j.ContentId)
			if b.Error != "" {
				failed = append(failed, j.ContentId)
				continue
			}
			ready = append(ready, j.ContentId)
			if j.IsPublished == 1 && j.IsDeleted == 0 && j.PublishedRevisionId != nil {
				docs = append(docs, b.Doc)
				postings = append(postings, b.Postings...)
			}
		}
		if len(valid) == 0 {
			return nil
		}
		if e := tx.Where("content_id IN ?", valid).Delete(&model.CMSSearchPosting{}).Error; e != nil {
			return e
		}
		if e := tx.Where("content_id IN ?", valid).Delete(&model.CMSSearchDocument{}).Error; e != nil {
			return e
		}
		if len(docs) > 0 {
			if e := tx.Create(&docs).Error; e != nil {
				return e
			}
		}
		if len(postings) > 0 {
			if e := tx.CreateInBatches(&postings, 500).Error; e != nil {
				return e
			}
		}
		if len(ready) > 0 {
			if e := tx.Model(&model.CMSSearchTask{}).Where("content_id IN ?", ready).Updates(map[string]any{"status": "READY", "error_message": "", "update_by": "indexer", "update_time": types.Now()}).Error; e != nil {
				return e
			}
		}
		if len(failed) > 0 {
			return tx.Model(&model.CMSSearchTask{}).Where("content_id IN ?", failed).Updates(map[string]any{"status": "FAILED", "error_message": "公开文本超出索引限制，请检查后重建", "update_by": "indexer", "update_time": types.Now()}).Error
		}
		return nil
	})
}
func PortalFullTextResults(ctx context.Context, keyword string, tag int64, offset, limit int) ([]model.PortalSearchRow, int64, []model.CmsChannel, error) {
	tx := cmsDB(ctx)
	active, e := cmsActiveChannels(tx)
	if e != nil {
		return nil, 0, nil, e
	}
	channels := []model.CmsChannel{}
	if e = tx.Where("channel_id IN ?", active).Order("order_num,channel_id").Find(&channels).Error; e != nil {
		return nil, 0, nil, e
	}
	hashes := portalsearch.Hashes(keyword)
	if len(hashes) == 0 {
		return []model.PortalSearchRow{}, 0, channels, nil
	}
	matches := tx.Model(&model.CMSSearchPosting{}).Select("content_id,SUM(weight) AS score").Where("token_hash IN ?", hashes).Group("content_id").Having("COUNT(*)=?", len(hashes))
	q := cmsPublished(tx, active).Joins("LEFT JOIN cms_search_document d ON d.content_id=c.content_id AND d.revision_id=r.revision_id").Joins("LEFT JOIN (?) m ON m.content_id=c.content_id", matches)
	if tag > 0 {
		q = q.Where("EXISTS(SELECT 1 FROM cms_revision_tag rt WHERE rt.revision_id=r.revision_id AND rt.tag_id=?)", tag)
	}
	indexed := tx.Session(&gorm.Session{NewDB: true}).Where("d.content_id IS NOT NULL AND m.content_id IS NOT NULL")
	for _, term := range portalsearch.Terms(keyword) {
		indexed = indexed.Where("d.search_text COLLATE utf8mb4_bin LIKE ?", "%"+escapeLike(term)+"%")
	}
	// Preserve immediate title/summary lookup while the managed body index catches up. No stale body is selected.
	pattern := "%" + escapeLike(keyword) + "%"
	q = q.Where(tx.Where(indexed).Or("d.content_id IS NULL AND (r.title LIKE ? OR r.summary LIKE ?)", pattern, pattern))
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, nil, e
	}
	out := []model.PortalSearchRow{}
	e = q.Select("c.content_id,c.kind,c.slug,r.title,r.summary,c.published_at,r.cover_asset_id,r.channel_id,COALESCE(d.body_text,'') AS body_text,CASE WHEN d.content_id IS NOT NULL THEN COALESCE(m.score,0) ELSE 0 END + CASE WHEN INSTR(LOWER(r.title),?)>0 THEN 10000 ELSE 0 END AS score", portalsearch.Normalize(keyword)).Order("score DESC,c.published_at DESC,c.content_id DESC").Offset(offset).Limit(limit).Scan(&out).Error
	for i := range out {
		out[i].URL = portalURL(out[i].Kind, out[i].Slug)
	}
	return out, n, channels, e
}
