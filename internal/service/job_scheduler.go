package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"ruoyi-go/internal/job"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/cronx"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/types"
)

// JobExecuteTimeout 单次任务执行的超时。
//
// 必须有 —— 没有超时的话，一个卡住的任务会一直占着 goroutine，
// 而且「禁止并发」的任务从此再也不会被触发。
// 给得比较宽松：定时任务本来就常有跑几分钟的。
const JobExecuteTimeout = 30 * time.Minute

// maxExceptionInfo sys_job_log.exception_info 是 varchar(2000)。
// 不截断的话超长报错会让整条日志写不进去 —— 任务失败的同时连失败原因都丢了。
const maxExceptionInfo = 2000

// jobScheduler 定时任务调度器。
//
// 【单机内存调度，多实例会重复执行】
// 和 Java 版的默认配置一致：RuoYi 的 ScheduleConfig 整个是注释掉的，
// QRTZ_* 表在单独的 quartz.sql 里、本项目用的 ry_20260417.sql 没有它们。
// 所以两边都是进程内调度，重启后从 sys_job 全量重建。
// 要多实例部署就得自己加分布式锁，那是另一个决定。
type jobScheduler struct {
	cron *cron.Cron

	mu sync.Mutex
	// entries 任务 ID -> cron 条目 ID，用于增删改时定位
	entries map[int64]cron.EntryID
	// running 正在执行的任务 ID，实现「禁止并发」
	running map[int64]bool
}

// scheduler 包级单例。
//
// 【cron 实例在这里就建好，不能等到 StartScheduler】
// 否则没调 StartScheduler 的场景（比如接口测试只装路由）一新增任务就空指针。
// 没 Start 过的 cron 可以正常 AddFunc，只是不会触发 —— 正是想要的行为。
var scheduler = &jobScheduler{
	cron: cron.New(
		cron.WithParser(cronx.Parser),
		cron.WithLocation(types.Location),
	),
	entries: make(map[int64]cron.EntryID),
	running: make(map[int64]bool),
}

// StartScheduler 装载数据库里状态正常的任务并开始调度。
//
// 必须在 repository 初始化之后调用。
func StartScheduler(ctx context.Context) error {
	jobs, err := repository.SelectJobAll(ctx)
	if err != nil {
		return err
	}

	loaded, skipped := 0, 0
	for i := range jobs {
		current := jobs[i]
		if current.Status != model.JobStatusNormal {
			continue
		}
		if err := scheduler.add(&current); err != nil {
			// 单个任务配错不能拖垮整个服务启动 —— 记日志跳过，
			// 用户在界面上还能看到它、改好再启用
			slog.WarnContext(ctx, "定时任务装载失败，已跳过",
				"jobId", current.JobID, "jobName", current.JobName, "err", err)
			skipped++
			continue
		}
		loaded++
	}

	scheduler.cron.Start()
	slog.InfoContext(ctx, "定时任务调度器已启动", "已装载", loaded, "已跳过", skipped)
	return nil
}

// StopScheduler 停止调度，等待正在执行的任务结束。
func StopScheduler(ctx context.Context) {
	stopped := scheduler.cron.Stop()

	select {
	case <-stopped.Done():
		slog.InfoContext(ctx, "定时任务调度器已停止")
	case <-ctx.Done():
		// 优雅关闭有总超时，不能被一个长任务无限拖住
		slog.WarnContext(ctx, "等待定时任务结束超时，强制退出")
	}
}

// add 把任务加入调度。调用方保证 status 为正常。
func (s *jobScheduler) add(target *model.SysJob) error {
	if _, _, err := job.Resolve(target.InvokeTarget); err != nil {
		return err
	}
	translated, err := cronx.Translate(target.CronExpression)
	if err != nil {
		return err
	}

	snapshot := *target
	entryID, err := s.cron.AddFunc(translated, func() {
		execute(&snapshot)
	})
	if err != nil {
		return fmt.Errorf("cron 表达式 %q 不正确", target.CronExpression)
	}

	s.mu.Lock()
	s.entries[target.JobID] = entryID
	s.mu.Unlock()
	return nil
}

