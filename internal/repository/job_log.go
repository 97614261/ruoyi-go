package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

func jobLogListDB(ctx context.Context, query model.JobLogQuery) *gorm.DB {
	db := DB(ctx).Model(&model.SysJobLog{})

	if query.JobLogID != 0 {
		db = db.Where("job_log_id = ?", query.JobLogID)
	}
	if query.JobName != "" {
		db = db.Where("job_name LIKE ?", "%"+query.JobName+"%")
	}
	if query.JobGroup != "" {
		db = db.Where("job_group = ?", query.JobGroup)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.InvokeTarget != "" {
		db = db.Where("invoke_target LIKE ?", "%"+query.InvokeTarget+"%")
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(create_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	return db
}

// SelectJobLogPage 分页查询调度日志。
func SelectJobLogPage(ctx context.Context, query model.JobLogQuery, pg page.Query) ([]model.SysJobLog, int64, error) {
	var total int64
	if err := jobLogListDB(ctx, query).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计调度日志总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysJobLog{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		// 默认倒序：日志页面永远是最新的最有用
		orderBy = "job_log_id DESC"
	}

	var list []model.SysJobLog
	err := jobLogListDB(ctx, query).
		Order(orderBy).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询调度日志列表失败: %w", err)
	}
	return list, total, nil
}

// SelectJobLogList 不分页查询，供导出使用。
func SelectJobLogList(ctx context.Context, query model.JobLogQuery) ([]model.SysJobLog, error) {
	var list []model.SysJobLog
	if err := jobLogListDB(ctx, query).Order("job_log_id DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询调度日志列表失败: %w", err)
	}
	return list, nil
}

// SelectJobLogByID 按主键查日志，不存在返回 (nil, nil)。
func SelectJobLogByID(ctx context.Context, jobLogID int64) (*model.SysJobLog, error) {
	var log model.SysJobLog
	err := DB(ctx).Where("job_log_id = ?", jobLogID).Take(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询调度日志 %d 失败: %w", jobLogID, err)
	}
	return &log, nil
}

// InsertJobLog 写一条执行记录。
func InsertJobLog(ctx context.Context, log *model.SysJobLog) error {
	if err := DB(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入调度日志失败: %w", err)
	}
	return nil
}

// DeleteJobLogByIDs 删除若干条日志。
func DeleteJobLogByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if err := DB(ctx).Where("job_log_id IN ?", ids).Delete(&model.SysJobLog{}).Error; err != nil {
		return fmt.Errorf("删除调度日志失败: %w", err)
	}
	return nil
}

// CleanJobLog 清空调度日志。
//
// 用 TRUNCATE 而不是 DELETE：日志表可能几十万行，DELETE 会写满 binlog
// 和 undo，还会把自增 ID 留在原地。与 Java 版 cleanJobLog 的语义一致。
func CleanJobLog(ctx context.Context) error {
	if err := DB(ctx).Exec("TRUNCATE TABLE sys_job_log").Error; err != nil {
		return fmt.Errorf("清空调度日志失败: %w", err)
	}
	return nil
}
