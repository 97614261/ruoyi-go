package service

import (
	"context"
	"time"

	"ruoyi-go/internal/job"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/asyncx"
	"ruoyi-go/pkg/cronx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

var manualJobPool = asyncx.NewPool(4, 64)

// ListJobPage 分页查询定时任务。
func ListJobPage(ctx context.Context, query model.JobQuery, pg page.Query) ([]model.SysJob, int64, error) {
	list, total, err := repository.SelectJobPage(ctx, query, pg)
	if err != nil {
		return nil, 0, err
	}
	fillNextValidTime(list)
	return list, total, nil
}

// ListJobExport 导出用的全量查询。
func ListJobExport(ctx context.Context, query model.JobQuery) ([]model.SysJob, error) {
	list, err := repository.SelectJobList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// GetJob 查任务详情。
func GetJob(ctx context.Context, jobID int64) (*model.SysJob, error) {
	target, err := repository.SelectJobByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errs.New("定时任务不存在")
	}
	target.NextValidTime = nextValidTime(target)
	return target, nil
}

// CreateJob 新增任务。
func CreateJob(ctx context.Context, target *model.SysJob, operator string) error {
	if err := validateJob(target); err != nil {
		return err
	}

	applyJobDefaults(target)
	target.CreateBy = operator
	target.CreateTime = types.Time(time.Now())

	if err := repository.InsertJob(ctx, target); err != nil {
		return err
	}
	// 装载失败不回滚：记录已经存进去了，用户在界面上能看到、能改。
	// 直接报错反而让他以为没保存成功，回头再建一条就重复了。
	if err := scheduler.reschedule(target); err != nil {
		return errs.Newf("任务已保存，但装载调度失败：%s", err.Error())
	}
	return nil
}

// UpdateJob 修改任务。
func UpdateJob(ctx context.Context, target *model.SysJob, operator string) error {
	existing, err := repository.SelectJobByID(ctx, target.JobID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("定时任务不存在")
	}
	if err := validateJob(target); err != nil {
		return err
	}

	applyJobDefaults(target)
	target.UpdateBy = operator
	target.UpdateTime = types.Time(time.Now())

	if err := repository.UpdateJob(ctx, target); err != nil {
		return err
	}
	if err := scheduler.reschedule(target); err != nil {
		return errs.Newf("任务已保存，但重新装载调度失败：%s", err.Error())
	}
	return nil
}

// ChangeJobStatus 暂停 / 启用。
func ChangeJobStatus(ctx context.Context, jobID int64, status, operator string) error {
	if status == "" {
		return errs.New("状态不能为空")
	}
	target, err := repository.SelectJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	if target == nil {
		return errs.New("定时任务不存在")
	}

	target.Status = status
	if err := repository.UpdateJobStatus(ctx, jobID, status, operator); err != nil {
		return err
	}
	if err := scheduler.reschedule(target); err != nil {
		return errs.Newf("状态已修改，但重新装载调度失败：%s", err.Error())
	}
	return nil
}

// RunJobOnce 立即执行一次。
//
// 【不等它跑完】任务可能要跑几分钟，同步等会把 HTTP 请求挂死。
// 前端点完立刻返回成功，结果去调度日志里看 —— 与 Java 版行为一致。
func RunJobOnce(ctx context.Context, jobID int64) error {
	target, err := repository.SelectJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	if target == nil {
		return errs.New("任务不存在或已过期！")
	}
	// 先验一遍，别让用户点完看着"成功"、实际连任务都没找到
	if _, _, err := job.Resolve(target.InvokeTarget); err != nil {
		return errs.New(err.Error())
	}

	snapshot := *target
	if !manualJobPool.Submit(func() { execute(&snapshot) }) {
		return errs.New("任务执行队列繁忙，请稍后再试")
	}
	return nil
}

// DeleteJobs 删除任务，同时撤下调度。
func DeleteJobs(ctx context.Context, jobIDs []int64) error {
	if len(jobIDs) == 0 {
		return errs.New("请选择要删除的任务")
	}
	if err := repository.DeleteJobByIDs(ctx, jobIDs); err != nil {
		return err
	}
	for _, id := range jobIDs {
		scheduler.remove(id)
	}
	return nil
}

// validateJob 校验 cron 表达式和调用目标。
//
// 【调用目标只查注册表】这既是白名单也是全部的安全边界。
// 不需要 Java 那套黑名单（java.net.URL / InitialContext / rmi: / ldap: …）——
// 那是给反射调用擦屁股的，注册表压根没有"调到别的东西"这种可能。
func validateJob(target *model.SysJob) error {
	if _, err := cronx.Parse(target.CronExpression); err != nil {
		return errs.Newf("任务'%s'的%s", target.JobName, err.Error())
	}
	if _, _, err := job.Resolve(target.InvokeTarget); err != nil {
		return errs.Newf("任务'%s'失败，%s", target.JobName, err.Error())
	}
	return nil
}

// applyJobDefaults 补上三个有 DEFAULT 的列。
//
// 模型上没给它们加 required（列有默认值），但 GORM 插入时会把空串写进去，
// 拿到的就是空的 status —— 前端的状态开关会显示成未知状态。
func applyJobDefaults(target *model.SysJob) {
	if target.JobGroup == "" {
		target.JobGroup = "DEFAULT"
	}
	if target.Status == "" {
		target.Status = model.JobStatusPause
	}
	if target.Concurrent == "" {
		target.Concurrent = model.JobConcurrentForbid
	}
	if target.MisfirePolicy == "" {
		target.MisfirePolicy = model.JobMisfireAbandon
	}
}

func fillNextValidTime(list []model.SysJob) {
	for i := range list {
		list[i].NextValidTime = nextValidTime(&list[i])
	}
}

// nextValidTime 算下次执行时间。
//
// 不管任务是不是暂停都算 —— 与 Java 的 getNextValidTime() 一致，
// 它只看 cron 表达式，前端展示的也是"这个表达式下次会在什么时候触发"。
func nextValidTime(target *model.SysJob) types.Time {
	next := cronx.Next(target.CronExpression, time.Now().In(types.Location))
	if next.IsZero() {
		return types.Time{}
	}
	return types.Time(next)
}
