package service

import (
	"context"
	"ruoyi-go/pkg/types"
	"testing"
	"time"
)

func TestCMSScheduleTime(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, types.Location)
	for _, raw := range []string{"", "2026-02-30 12:00:00", "2026-10-30 10:00:05", "2026-10-30T10:10:00Z", "2028-01-01 00:00:00"} {
		if _, e := cmsScheduleTime(raw, now); e == nil {
			t.Fatalf("invalid time accepted: %q", raw)
		}
	}
	due, e := cmsScheduleTime("2026-10-30 10:10:00", now)
	if e != nil || due.Sub(now) != 10*time.Minute {
		t.Fatal("Beijing time parse failed")
	}
}
func TestCMSScheduleWorkerDrains(t *testing.T) {
	started := make(chan struct{})
	exited := make(chan struct{})
	w := startCMSScheduleWorker(context.Background(), func(ctx context.Context) error { close(started); <-ctx.Done(); close(exited); return ctx.Err() })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no immediate startup scan")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := w.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("worker did not drain")
	}
	if e := w.Stop(ctx); e != nil {
		t.Fatal("stop should be idempotent")
	}
}
