package asyncx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolAppliesBackpressure(t *testing.T) {
	pool := NewPool(1, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if !pool.Submit(func() { close(started); <-release }) {
		t.Fatal("first task should be accepted")
	}
	<-started
	if !pool.Submit(func() {}) {
		t.Fatal("one queued task should be accepted")
	}
	if pool.Submit(func() {}) {
		t.Fatal("full bounded queue must reject additional work")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown should drain accepted work: %v", err)
	}
}

func TestPoolShutdownDrainsAndRejectsNewWork(t *testing.T) {
	pool := NewPool(2, 8)
	var completed atomic.Int32
	for range 6 {
		if !pool.Submit(func() {
			time.Sleep(time.Millisecond)
			completed.Add(1)
		}) {
			t.Fatal("task should be accepted")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if got := completed.Load(); got != 6 {
		t.Fatalf("shutdown lost accepted work: got %d want 6", got)
	}
	if pool.Submit(func() {}) {
		t.Fatal("closed pool must reject new work")
	}
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown must be idempotent: %v", err)
	}
}

func TestPoolShutdownHonorsContext(t *testing.T) {
	pool := NewPool(1, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if !pool.Submit(func() { close(started); <-release }) {
		t.Fatal("blocking task should be accepted")
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := pool.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown should honor deadline, got %v", err)
	}
	if pool.Submit(func() {}) {
		t.Fatal("pool must remain closed after a timed-out shutdown")
	}

	close(release)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := pool.Shutdown(waitCtx); err != nil {
		t.Fatalf("second shutdown should observe completed workers: %v", err)
	}
}
