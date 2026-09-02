package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/cronx"
)

func TestJobSchedulerConcurrentRescheduleKeepsSingleEntry(t *testing.T) {
	s := newTestJobScheduler()
	const (
		jobID   = int64(42)
		workers = 32
		rounds  = 20
	)

	for round := 0; round < rounds; round++ {
		start := make(chan struct{})
		errs := make([]error, workers)
		var wg sync.WaitGroup
		for i := range errs {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				target := testScheduledJob(jobID)
				if index%2 == 1 {
					target.CronExpression = "1/10 * * * * ?"
				}
				errs[index] = s.reschedule(&target)
			}(i)
		}
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("第 %d 轮第 %d 个并发重排失败：%v", round+1, i+1, err)
			}
		}
		entries := s.cron.Entries()
		if len(entries) != 1 || len(s.entries) != 1 {
			t.Fatalf("第 %d 轮重排后应只有一个条目，cron=%d map=%d",
				round+1, len(entries), len(s.entries))
		}
		trackedID, ok := s.entries[jobID]
		if !ok || entries[0].ID != trackedID {
			t.Fatalf("第 %d 轮 entries 必须指向唯一 cron 条目，tracked=%d cron=%d",
				round+1, trackedID, entries[0].ID)
		}

		s.remove(jobID)
		if len(s.cron.Entries()) != 0 || len(s.entries) != 0 {
			t.Fatalf("第 %d 轮删除后不应残留幽灵条目，cron=%d map=%d",
				round+1, len(s.cron.Entries()), len(s.entries))
		}
	}
}

func TestJobSchedulerConcurrentPauseRemovesEntry(t *testing.T) {
	s := newTestJobScheduler()
	target := testScheduledJob(43)
	if err := s.reschedule(&target); err != nil {
		t.Fatalf("准备已调度任务失败：%v", err)
	}

	target.Status = model.JobStatusPause
	const workers = 16
	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			errs[index] = s.reschedule(&target)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 个并发暂停失败：%v", i+1, err)
		}
	}
	if len(s.cron.Entries()) != 0 || len(s.entries) != 0 {
		t.Fatalf("并发暂停后不应残留条目，cron=%d map=%d",
			len(s.cron.Entries()), len(s.entries))
	}
}

func TestJobSchedulerScheduleAndRunLocksAreIndependent(t *testing.T) {
	s := newTestJobScheduler()
	target := testScheduledJob(44)

	s.scheduleMu.Lock()
	runResult := make(chan bool, 1)
	go func() { runResult <- s.beginRun(&target) }()
	select {
	case started := <-runResult:
		if !started {
			t.Fatal("首次运行应成功登记")
		}
	case <-time.After(time.Second):
		s.scheduleMu.Unlock()
		t.Fatal("运行状态不应等待调度变更锁")
	}
	s.endRun(&target)
	s.scheduleMu.Unlock()

	s.runMu.Lock()
	scheduleResult := make(chan error, 1)
	go func() { scheduleResult <- s.reschedule(&target) }()
	select {
	case err := <-scheduleResult:
		if err != nil {
			s.runMu.Unlock()
			t.Fatalf("登记调度失败：%v", err)
		}
	case <-time.After(time.Second):
		s.runMu.Unlock()
		t.Fatal("调度变更不应等待任务运行锁")
	}
	s.runMu.Unlock()
}

func TestTimedOutTaskKeepsForbidFlagUntilTaskActuallyExits(t *testing.T) {
	s := newTestJobScheduler()
	target := testScheduledJob(45)
	started := make(chan struct{})
	release := make(chan struct{})

	if !s.beginRun(&target) {
		t.Fatal("首次运行应成功登记")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := s.runStartedTaskWith(ctx, &target, func(context.Context, *model.SysJob) error {
		close(started)
		<-release // 模拟完全忽略 context 的任务
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时任务应返回 DeadlineExceeded，实际 %v", err)
	}
	<-started
	if s.beginRun(&target) {
		s.endRun(&target)
		t.Fatal("任务函数尚未退出时不能释放禁止并发标记")
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.beginRun(&target) {
			s.endRun(&target)
			if !s.waitForRuns(context.Background()) {
				t.Fatal("任务退出后 runWG 应完成")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("任务函数退出后禁止并发标记没有释放")
}

func TestTruncateUTF8Bytes(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want string
	}{
		{name: "ASCII 未超限", text: "failure", max: 7, want: "failure"},
		{name: "ASCII 截断", text: "failure", max: 4, want: "fail"},
		{name: "中文完整字符", text: "任务失败", max: 7, want: "任务"},
		{name: "中文精确边界", text: "任务失败", max: 6, want: "任务"},
		{name: "emoji 不切断", text: "A😀B", max: 5, want: "A😀"},
		{name: "零上限", text: "failure", max: 0, want: ""},
		{name: "负上限", text: "failure", max: -1, want: ""},
		{name: "非法 UTF-8 被替换", text: string([]byte{'A', 0xff, 'B'}), max: 5, want: "A�B"},
		{name: "替换字符也服从上限", text: string([]byte{'A', 0xff, 'B'}), max: 3, want: "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8Bytes(tt.text, tt.max)
			if got != tt.want {
				t.Fatalf("truncateUTF8Bytes(%q, %d) = %q，期望 %q", tt.text, tt.max, got, tt.want)
			}
			if len(got) > max(tt.max, 0) {
				t.Fatalf("结果长度 %d 超过上限 %d", len(got), tt.max)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("结果不是合法 UTF-8：%x", []byte(got))
			}
		})
	}
}

func TestTruncateUTF8BytesLargeInput(t *testing.T) {
	input := strings.Repeat("任务失败😀", 200_000)
	got := truncateUTF8Bytes(input, maxExceptionInfo)
	if len(got) > maxExceptionInfo {
		t.Fatalf("结果长度 %d 超过上限 %d", len(got), maxExceptionInfo)
	}
	if !utf8.ValidString(got) {
		t.Fatal("长输入截断后不是合法 UTF-8")
	}
}

func newTestJobScheduler() *jobScheduler {
	return &jobScheduler{
		cron:    cron.New(cron.WithParser(cronx.Parser)),
		entries: make(map[int64]cron.EntryID),
		running: make(map[int64]bool),
	}
}

func testScheduledJob(jobID int64) model.SysJob {
	return model.SysJob{
		JobID:          jobID,
		JobName:        "scheduler-test",
		InvokeTarget:   "ryTask.ryNoParams",
		CronExpression: "0/10 * * * * ?",
		Status:         model.JobStatusNormal,
		Concurrent:     model.JobConcurrentForbid,
	}
}
