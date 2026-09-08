package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ruoyi-go/internal/model"
)

func TestPrepareJobForCreateOwnsIDAndStatus(t *testing.T) {
	target := &model.SysJob{
		JobID:          987654,
		Status:         model.JobStatusNormal, // Vue's actual create payload.
		MisfirePolicy:  "",
		Concurrent:     "",
		JobGroup:       "",
		JobName:        "test",
		InvokeTarget:   "ryTask.ryNoParams",
		CronExpression: "0 0 3 * * ?",
	}
	prepareJobForCreate(target)

	if target.JobID != 0 {
		t.Fatalf("客户端伪造的 jobId 未被清空: %d", target.JobID)
	}
	if target.Status != model.JobStatusPause {
		t.Fatalf("新建任务必须强制暂停，实际 status=%q", target.Status)
	}
	if target.MisfirePolicy != model.JobMisfireDefault {
		t.Fatalf("misfirePolicy 默认值未对齐 Java: %q", target.MisfirePolicy)
	}
	if target.Concurrent != model.JobConcurrentForbid || target.JobGroup != "DEFAULT" {
		t.Fatalf("其余默认值不正确: group=%q concurrent=%q", target.JobGroup, target.Concurrent)
	}
}

func TestValidateJobEnums(t *testing.T) {
	valid := &model.SysJob{
		Status: model.JobStatusPause, Concurrent: model.JobConcurrentForbid,
		MisfirePolicy: model.JobMisfireDefault,
	}
	if err := validateJobEnums(valid); err != nil {
		t.Fatalf("合法枚举被拒绝: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*model.SysJob)
		want   string
	}{
		{"status", func(job *model.SysJob) { job.Status = "x" }, "状态"},
		{"concurrent", func(job *model.SysJob) { job.Concurrent = "x" }, "并发执行"},
		{"misfire", func(job *model.SysJob) { job.MisfirePolicy = "x" }, "计划策略"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := *valid
			tc.mutate(&candidate)
			if err := validateJobEnums(&candidate); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("非法枚举未被拒绝: %v", err)
			}
		})
	}
}

type jobEventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *jobEventLog) add(event string) {
	l.mu.Lock()
	l.events = append(l.events, event)
	l.mu.Unlock()
}

func (l *jobEventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

func validMutationJob() *model.SysJob {
	return &model.SysJob{
		JobID: 7, JobName: "test", JobGroup: "DEFAULT",
		InvokeTarget: "ryTask.ryNoParams", CronExpression: "0 0 3 * * ?",
		Status: model.JobStatusNormal, Concurrent: model.JobConcurrentForbid,
		MisfirePolicy: model.JobMisfireDefault,
	}
}

func TestJobUpdateAndDeleteAreOneOrderedMutation(t *testing.T) {
	var log jobEventLog
	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	deleteEntered := make(chan struct{})
	ops := jobMutationOps{
		selectByID: func(context.Context, int64) (*model.SysJob, error) {
			log.add("select")
			return validMutationJob(), nil
		},
		update: func(context.Context, *model.SysJob) error {
			log.add("db:update")
			close(updateEntered)
			<-releaseUpdate
			return nil
		},
		delete: func(context.Context, []int64) error {
			log.add("db:delete")
			close(deleteEntered)
			return nil
		},
		reschedule: func(*model.SysJob) error { log.add("scheduler:update"); return nil },
		remove:     func(int64) { log.add("scheduler:delete") },
	}

	updateDone := make(chan error, 1)
	go func() { updateDone <- updateJobWithOps(context.Background(), validMutationJob(), "tester", ops) }()
	<-updateEntered
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- deleteJobsWithOps(context.Background(), []int64{7}, ops) }()
	select {
	case <-deleteEntered:
		t.Fatal("Update 尚未更新 scheduler 时 Delete 已进入数据库写入")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseUpdate)
	if err := <-updateDone; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"select", "db:update", "scheduler:update", "db:delete", "scheduler:delete"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("调用顺序=%v，期望 %v", got, want)
	}
}

func TestJobChangeStatusWaitsForUpdateScheduler(t *testing.T) {
	var log jobEventLog
	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	ops := jobMutationOps{
		selectByID: func(context.Context, int64) (*model.SysJob, error) {
			log.add("select")
			return validMutationJob(), nil
		},
		update: func(context.Context, *model.SysJob) error {
			log.add("db:update")
			close(updateEntered)
			<-releaseUpdate
			return nil
		},
		updateStatus: func(context.Context, int64, string, string) error {
			log.add("db:status")
			return nil
		},
		reschedule: func(*model.SysJob) error { log.add("scheduler"); return nil },
	}
	updateDone := make(chan error, 1)
	go func() { updateDone <- updateJobWithOps(context.Background(), validMutationJob(), "tester", ops) }()
	<-updateEntered
	statusDone := make(chan error, 1)
	statusStarted := make(chan struct{})
	go func() {
		close(statusStarted)
		statusDone <- changeJobStatusWithOps(context.Background(), 7, model.JobStatusPause, "tester", ops)
	}()
	<-statusStarted
	time.Sleep(30 * time.Millisecond)
	if got := log.snapshot(); !slices.Equal(got, []string{"select", "db:update"}) {
		t.Fatalf("Update 未完成时 ChangeStatus 已进入操作层: %v", got)
	}
	close(releaseUpdate)
	if err := <-updateDone; err != nil {
		t.Fatal(err)
	}
	if err := <-statusDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"select", "db:update", "scheduler", "select", "db:status", "scheduler"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("调用顺序=%v，期望 %v", got, want)
	}
}

func TestJobRunOnceAndDeleteAreSerialized(t *testing.T) {
	var log jobEventLog
	submitEntered := make(chan struct{})
	releaseSubmit := make(chan struct{})
	deleteEntered := make(chan struct{})
	ops := jobMutationOps{
		selectByID: func(context.Context, int64) (*model.SysJob, error) { return validMutationJob(), nil },
		resolve:    func(string) error { return nil },
		submit: func(func()) bool {
			log.add("submit")
			close(submitEntered)
			<-releaseSubmit
			return true
		},
		execute: func(*model.SysJob) {},
		delete: func(context.Context, []int64) error {
			log.add("db:delete")
			close(deleteEntered)
			return nil
		},
		remove: func(int64) { log.add("scheduler:delete") },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- runJobOnceWithOps(context.Background(), 7, ops) }()
	<-submitEntered
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- deleteJobsWithOps(context.Background(), []int64{7}, ops) }()
	select {
	case <-deleteEntered:
		t.Fatal("RunOnce 尚未入队时 Delete 已写数据库")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseSubmit)
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	want := []string{"submit", "db:delete", "scheduler:delete"}
	if got := log.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("调用顺序=%v，期望 %v", got, want)
	}
}
