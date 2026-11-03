package repository

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"time"
)

func cmsEnqueue(tx *gorm.DB, resource string, id int64, kind, event string, version int64, recipients []int64, subject string) error {
	rows := []model.CMSNotificationOutbox{}
	seen := map[int64]bool{}
	for _, uid := range recipients {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		rows = append(rows, model.CMSNotificationOutbox{EventKey: fmt.Sprintf("%s:%d:%s:%d:%d", resource, id, event, version, uid), ResourceType: resource, ResourceId: id, Kind: kind, EventType: event, BusinessVersion: version, RecipientUserId: uid, Subject: subject, Status: "PENDING", CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: "workflow", CreateTime: types.Now()}})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100).Error
}

// Apply visibility in SQL BEFORE both Count and pagination; no stale page-side filtering.
func cmsNotificationScope(tx *gorm.DB) (*gorm.DB, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return nil, e
	}
	if !a.has("cms:notification:list") {
		return nil, errs.ErrForbidden
	}
	q := tx.Table("cms_notification n").Joins("LEFT JOIN cms_inquiry i ON n.resource_type='INQUIRY' AND i.inquiry_id=n.resource_id").Joins("LEFT JOIN cms_inquiry_team t ON t.inquiry_id=i.inquiry_id").Joins("LEFT JOIN cms_review_request r ON n.resource_type='REVIEW' AND r.review_id=n.resource_id").Joins("LEFT JOIN cms_content rc ON r.kind<>'SITE' AND rc.content_id=r.object_id").Joins("LEFT JOIN cms_schedule s ON n.resource_type='SCHEDULE' AND s.schedule_id=n.resource_id").Joins("LEFT JOIN cms_content sc ON sc.content_id=s.content_id").Where("n.recipient_user_id=? AND n.expires_at>?", a.ID, types.Now())
	queryKinds, reviewKinds := []string{}, []string{}
	for _, k := range []string{"ARTICLE", "PRODUCT", "PAGE", "SITE"} {
		if a.source(k, "query") {
			queryKinds = append(queryKinds, k)
			if a.source(k, "review") {
				reviewKinds = append(reviewKinds, k)
			}
		}
	}
	// Empty IN lists bind as (NULL), not arbitrary SQL. All branches also check the resource's current state.
	reviewSQL := "(n.resource_type='REVIEW' AND (r.kind='SITE' OR rc.is_deleted=0) AND r.kind IN ? AND ((r.submitter_user_id=? OR r.author_user_id=?) OR (r.kind IN ? AND r.status='PENDING' AND r.submitter_user_id<>? AND r.author_user_id<>?)))"
	inquirySQL := "(1=0)"
	args := []any{queryKinds, a.ID, a.ID, reviewKinds, a.ID, a.ID}
	if a.has("cms:inquiry:list") && a.has("cms:inquiry:query") {
		var setting model.CMSCollaborationSetting
		if e = tx.Where("setting_id=1").Take(&setting).Error; e != nil {
			return nil, e
		}
		inquirySQL = "(n.resource_type='INQUIRY' AND i.is_deleted=0 AND (n.event_type<>'INQUIRY_DUE' OR (i.status='NEW' AND i.version=n.business_version AND t.next_follow_at<=NOW())))"
		if setting.InquiryIsolation && !a.has("cms:inquiry:all") && !a.has("cms:inquiry:assign") {
			inquirySQL = "(" + inquirySQL + " AND t.owner_user_id=?)"
			args = append(args, a.ID)
		}
	}
	scheduleSQL := "(n.resource_type='SCHEDULE' AND s.kind IN ? AND s.owner_user_id=? AND s.status='FAILED' AND sc.is_deleted=0)"
	args = append(args, queryKinds, a.ID)
	return q.Where("("+reviewSQL+" OR "+inquirySQL+" OR "+scheduleSQL+")", args...), nil
}
func CMSNotifications(ctx context.Context, unread bool, pg page.Query) ([]model.CMSNotification, int64, error) {
	q, e := cmsNotificationScope(cmsDB(ctx))
	if e != nil {
		return nil, 0, e
	}
	if unread {
		q = q.Where("n.read_at IS NULL")
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSNotification{}
	e = q.Select("n.*").Order("n.notification_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
func CMSNotificationUnread(ctx context.Context) (int64, error) {
	q, e := cmsNotificationScope(cmsDB(ctx))
	if e != nil {
		return 0, e
	}
	var n int64
	e = q.Where("n.read_at IS NULL").Count(&n).Error
	return n, e
}
func CMSReadNotification(ctx context.Context, id, version int64) (model.CMSNotification, error) {
	var row model.CMSNotification
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		q, e := cmsNotificationScope(tx)
		if e != nil {
			return e
		}
		if e = q.Select("n.*,s.content_id AS target_content_id").Where("n.notification_id=?", id).Take(&row).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return errs.ErrNotFound
			}
			return e
		}
		if row.Version != version {
			return errs.New("消息已变化，请刷新")
		}
		row.ReadAt = types.Now()
		row.Version++
		return tx.Model(&model.CMSNotification{}).Where("notification_id=? AND recipient_user_id=? AND version=?", id, cmsActor(ctx), version).Updates(map[string]any{"read_at": row.ReadAt, "version": row.Version, "update_time": types.Now(), "update_by": "recipient"}).Error
	})
	return row, e
}
func CMSDispatchNotifications(ctx context.Context) error {
	ids := []int64{}
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		out := []model.CMSNotificationOutbox{}
		if e := tx.Where("status='PENDING' AND attempts<5").Order("outbox_id").Limit(30).Find(&out).Error; e != nil {
			return e
		}
		if len(out) == 0 {
			return nil
		}

		for _, o := range out {
			ids = append(ids, o.OutboxId)
		}
		var eligible []int64
		query := tx.Table("cms_notification_outbox o").Joins("JOIN sys_user u ON u.user_id=o.recipient_user_id AND u.status='0' AND u.del_flag='0'").Joins("JOIN cms_collaboration_setting g ON g.setting_id=1").Joins("LEFT JOIN cms_inquiry i ON o.resource_type='INQUIRY' AND i.inquiry_id=o.resource_id").Joins("LEFT JOIN cms_inquiry_team t ON t.inquiry_id=i.inquiry_id").Joins("LEFT JOIN cms_review_request r ON o.resource_type='REVIEW' AND r.review_id=o.resource_id").Joins("LEFT JOIN cms_content rc ON r.kind<>'SITE' AND rc.content_id=r.object_id").Joins("LEFT JOIN cms_schedule sc ON o.resource_type='SCHEDULE' AND sc.schedule_id=o.resource_id").Joins("LEFT JOIN cms_content cc ON cc.content_id=sc.content_id").Where("o.outbox_id IN ?", ids)
		query = query.Where(`(u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET('cms:notification:list',m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)))`).Where(`((o.resource_type='REVIEW' AND (r.kind='SITE' OR rc.is_deleted=0) AND (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(CONCAT('cms:',LOWER(o.kind),':query'),m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0))) AND (r.submitter_user_id=u.user_id OR r.author_user_id=u.user_id OR (r.status='PENDING' AND r.submitter_user_id<>u.user_id AND r.author_user_id<>u.user_id AND (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(CONCAT('cms:',LOWER(o.kind),':review'),m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)))))) OR (o.resource_type='INQUIRY' AND i.is_deleted=0 AND (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET('cms:inquiry:list',m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0))) AND (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET('cms:inquiry:query',m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0))) AND (g.inquiry_isolation=0 OR t.owner_user_id=u.user_id OR (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET('cms:inquiry:all',m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0))) OR (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET('cms:inquiry:assign',m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0)))) AND (o.event_type<>'INQUIRY_DUE' OR (i.status='NEW' AND i.version=o.business_version AND t.next_follow_at<=NOW()))) OR (o.resource_type='SCHEDULE' AND (u.user_id=1 OR EXISTS (SELECT 1 FROM sys_user_role ur JOIN sys_role r0 ON r0.role_id=ur.role_id AND r0.status='0' AND r0.del_flag='0' JOIN sys_role_menu rm ON rm.role_id=r0.role_id JOIN sys_menu m ON m.menu_id=rm.menu_id AND m.status='0' WHERE ur.user_id=u.user_id AND (FIND_IN_SET(CONCAT('cms:',LOWER(o.kind),':query'),m.perms)>0 OR FIND_IN_SET('*:*:*',m.perms)>0))) AND sc.owner_user_id=u.user_id AND sc.status='FAILED' AND cc.is_deleted=0))`)
		if e := query.Pluck("o.outbox_id", &eligible).Error; e != nil {
			return e
		}
		active := map[int64]bool{}
		for _, id := range eligible {
			active[id] = true
		}
		notices := []model.CMSNotification{}
		sent, canceled := []int64{}, []int64{}
		for _, o := range out {
			if !active[o.OutboxId] {
				canceled = append(canceled, o.OutboxId)
				continue
			}
			sent = append(sent, o.OutboxId)
			notices = append(notices, model.CMSNotification{OutboxId: o.OutboxId, RecipientUserId: o.RecipientUserId, ResourceType: o.ResourceType, ResourceId: o.ResourceId, Kind: o.Kind, EventType: o.EventType, BusinessVersion: o.BusinessVersion, Subject: o.Subject, Version: 1, ExpiresAt: types.Time(time.Now().Add(30 * 24 * time.Hour)), CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: "workflow", CreateTime: types.Now()}})
		}
		if len(notices) > 0 {
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notices).Error; e != nil {
				return e
			}
		}
		if len(sent) > 0 {
			if e := tx.Model(&model.CMSNotificationOutbox{}).Where("outbox_id IN ?", sent).Updates(map[string]any{"status": "SENT", "update_time": types.Now()}).Error; e != nil {
				return e
			}
		}
		if len(canceled) > 0 {
			return tx.Model(&model.CMSNotificationOutbox{}).Where("outbox_id IN ?", canceled).Update("status", "CANCELED").Error
		}
		return nil
	})
	if e != nil && len(ids) > 0 {
		// A failed delivery transaction leaves the outbox intact. Account attempts in a separate bounded statement.
		retryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = cmsDB(retryCtx).Model(&model.CMSNotificationOutbox{}).Where("outbox_id IN ? AND status='PENDING'", ids).Updates(map[string]any{"attempts": gorm.Expr("attempts+1"), "status": gorm.Expr("CASE WHEN attempts>=5 THEN 'FAILED' ELSE 'PENDING' END"), "update_time": types.Now()}).Error
	}
	return e
}
func CMSQueueInquiryReminders(ctx context.Context) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var rows []struct {
			InquiryId   int64
			OwnerUserId int64
			Version     int64
		}
		if e := tx.Table("cms_inquiry_team t").Select("i.inquiry_id,t.owner_user_id,i.version").Joins("JOIN cms_inquiry i ON i.inquiry_id=t.inquiry_id AND i.is_deleted=0 AND i.status='NEW'").Where("t.next_follow_at<=? AND t.reminder_sent_version<>i.version", types.Now()).Order("t.next_follow_at,t.inquiry_id").Limit(20).Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		var s model.CMSCollaborationSetting
		if e := tx.Where("setting_id=1").Take(&s).Error; e != nil {
			return e
		}
		out := []model.CMSNotificationOutbox{}
		ids := []int64{}
		for _, r := range rows {
			ids = append(ids, r.InquiryId)
			recipients := s.ManagerIds
			if r.OwnerUserId > 0 {
				recipients = []int64{r.OwnerUserId}
			}
			for _, u := range recipients {
				out = append(out, model.CMSNotificationOutbox{EventKey: fmt.Sprintf("INQUIRY:%d:INQUIRY_DUE:%d:%d", r.InquiryId, r.Version, u), RecipientUserId: u, ResourceType: "INQUIRY", ResourceId: r.InquiryId, EventType: "INQUIRY_DUE", BusinessVersion: r.Version, Subject: "一条询盘已到跟进时间", Status: "PENDING", CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: "workflow", CreateTime: types.Now()}})
			}
		}
		if len(out) > 0 {
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(out, 100).Error; e != nil {
				return e
			}
		}
		return tx.Exec("UPDATE cms_inquiry_team t JOIN cms_inquiry i ON i.inquiry_id=t.inquiry_id SET t.reminder_sent_version=i.version,t.update_time=NOW() WHERE t.inquiry_id IN ?", ids).Error
	})
}
func CMSCleanupNotifications(ctx context.Context) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		ids := []int64{}
		if e := tx.Model(&model.CMSNotification{}).Where("expires_at<=?", types.Now()).Order("expires_at,notification_id").Limit(500).Pluck("notification_id", &ids).Error; e != nil {
			return e
		}
		if len(ids) > 0 {
			if e := tx.Where("notification_id IN ?", ids).Delete(&model.CMSNotification{}).Error; e != nil {
				return e
			}
		}
		old := []int64{}
		if e := tx.Model(&model.CMSNotificationOutbox{}).Where("status<>'PENDING' AND create_time<?", time.Now().Add(-30*24*time.Hour)).Order("create_time,outbox_id").Limit(500).Pluck("outbox_id", &old).Error; e != nil {
			return e
		}
		if len(old) > 0 {
			return tx.Where("outbox_id IN ?", old).Delete(&model.CMSNotificationOutbox{}).Error
		}
		return nil
	})
}
