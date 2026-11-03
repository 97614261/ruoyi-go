package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsSyncInquiryClock(tx *gorm.DB, id int64) error {
	return tx.Exec(`INSERT INTO cms_inquiry_clock(inquiry_id,submitted_at,active_at,status,is_erased,source_version,create_by,create_time,update_by,update_time,remark) SELECT inquiry_id,create_time,COALESCE(update_time,create_time),status,is_deleted,version,'workflow',NOW(),'workflow',NOW(),'' FROM cms_inquiry WHERE inquiry_id=? ON DUPLICATE KEY UPDATE active_at=VALUES(active_at),status=VALUES(status),is_erased=VALUES(is_erased),source_version=VALUES(source_version),update_by='workflow',update_time=NOW()`, id).Error
}
func cmsInquiryFiltered(tx *gorm.DB, in model.CMSInquiryFilter) (*gorm.DB, error) {
	q, e := cmsInquiryScope(tx)
	if e != nil {
		return nil, e
	}
	q = cmsInquiryTaskFilter(q, in.CMSInquiryTaskQuery, cmsActor(tx.Statement.Context)).Joins("LEFT JOIN cms_inquiry_clock k ON k.inquiry_id=i.inquiry_id")
	if in.From != "" {
		q = q.Where("k.submitted_at>=?", in.From)
	}
	if in.To != "" {
		q = q.Where("k.submitted_at<?", in.To)
	}
	if in.OwnerUserId != nil {
		q = q.Where("COALESCE(t.owner_user_id,0)=?", *in.OwnerUserId)
	}
	return q, nil
}
func CMSInquiryFilteredTasks(ctx context.Context, in model.CMSInquiryFilter, pg page.Query) ([]model.CMSInquiryTask, int64, error) {
	q, e := cmsInquiryFiltered(cmsDB(ctx), in)
	if e != nil {
		return nil, 0, e
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSInquiryTask{}
	e = q.Joins("LEFT JOIN sys_user u ON u.user_id=t.owner_user_id").Select(inquiryMasked + ",COALESCE(t.owner_user_id,0) AS owner_user_id,COALESCE(u.nick_name,'未分配') AS owner_name,t.next_follow_at").Order("i.inquiry_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, e
}
func cmsDataManager(tx *gorm.DB, permission string, erase bool) (cmsAccess, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return a, e
	}
	if !a.has(permission) || !a.has("cms:data:list") {
		return a, errs.ErrForbidden
	}
	if permission != "cms:data:list" && (!a.has("cms:inquiry:list") || !a.has("cms:inquiry:query") || (!a.has("cms:inquiry:all") && !a.has("cms:inquiry:assign"))) {
		return a, errs.ErrForbidden
	}
	if erase && !a.has("cms:inquiry:remove") {
		return a, errs.ErrForbidden
	}
	return a, nil
}
func cmsDataEvent(tx *gorm.DB, a cmsAccess, action string, id, count int64, details, hash string) error {
	return tx.Create(&model.CMSDataEvent{ActorUserId: a.ID, Action: action, ResourceId: id, RowCount: count, Details: details, FileSHA256: hash, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}).Error
}
func CMSGetDataPolicy(ctx context.Context) (model.CMSDataPolicy, error) {
	var row model.CMSDataPolicy
	tx := cmsDB(ctx)
	if _, e := cmsDataManager(tx, "cms:data:list", false); e != nil {
		return row, e
	}
	e := tx.Where("policy_id=1").Take(&row).Error
	return row, e
}
func CMSSetDataPolicy(ctx context.Context, in model.CMSDataPolicyInput) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDataManager(tx, "cms:data:edit", false)
		if e != nil {
			return e
		}
		var p model.CMSDataPolicy
		if e = tx.Where("policy_id=1").Take(&p).Error; e != nil {
			return e
		}
		if p.Version != in.Version {
			return errs.New("保留策略已变化，请刷新")
		}
		old, _ := json.Marshal(p)
		next, _ := json.Marshal(in)
		statuses, _ := json.Marshal(in.Statuses)
		if e = tx.Model(&model.CMSDataPolicy{}).Where("policy_id=1 AND version=?", in.Version).Updates(map[string]any{"retention_days": in.RetentionDays, "statuses": string(statuses), "version": in.Version + 1, "remark": in.Reason, "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		return cmsDataEvent(tx, a, "POLICY", 1, 0, string(old)+"\n"+string(next), "")
	})
}
func cmsRetentionEligible(tx *gorm.DB, statuses []string, cutoff types.Time) *gorm.DB {
	return tx.Table("cms_inquiry_clock k").Joins("JOIN cms_inquiry i ON i.inquiry_id=k.inquiry_id AND i.is_deleted=0 AND i.version=k.source_version").Where("k.is_erased=0 AND k.status IN ? AND k.active_at<? AND i.status=k.status", statuses, cutoff)
}
func CMSPreviewCleanup(ctx context.Context, in model.CMSCleanupInput) (model.CMSCleanupBatch, error) {
	var batch model.CMSCleanupBatch
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDataManager(tx, "cms:data:cleanup", true)
		if e != nil {
			return e
		}
		var p model.CMSDataPolicy
		if e = tx.Where("policy_id=1").Take(&p).Error; e != nil {
			return e
		}
		if p.RetentionDays == 0 {
			return errs.New("当前为不限期保留，请先配置保留天数")
		}
		cutoff := types.Time(time.Now().In(types.Location).AddDate(0, 0, -p.RetentionDays))
		q := cmsRetentionEligible(tx, p.Statuses, cutoff)
		if len(in.InquiryIds) > 0 {
			q = q.Where("i.inquiry_id IN ?", in.InquiryIds)
		}
		var total int64
		if e = q.Count(&total).Error; e != nil {
			return e
		}
		var source []struct {
			InquiryId int64
			Version   int64
		}
		if e = q.Select("i.inquiry_id,i.version").Order("k.active_at,k.inquiry_id").Limit(100).Scan(&source).Error; e != nil {
			return e
		}
		batch = model.CMSCleanupBatch{ActorUserId: a.ID, PolicyVersion: p.Version, Cutoff: cutoff, Statuses: p.Statuses, TotalEligible: total, PreviewCount: len(source), ExpiresAt: types.Time(time.Now().Add(15 * time.Minute)), Status: "PREVIEWED", Version: 1, Reason: in.Reason, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
		if e = tx.Create(&batch).Error; e != nil {
			return e
		}
		items := []model.CMSCleanupItem{}
		for _, r := range source {
			items = append(items, model.CMSCleanupItem{BatchId: batch.BatchId, InquiryId: r.InquiryId, SourceVersion: r.Version, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}})
		}
		if len(items) > 0 {
			if e = tx.Create(&items).Error; e != nil {
				return e
			}
		}
		return cmsDataEvent(tx, a, "PREVIEW", batch.BatchId, int64(len(source)), fmt.Sprintf("policy=%d;total=%d;最多100条;预览不擦除", p.Version, total), "")
	})
	return batch, e
}
func cmsCleanupBatch(tx *gorm.DB, id, actor int64) (model.CMSCleanupBatch, error) {
	var b model.CMSCleanupBatch
	e := tx.Where("batch_id=? AND actor_user_id=?", id, actor).Take(&b).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = errs.ErrNotFound
	}
	return b, e
}
func CMSCleanupBatches(ctx context.Context, pg page.Query) ([]model.CMSCleanupBatch, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsDataManager(tx, "cms:data:cleanup", true)
	if e != nil {
		return nil, 0, e
	}
	q := tx.Model(&model.CMSCleanupBatch{}).Where("actor_user_id=?", a.ID)
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.CMSCleanupBatch{}
	e = q.Order("batch_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Find(&out).Error
	return out, n, e
}
func CMSCleanupItems(ctx context.Context, id int64, pg page.Query) ([]model.CMSCleanupItemView, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsDataManager(tx, "cms:data:cleanup", true)
	if e != nil {
		return nil, 0, e
	}
	b, e := cmsCleanupBatch(tx, id, a.ID)
	if e != nil {
		return nil, 0, e
	}
	rows := []model.CMSCleanupItemView{}
	q := tx.Table("cms_data_cleanup_item x").Joins("LEFT JOIN cms_inquiry i ON i.inquiry_id=x.inquiry_id").Joins("LEFT JOIN cms_inquiry_clock k ON k.inquiry_id=x.inquiry_id").Where("x.batch_id=?", id)
	e = q.Select("x.inquiry_id,x.source_version,COALESCE(i.version,0) AS current_version,i.name,CASE WHEN i.is_deleted<>0 THEN '' WHEN CHAR_LENGTH(i.contact)>4 THEN CONCAT(LEFT(i.contact,2),'***',RIGHT(i.contact,2)) ELSE '***' END AS contact,i.status,k.active_at").Order("x.inquiry_id").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, int64(b.PreviewCount), e
}
func cmsEraseInquiries(tx *gorm.DB, ids []int64, operator string) error {
	if len(ids) == 0 {
		return nil
	}
	if e := tx.Model(&model.CMSInquiry{}).Where("inquiry_id IN ? AND is_deleted=0", ids).Updates(map[string]any{"name": "", "contact": "", "message": "", "note": "", "status": "SPAM", "is_deleted": 1, "version": gorm.Expr("version+1"), "update_by": operator, "update_time": types.Now()}).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.CMSInquiryTeam{}).Where("inquiry_id IN ?", ids).Updates(map[string]any{"next_follow_at": nil, "reminder_sent_version": 0, "version": gorm.Expr("version+1"), "update_by": operator, "update_time": types.Now()}).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.CMSInquiryFollowup{}).Where("inquiry_id IN ?", ids).Updates(map[string]any{"text": "", "result": "", "is_erased": 1, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
		return e
	}
	if e := tx.Model(&model.CMSNotificationOutbox{}).Where("resource_type='INQUIRY' AND resource_id IN ? AND status='PENDING'", ids).Update("status", "CANCELED").Error; e != nil {
		return e
	}
	if e := tx.Model(&model.CMSNotification{}).Where("resource_type='INQUIRY' AND resource_id IN ?", ids).Update("expires_at", gorm.Expr("NOW()")).Error; e != nil {
		return e
	}
	return tx.Model(&model.CMSInquiryClock{}).Where("inquiry_id IN ?", ids).Updates(map[string]any{"is_erased": 1, "source_version": gorm.Expr("source_version+1"), "active_at": types.Now(), "update_by": operator, "update_time": types.Now()}).Error
}
func CMSExecuteCleanup(ctx context.Context, id, version int64) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDataManager(tx, "cms:data:cleanup", true)
		if e != nil {
			return e
		}
		b, e := cmsCleanupBatch(tx, id, a.ID)
		if e != nil {
			return e
		}
		if b.Status != "PREVIEWED" || b.Version != version || !time.Time(b.ExpiresAt).After(time.Now()) {
			return errs.New("清理预览已执行、过期或变化，请重新预览")
		}
		var p model.CMSDataPolicy
		if e = tx.Where("policy_id=1").Take(&p).Error; e != nil {
			return e
		}
		if p.Version != b.PolicyVersion || p.RetentionDays == 0 {
			return errs.New("保留策略已变化，请重新预览")
		}
		items := []model.CMSCleanupItem{}
		if e = tx.Where("batch_id=?", id).Order("inquiry_id").Find(&items).Error; e != nil {
			return e
		}
		if len(items) == 0 {
			return errs.New("此预览没有可清理数据")
		}
		ids := []int64{}
		for _, x := range items {
			ids = append(ids, x.InquiryId)
		}
		var current []struct {
			InquiryId int64
			Version   int64
		}
		if e = cmsRetentionEligible(tx, b.Statuses, b.Cutoff).Where("i.inquiry_id IN ?", ids).Select("i.inquiry_id,i.version").Scan(&current).Error; e != nil {
			return e
		}
		versions := map[int64]int64{}
		for _, r := range current {
			versions[r.InquiryId] = r.Version
		}
		for _, x := range items {
			if versions[x.InquiryId] != x.SourceVersion {
				return errs.New("预览后有询盘被跟进、改派、处理或清除，本批未执行，请重新预览")
			}
		}
		if e = cmsEraseInquiries(tx, ids, a.Name); e != nil {
			return e
		}
		if e = tx.Model(&model.CMSCleanupBatch{}).Where("batch_id=? AND version=?", id, version).Updates(map[string]any{"status": "EXECUTED", "erased_count": len(ids), "version": version + 1, "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		return cmsDataEvent(tx, a, "ERASE", id, int64(len(ids)), "按已确认预览原子擦除，备份另按保留策略管理", "")
	})
}
func CMSDataEvents(ctx context.Context, ownExport bool, pg page.Query) ([]model.CMSDataEvent, int64, error) {
	tx := cmsDB(ctx)
	a, e := cmsCurrentAccess(tx, cmsActor(ctx))
	if e != nil {
		return nil, 0, e
	}
	q := tx.Model(&model.CMSDataEvent{})
	if ownExport {
		if !a.has("cms:inquiry:export") {
			return nil, 0, errs.ErrForbidden
		}
		q = q.Where("actor_user_id=? AND action='EXPORT'", a.ID)
	} else {
		if _, e = cmsDataManager(tx, "cms:data:audit", false); e != nil {
			return nil, 0, e
		}
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.CMSDataEvent{}
	e = q.Order("event_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&out).Error
	return out, n, e
}
func CMSExportInquiries(ctx context.Context, in model.CMSInquiryExport, render func([]model.CMSInquiryExportRow) ([]byte, error)) ([]byte, error) {
	var data []byte
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		if !a.has("cms:inquiry:export") || !a.has("cms:inquiry:list") {
			return errs.ErrForbidden
		}
		if in.IncludePrivate && (!a.has("cms:inquiry:exportPrivate") || !a.has("cms:inquiry:query")) {
			return errs.ErrForbidden
		}
		q, e := cmsInquiryFiltered(tx, in.CMSInquiryFilter)
		if e != nil {
			return e
		}
		columns := inquiryMasked + ",COALESCE(u.nick_name,'未分配') AS owner_name,t.next_follow_at"
		if in.IncludePrivate {
			columns = "i.inquiry_id,i.name,i.contact,i.message,i.note,i.status,i.create_time,COALESCE(u.nick_name,'未分配') AS owner_name,t.next_follow_at"
		}
		rows := []model.CMSInquiryExportRow{}
		if e = q.Joins("LEFT JOIN sys_user u ON u.user_id=t.owner_user_id").Select(columns).Order("i.inquiry_id DESC").Limit(5001).Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) > 5000 {
			return errs.New("筛选结果超过5000条，请缩小时间或负责人范围后分批导出，不会截断")
		}
		data, e = render(rows)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		details, _ := json.Marshal(in)
		return cmsDataEvent(tx, a, "EXPORT", 0, int64(len(rows)), string(details), hash)
	})
	if e != nil {
		return nil, e
	}
	return data, nil
}
