package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
)

// Locking reads recheck the actor and current permission, rather than a saved JWT/cache snapshot.
func cmsScheduleActor(tx *gorm.DB, id int64, kind string) (string, error) {
	var u model.SysUser
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("user_id,user_name,status,del_flag").Where("user_id=?", id).Take(&u).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errs.New("创建账号已不存在")
	} else if err != nil {
		return "", err
	}
	if u.Status != "0" || u.DelFlag != "0" {
		return "", errs.New("创建账号已停用或删除")
	}
	if id == model.AdminUserID {
		return u.UserName, nil
	}
	var rows []struct{ Perms string }
	err := tx.Table("sys_user_role ur").Select("IFNULL(m.perms,'') AS perms").Joins("JOIN sys_role r ON r.role_id=ur.role_id").Joins("JOIN sys_role_menu rm ON rm.role_id=r.role_id").Joins("JOIN sys_menu m ON m.menu_id=rm.menu_id").Where("ur.user_id=? AND r.status='0' AND m.status='0'", id).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&rows).Error
	if err != nil {
		return "", err
	}
	want := "cms:" + strings.ToLower(kind) + ":publish"
	for _, r := range rows {
		for _, p := range strings.Split(r.Perms, ",") {
			p = strings.TrimSpace(p)
			if p == want || p == model.AllPermission {
				return u.UserName, nil
			}
		}
	}
	return "", errs.New("创建账号已无发布权限")
}
func CMSCreateSchedule(ctx context.Context, row *model.CMSSchedule) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		operator, err := cmsScheduleActor(tx, row.OwnerUserId, row.Kind)
		if err != nil {
			return err
		}
		c, err := cmsFindContent(tx, row.Kind, row.ContentId)
		if err != nil {
			return err
		}
		if c.Version != row.ContentVersion {
			return errs.New("内容已变化，请刷新后创建任务")
		}
		if row.Action == "unpublish" && c.IsPublished == 0 {
			return errs.New("未发布内容不能安排定时下架")
		}
		var count int64
		if err = tx.Model(&model.CMSSchedule{}).Where("content_id=? AND status='PENDING'", row.ContentId).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errs.New("此内容已有待执行任务，请先取消")
		}
		row.Status = "PENDING"
		row.Version = 1
		row.CreateBy = operator
		row.CreateTime = types.Now()
		return tx.Create(row).Error
	})
}
func CMSSchedulePage(ctx context.Context, kind string, q model.CMSScheduleQuery, pg page.Query) ([]model.CMSSchedule, int64, error) {
	db := cmsDB(ctx).Model(&model.CMSSchedule{}).Where("kind=?", kind)
	if q.ContentId > 0 {
		db = db.Where("content_id=?", q.ContentId)
	}
	if q.Status != "" {
		db = db.Where("status=?", q.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSSchedule{}
	err := db.Order("schedule_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func CMSCancelSchedule(ctx context.Context, kind string, id, version, actor int64) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		operator, err := cmsScheduleActor(tx, actor, kind)
		if err != nil {
			return err
		}
		var row model.CMSSchedule
		if err = tx.Where("schedule_id=? AND kind=?", id, kind).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.New("定时任务不存在")
		} else if err != nil {
			return err
		}
		if actor != row.OwnerUserId && actor != model.AdminUserID {
			return errs.ErrForbidden
		}
		if row.Status != "PENDING" || row.Version != version {
			return errs.New("任务已变化或已结束，请刷新")
		}
		return tx.Model(&row).Updates(map[string]any{"status": "CANCELED", "version": row.Version + 1, "result_message": "已取消", "finished_at": types.Now(), "update_by": operator, "update_time": types.Now()}).Error
	})
}
func CMSDueSchedules(ctx context.Context, now time.Time) ([]model.CMSSchedule, error) {
	rows := []model.CMSSchedule{}
	err := cmsDB(ctx).Where("status='PENDING' AND due_at<=?", types.Time(now)).Order("due_at,schedule_id").Limit(10).Find(&rows).Error
	return rows, err
}
func CMSExecuteSchedule(ctx context.Context, snapshot model.CMSSchedule, now time.Time, preflight error, normalize func(model.CMSRevision) (string, []int64, error)) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var row model.CMSSchedule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("schedule_id=?", snapshot.ScheduleId).Take(&row).Error; err != nil {
			return err
		}
		if row.Status != "PENDING" || time.Time(row.DueAt).After(now) {
			return nil
		}
		operator, err := cmsScheduleActor(tx, row.OwnerUserId, row.Kind)
		if err == nil && now.Sub(time.Time(row.DueAt)) > 24*time.Hour {
			err = errs.New("任务错过执行时间超过24小时，请人工重新安排")
		}
		if err == nil {
			err = preflight
		}
		if err == nil {
			err = cmsChangeContentTx(tx, row.Kind, row.ContentId, model.CMSContentAction{Version: row.ContentVersion}, row.Action, operator, true, normalize)
		}
		status, message := "SUCCESS", "执行成功"
		if err != nil {
			if business := errs.As(err); business != nil {
				status, message = "FAILED", business.Msg
			} else {
				return err
			}
		}
		return tx.Model(&row).Updates(map[string]any{"status": status, "result_message": message, "finished_at": types.Time(now), "version": row.Version + 1, "update_by": operator, "update_time": types.Time(now)}).Error
	})
}
