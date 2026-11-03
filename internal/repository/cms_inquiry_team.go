package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsCancelInquiryDue(tx *gorm.DB, id int64) error {
	return tx.Model(&model.CMSNotificationOutbox{}).Where("resource_type='INQUIRY' AND resource_id=? AND event_type='INQUIRY_DUE' AND status='PENDING'", id).Updates(map[string]any{"status": "CANCELED", "update_time": types.Now()}).Error
}
func CMSInquiryTeamDetail(ctx context.Context, id int64) (model.CMSInquiryTeam, error) {
	tx := cmsDB(ctx)
	var row model.CMSInquiryTeam
	if _, e := cmsInquiryVisible(tx, id); e != nil {
		return row, e
	}
	e := tx.Where("inquiry_id=?", id).Find(&row).Error
	if row.InquiryId == 0 {
		row.InquiryId = id
	}
	return row, e
}
func cmsTeamActor(tx *gorm.DB, perm string) (cmsAccess, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return a, e
	}
	if !a.has("cms:inquiry:query") || !a.has(perm) {
		return a, errs.ErrForbidden
	}
	return a, nil
}
func CMSAssignInquiry(ctx context.Context, id int64, in model.CMSInquiryAssign) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsTeamActor(tx, "cms:inquiry:assign")
		if e != nil {
			return e
		}
		row, e := cmsInquiryVisible(tx, id)
		if e != nil {
			return e
		}
		if row.Version != in.Version {
			return errs.New("询盘已变化，请刷新")
		}
		if in.OwnerUserId > 0 {
			var n int64
			if e = cmsPeopleDB(tx, "inquiry", "").Where("u.user_id=?", in.OwnerUserId).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return errs.New("负责人须启用并具备询盘列表、详情及处理权限")
			}
		}
		var old model.CMSInquiryTeam
		if e = tx.Where("inquiry_id=?", id).Find(&old).Error; e != nil {
			return e
		}
		team := model.CMSInquiryTeam{InquiryId: id, OwnerUserId: in.OwnerUserId, Version: row.Version + 1, NextFollowAt: old.NextFollowAt, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now(), UpdateBy: a.Name, UpdateTime: types.Now()}}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "inquiry_id"}}, DoUpdates: clause.AssignmentColumns([]string{"owner_user_id", "version", "reminder_sent_version", "update_by", "update_time"})}).Create(&team).Error; e != nil {
			return e
		}
		if e = tx.Model(&model.CMSInquiry{}).Where("inquiry_id=? AND version=?", id, in.Version).Updates(map[string]any{"version": row.Version + 1, "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		if e = cmsSyncInquiryClock(tx, id); e != nil {
			return e
		}
		record := model.CMSInquiryFollowup{InquiryId: id, ActorUserId: a.ID, EventType: "ASSIGN", Result: fmt.Sprintf("负责人 #%d → #%d", old.OwnerUserId, in.OwnerUserId), CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
		if e = tx.Create(&record).Error; e != nil {
			return e
		}
		if e = cmsCancelInquiryDue(tx, id); e != nil {
			return e
		}
		if in.OwnerUserId > 0 {
			return cmsEnqueue(tx, "INQUIRY", id, "", "INQUIRY_ASSIGNED", row.Version+1, []int64{in.OwnerUserId}, "你有一条新分配的询盘")
		}
		return nil
	})
}
func CMSFollowInquiry(ctx context.Context, id int64, in model.CMSInquiryFollowInput) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsTeamActor(tx, "cms:inquiry:edit")
		if e != nil {
			return e
		}
		row, e := cmsInquiryVisible(tx, id)
		if e != nil {
			return e
		}
		if row.Version != in.Version {
			return errs.New("询盘已变化，请刷新")
		}
		if !in.Due.IsZero() && row.Status != "NEW" {
			return errs.New("仅待处理询盘可安排跟进提醒")
		}
		team := model.CMSInquiryTeam{InquiryId: id, NextFollowAt: in.Due, Version: row.Version + 1, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now(), UpdateBy: a.Name, UpdateTime: types.Now()}}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "inquiry_id"}}, DoUpdates: clause.AssignmentColumns([]string{"next_follow_at", "reminder_sent_version", "version", "update_by", "update_time"})}).Create(&team).Error; e != nil {
			return e
		}
		if e = tx.Model(&model.CMSInquiry{}).Where("inquiry_id=? AND version=?", id, in.Version).Updates(map[string]any{"version": row.Version + 1, "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		if e = cmsSyncInquiryClock(tx, id); e != nil {
			return e
		}
		if e = tx.Create(&model.CMSInquiryFollowup{InquiryId: id, ActorUserId: a.ID, EventType: "FOLLOWUP", Text: in.Text, Result: in.Result, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}).Error; e != nil {
			return e
		}
		return cmsCancelInquiryDue(tx, id)
	})
}
func CMSInquiryFollowups(ctx context.Context, id int64, pg page.Query) ([]model.CMSInquiryFollowup, int64, error) {
	tx := cmsDB(ctx)
	if _, e := cmsInquiryVisible(tx, id); e != nil {
		return nil, 0, e
	}
	q := tx.Model(&model.CMSInquiryFollowup{}).Where("inquiry_id=? AND is_erased=0", id)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSInquiryFollowup{}
	e := q.Order("followup_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func cmsInquiryTaskFilter(q *gorm.DB, in model.CMSInquiryTaskQuery, actor int64) *gorm.DB {
	if in.Status != "" {
		q = q.Where("i.status=?", in.Status)
	}
	if in.Mine {
		q = q.Where("t.owner_user_id=?", actor)
	}
	now := time.Now().In(types.Location)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, types.Location)
	switch in.Due {
	case "today":
		q = q.Where("i.status='NEW' AND t.next_follow_at>=? AND t.next_follow_at<?", day, day.AddDate(0, 0, 1))
	case "overdue":
		q = q.Where("i.status='NEW' AND t.next_follow_at<?", now)
	case "scheduled":
		q = q.Where("i.status='NEW' AND t.next_follow_at IS NOT NULL")
	}
	return q
}
func CMSInquiryTasks(ctx context.Context, in model.CMSInquiryTaskQuery, pg page.Query) ([]model.CMSInquiryTask, int64, error) {
	q, e := cmsInquiryScope(cmsDB(ctx))
	if e != nil {
		return nil, 0, e
	}
	q = cmsInquiryTaskFilter(q, in, cmsActor(ctx))
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSInquiryTask{}
	e = q.Joins("LEFT JOIN sys_user u ON u.user_id=t.owner_user_id").Select(inquiryMasked + ",COALESCE(t.owner_user_id,0) AS owner_user_id,COALESCE(u.nick_name,'未分配') AS owner_name,t.next_follow_at").Order("i.inquiry_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