// remove 把任务移出调度。不存在时静默返回。
func (s *jobScheduler) remove(jobID int64) {
	s.mu.Lock()
	entryID, ok := s.entries[jobID]
	delete(s.entries, jobID)
	s.mu.Unlock()

	if ok {
		s.cron.Remove(entryID)
	}
}

// reschedule 先移除再按当前状态重新加入。改任务、改状态都走它。
func (s *jobScheduler) reschedule(target *model.SysJob) error {
	s.remove(target.JobID)
	if target.Status != model.JobStatusNormal {
		return nil
	}
	return s.add(target)
}

// beginRun 标记任务开始执行；返回 false 表示上一次还没跑完且禁止并发。
func (s *jobScheduler) beginRun(target *model.SysJob) bool {
	if target.Concurrent == model.JobConcurrentAllow {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[target.JobID] {
		return false
	}
	s.running[target.JobID] = true
	return true
}

func (s *jobScheduler) endRun(target *model.SysJob) {
	if target.Concurrent == model.JobConcurrentAllow {
		return
	}
	s.mu.Lock()
	delete(s.running, target.JobID)
	s.mu.Unlock()
}

// execute 执行一次任务并落一条调度日志。
//
// 【绝不能 panic 出去】任务代码是业务写的，panic 会顺着 cron 的 goroutine
// 把整个进程带走。这里必须自己 recover。
func execute(target *model.SysJob) {
	// 定时触发没有请求上下文，自己造一个带 traceId 的，
	// 这样任务内部打的日志也能和这次执行串起来
	ctx, cancel := context.WithTimeout(
		logx.WithTraceID(context.Background(), logx.NewTraceID()), JobExecuteTimeout)
	defer cancel()

	if !scheduler.beginRun(target) {
		slog.WarnContext(ctx, "上一次尚未执行完毕且配置为禁止并发，本次跳过",
			"jobId", target.JobID, "jobName", target.JobName)
		return
	}
	defer scheduler.endRun(target)

	start := time.Now().In(types.Location)
	record := model.SysJobLog{
		JobName:      target.JobName,
		JobGroup:     target.JobGroup,
		InvokeTarget: target.InvokeTarget,
		Status:       model.JobLogStatusNormal,
		StartTime:    types.Time(start),
	}

	err := runTask(ctx, target)

	end := time.Now().In(types.Location)
	record.EndTime = types.Time(end)
	record.CreateTime = types.Time(end)
	cost := end.Sub(start)

	if err != nil {
		record.Status = model.JobLogStatusFail
		record.JobMessage = fmt.Sprintf("%s 执行失败，耗时 %s", target.JobName, cost.Round(time.Millisecond))
		record.ExceptionInfo = truncateRunes(err.Error(), maxExceptionInfo)
		slog.ErrorContext(ctx, "定时任务执行失败",
			"jobId", target.JobID, "jobName", target.JobName, "costMs", cost.Milliseconds(), "err", err)
	} else {
		record.JobMessage = fmt.Sprintf("%s 执行成功，耗时 %s", target.JobName, cost.Round(time.Millisecond))
		slog.InfoContext(ctx, "定时任务执行成功",
			"jobId", target.JobID, "jobName", target.JobName, "costMs", cost.Milliseconds())
	}

	// 写日志用独立的短超时 ctx：任务本身可能已经把 ctx 耗到超时了，
	// 用同一个 ctx 会导致失败记录写不进去 —— 恰恰是最需要留痕的那次
	logCtx, logCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer logCancel()
	if err := repository.InsertJobLog(logCtx, &record); err != nil {
		slog.ErrorContext(ctx, "写入调度日志失败", "jobId", target.JobID, "err", err)
	}
}

// runTask 查注册表并调用，把 panic 转成 error。
func runTask(ctx context.Context, target *model.SysJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("任务执行时发生 panic: %v", r)
		}
	}()

	task, args, err := job.Resolve(target.InvokeTarget)
	if err != nil {
		return err
	}
	return task(ctx, args)
}

// truncateRunes 按字符截断，避免切断多字节字符产生非法 UTF-8。
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	for len(string(runes)) > max {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}
