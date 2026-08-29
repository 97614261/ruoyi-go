package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

func jobListDB(ctx context.Context, query model.JobQuery) *gorm.DB {
	db := DB(ctx).Model(&model.SysJob{})

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
	return db
}

// SelectJobPage 分页查询定时任务。
func SelectJobPage(ctx context.Context, query model.JobQuery, pg page.Query) ([]model.SysJob, int64, error) {
	var total int64
	if err := jobListDB(ctx, query).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计定时任务总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysJob{}, 0, nil
	}

	orderBy := pg.Stable("job_id", "job_id")

	var list []model.SysJob
	err := jobListDB(ctx, query).
		Order(orderBy).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询定时任务列表失败: %w", err)
	}
	return list, total, nil
}

// SelectJobList 不分页查询，供导出使用。
func SelectJobList(ctx context.Context, query model.JobQuery, limit ...int) ([]model.SysJob, error) {
	var list []model.SysJob
	if err := applyOptionalLimit(jobListDB(ctx, query), limit).Order("job_id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询定时任务列表失败: %w", err)
	}
	return list, nil
}

// SelectJobAll 查全部任务，服务启动时重建调度用。
func SelectJobAll(ctx context.Context) ([]model.SysJob, error) {
	var list []model.SysJob
	if err := DB(ctx).Order("job_id").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询全部定时任务失败: %w", err)
	}
	return list, nil
}

// SelectJobByID 按主键查任务，不存在返回 (nil, nil)。
func SelectJobByID(ctx context.Context, jobID int64) (*model.SysJob, error) {
	var job model.SysJob
	err := DB(ctx).Where("job_id = ?", jobID).Take(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询定时任务 %d 失败: %w", jobID, err)
	}
	return &job, nil
}

// InsertJob 新增任务。
func InsertJob(ctx context.Context, job *model.SysJob) error {
	if err := DB(ctx).Create(job).Error; err != nil {
		return fmt.Errorf("新增定时任务失败: %w", err)
	}
	return nil
}

// UpdateJob 修改任务。
//
// 显式列出可改字段：用 Updates(struct) 会跳过零值，
// 「把并发从禁止改成允许」这种恰好是零值的改动会静默失效。
func UpdateJob(ctx context.Context, job *model.SysJob) error {
	err := DB(ctx).Model(&model.SysJob{}).
		Where("job_id = ?", job.JobID).
		Select("job_name", "job_group", "invoke_target", "cron_expression",
			"misfire_policy", "concurrent", "status", "remark", "update_by", "update_time").
		Updates(job).Error
	if err != nil {
		return fmt.Errorf("修改定时任务 %d 失败: %w", job.JobID, err)
	}
	return nil
}

// UpdateJobStatus 只改状态，供暂停/启用使用。
func UpdateJobStatus(ctx context.Context, jobID int64, status, operator string) error {
	err := DB(ctx).Model(&model.SysJob{}).
		Where("job_id = ?", jobID).
		Updates(map[string]any{
			"status":      status,
			"update_by":   operator,
			"update_time": time.Now(),
		}).Error
	if err != nil {
		return fmt.Errorf("修改定时任务 %d 状态失败: %w", jobID, err)
	}
	return nil
}

// DeleteJobByIDs 物理删除任务。
//
// sys_job 没有 del_flag 列，与 Java 版一致是真删。
func DeleteJobByIDs(ctx context.Context, jobIDs []int64) error {
	if len(jobIDs) == 0 {
		return nil
	}
	if err := DB(ctx).Where("job_id IN ?", jobIDs).Delete(&model.SysJob{}).Error; err != nil {
		return fmt.Errorf("删除定时任务失败: %w", err)
	}
	return nil
}
