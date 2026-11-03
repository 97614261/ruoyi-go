package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsSourceKinds(a cmsAccess, action string) []string {
	out := []string{}
	for _, k := range []string{"ARTICLE", "PRODUCT", "PAGE"} {
		if a.source(k, "query") && a.source(k, action) {
			out = append(out, k)
		}
	}
	return out
}
func cmsOwnDrafts(tx *gorm.DB, a cmsAccess) *gorm.DB {
	return tx.Table("cms_content c").Joins("JOIN cms_content_revision r ON r.revision_id=c.working_revision_id AND r.content_id=c.content_id").Joins("JOIN cms_draft_actor d ON d.kind COLLATE utf8mb4_general_ci=c.kind AND d.snapshot_key=CAST(c.working_revision_id AS CHAR CHARACTER SET utf8mb4) COLLATE utf8mb4_unicode_ci AND d.object_id=c.content_id").Where("c.is_deleted=0 AND d.author_user_id=? AND c.kind IN ? AND (c.is_published=0 OR c.published_revision_id<>c.working_revision_id)", a.ID, cmsSourceKinds(a, "edit"))
}
func CMSOwnDrafts(ctx context.Context, pg page.Query) ([]model.CMSContentView, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	q := cmsOwnDrafts(tx, a)
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.CMSContentView{}
	e = q.Select("c.*,r.title,r.summary,r.channel_id").Order("c.content_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&out).Error
	return out, n, e
}
func CMSOwnFailedSchedules(ctx context.Context, pg page.Query) ([]model.CMSSchedule, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	q := tx.Table("cms_schedule s").Joins("JOIN cms_content c ON c.content_id=s.content_id AND c.is_deleted=0").Where("s.owner_user_id=? AND s.status='FAILED' AND s.kind IN ?", a.ID, cmsSourceKinds(a, "query"))
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.CMSSchedule{}
	e = q.Select("s.*").Order("s.schedule_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&out).Error
	return out, n, e
}
func CMSWorkbench(ctx context.Context) (model.CMSWorkbench, error) {
	out := model.CMSWorkbench{Visits: []model.CMSVisitRow{}}
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return out, e
	}
	if e = cmsOwnDrafts(tx, a).Count(&out.OwnDrafts).Error; e != nil {
		return out, e
	}
	// Aggregate all review states once, filtered by the same visibility scope as the center.
	var counts []struct {
		Status string
		Total  int64
	}
	if e = cmsReviewVisible(cmsCurrentReviewTasks(tx), a).Where("submitter_user_id=?", a.ID).Select("cms_review_request.status,COUNT(*) AS total").Group("status").Scan(&counts).Error; e != nil {
		return out, e
	}
	for _, c := range counts {
		switch c.Status {
		case "PENDING":
			out.Submitted = c.Total
		case "APPROVED":
			out.Approved = c.Total
		case "REJECTED":
			out.Rejected = c.Total
		}
	}
	kinds := []string{}
	for _, k := range []string{"ARTICLE", "PRODUCT", "PAGE", "SITE"} {
		if a.source(k, "query") && a.source(k, "review") {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) > 0 {
		if e = cmsCurrentReviewTasks(tx).Where("cms_review_request.kind IN ? AND status='PENDING' AND submitter_user_id<>? AND author_user_id<>?", kinds, a.ID, a.ID).Count(&out.PendingReview).Error; e != nil {
			return out, e
		}
	}
	if a.has("cms:inquiry:list") {
		out.InquiryVisible = true
		q, e := cmsInquiryScope(tx)
		if e != nil {
			return out, e
		}
		now := time.Now().In(types.Location)
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, types.Location)
		var c struct {
			Total   int64
			Today   int64
			Overdue int64
		}
		if e = q.Where("i.status='NEW'").Select("COUNT(*) AS total,COALESCE(SUM(t.next_follow_at>=? AND t.next_follow_at<?),0) AS today,COALESCE(SUM(t.next_follow_at<?),0) AS overdue", day, day.AddDate(0, 0, 1), now).Scan(&c).Error; e != nil {
			return out, e
		}
		out.InquiryNew = c.Total
		out.InquiryToday = c.Today
		out.InquiryOverdue = c.Overdue
	}
	if e = tx.Table("cms_schedule s").Joins("JOIN cms_content c ON c.content_id=s.content_id AND c.is_deleted=0").Where("s.owner_user_id=? AND s.status='FAILED' AND s.kind IN ?", a.ID, cmsSourceKinds(a, "query")).Count(&out.FailedSchedules).Error; e != nil {
		return out, e
	}
	if a.has("cms:statistics:list") {
		out.AnalyticsVisible = true
		cfg, e := CMSRawPublishedSite(ctx)
		if e != nil {
			return out, e
		}
		out.AnalyticsEnabled = cfg.AnalyticsEnabled
		if cfg.AnalyticsEnabled {
			e = tx.Model(&model.CMSVisit{}).Select("DATE_FORMAT(visit_day,'%Y-%m-%d') AS value,hits").Where("dimension='total' AND visit_day BETWEEN ? AND ?", time.Now().In(types.Location).AddDate(0, 0, -6).Format("2006-01-02"), time.Now().In(types.Location).Format("2006-01-02")).Order("visit_day").Limit(7).Scan(&out.Visits).Error
			if e != nil {
				return out, e
			}
		}
	}
	return out, nil
}

func cmsCurrentReviewTasks(tx *gorm.DB) *gorm.DB {
	return tx.Model(&model.CMSReviewRequest{}).Joins("LEFT JOIN cms_content wc ON cms_review_request.kind<>'SITE' AND wc.content_id=cms_review_request.object_id").Joins("LEFT JOIN cms_site ws ON cms_review_request.kind='SITE' AND ws.site_id=cms_review_request.object_id").Where("(cms_review_request.kind='SITE' AND ws.version=cms_review_request.content_version) OR (cms_review_request.kind<>'SITE' AND wc.is_deleted=0 AND wc.version=cms_review_request.content_version)")
}
