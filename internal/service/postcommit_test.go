package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunPostCommitSurvivesParentCancellation(t *testing.T) {
	type traceKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), traceKey{}, "trace-1"))
	cancel()

	err := runPostCommit(parent, func(ctx context.Context) error {
		if ctx.Err() != nil {
			t.Fatalf("提交后 context 不应继承请求取消: %v", ctx.Err())
		}
		if got := ctx.Value(traceKey{}); got != "trace-1" {
			t.Fatalf("提交后 context 丢失 trace 值: %v", got)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > postCommitTimeout {
			t.Fatalf("提交后 context deadline 不正确: ok=%v deadline=%v", ok, deadline)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("提交后任务失败: %v", err)
	}
}

func TestRunPostCommitReturnsCallbackError(t *testing.T) {
	want := errors.New("post-commit failure")
	if got := runPostCommit(context.Background(), func(context.Context) error { return want }); !errors.Is(got, want) {
		t.Fatalf("回调错误未向上传递: %v", got)
	}
}
