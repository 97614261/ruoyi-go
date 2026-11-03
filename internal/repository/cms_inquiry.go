package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// This shared scope covers the original endpoints, tasks, followups, and workbench.
func cmsInquiryScope(tx *gorm.DB) (*gorm.DB, error) {
	q := tx.Table("cms_inquiry i").Joins("LEFT JOIN cms_inquiry_team t ON t.inquiry_id=i.inquiry_id").Where("i.is_deleted=0")
	var setting model.CMSCollaborationSetting
	if e := tx.Where("setting_id=1").Take(&setting).Error; e != nil {
		return nil, e
	}
	if !setting.InquiryIsolation {
		return q, nil
	}
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return nil, e
	}
	if !a.has("cms:inquiry:all") && !a.has("cms:inquiry:assign") {
		q = q.Where("t.owner_user_id=?", a.ID)
	}
	return q, nil
}
func CMSInsertInquiry(ctx context.Context, in model.CMSInquiryInput) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		row := model.CMSInquiry{Name: in.Name, Contact: in.Contact, Message: in.Message, Status: "NEW", Version: 1, CreateBy: "visitor", CreateTime: types.Now()}
		if e := tx.Create(&row).Error; e != nil {
			return e
		}
		if e := tx.Create(&model.CMSInquiryTeam{InquiryId: row.InquiryId, Version: 1, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: "visitor", CreateTime: types.Now()}}).Error; e != nil {
			return e
		}
		var setting model.CMSCollaborationSetting
		if e := tx.Where("setting_id=1").Take(&setting).Error; e != nil {
			return e
		}
		if e := cmsSyncInquiryClock(tx, row.InquiryId); e != nil {
			return e
		}
		return cmsEnqueue(tx, "INQUIRY", row.InquiryId, "", "INQUIRY_NEW", row.Version, setting.ManagerIds, "收到新的询盘留言")
	})
}

const inquiryMasked = "i.inquiry_id,i.name,CASE WHEN CHAR_LENGTH(i.contact)>4 THEN CONCAT(LEFT(i.contact,2),'***',RIGHT(i.contact,2)) ELSE '***' END AS contact,i.status,i.version,i.create_time"

func CMSInquiryPage(ctx context.Context, in model.CMSInquiryQuery, pg page.Query) ([]model.CMSInquirySummary, int64, error) {
	q, e := cmsInquiryScope(cmsDB(ctx))
	if e != nil {
		return nil, 0, e
	}
	if in.Status != "" {
		q = q.Where("i.status=?", in.Status)
	}
	var n int64
	if e = q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSInquirySummary{}
	e = q.Select(inquiryMasked).Order("i.inquiry_id DESC").Limit(pg.PageSize).Offset(pg.Offset()).Scan(&rows).Error
	return rows, n, e
}
func cmsInquiryVisible(tx *gorm.DB, id int64) (model.CMSInquiry, error) {
	var row model.CMSInquiry
	q, e := cmsInquiryScope(tx)
	if e != nil {
		return row, e
	}
	e = q.Select("i.*").Where("i.inquiry_id=?", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return row, errs.ErrNotFound
	}
	return row, e
}
func CMSInquiryDetail(ctx context.Context, id int64) (*model.CMSInquiry, error) {
	r, e := cmsInquiryVisible(cmsDB(ctx), id)
	return &r, e
}
func CMSUpdateInquiry(ctx context.Context, id int64, in model.CMSInquiryUpdate, operator string, erase bool) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		row, e := cmsInquiryVisible(tx, id)
		if e != nil {
			return e
		}
		if row.Version != in.Version {
			return errs.New("询盘已变化，请刷新")
		}
		changes := map[string]any{"status": in.Status, "note": in.Note, "version": in.Version + 1, "update_by": operator, "update_time": types.Now()}
		if erase {
			changes["is_deleted"] = 1
			changes["name"] = ""
			changes["contact"] = ""
			changes["message"] = ""
			changes["note"] = ""
		}
		if e = tx.Model(&model.CMSInquiry{}).Where("inquiry_id=? AND version=? AND is_deleted=0", id, in.Version).Updates(changes).Error; e != nil {
			return e
		}
		if e = cmsSyncInquiryClock(tx, id); e != nil {
			return e
		}
		teamChanges := map[string]any{"version": in.Version + 1, "reminder_sent_version": 0, "update_by": operator, "update_time": types.Now()}
		if erase || in.Status != "NEW" {
			teamChanges["next_follow_at"] = nil
		}
		if e = tx.Model(&model.CMSInquiryTeam{}).Where("inquiry_id=?", id).Updates(teamChanges).Error; e != nil {
			return e
		}
		if e = cmsCancelInquiryDue(tx, id); e != nil {
			return e
		}
		if erase {
			if e = tx.Model(&model.CMSInquiryFollowup{}).Where("inquiry_id=?", id).Updates(map[string]any{"text": "", "result": "", "is_erased": 1, "update_by": operator, "update_time": types.Now()}).Error; e != nil {
				return e
			}
			if e = tx.Model(&model.CMSNotificationOutbox{}).Where("resource_type='INQUIRY' AND resource_id=? AND status='PENDING'", id).Update("status", "CANCELED").Error; e != nil {
				return e
			}
			return tx.Model(&model.CMSNotification{}).Where("resource_type='INQUIRY' AND resource_id=?", id).Update("expires_at", gorm.Expr("NOW()")).Error
		}
		return nil
	})
}
