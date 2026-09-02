package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestShutdownComponentsUsesIndependentContexts(t *testing.T) {
	var order []string
	err := shutdownComponents(5*time.Millisecond,
		func(ctx context.Context) error {
			order = append(order, "http")
			<-ctx.Done()
			return ctx.Err()
		},
		func(ctx context.Context) error {
			order = append(order, "async")
			if err := ctx.Err(); err != nil {
				t.Fatalf("异步池不应复用已经超时的 HTTP context: %v", err)
			}
			return nil
		},
		func(ctx context.Context) {
			order = append(order, "scheduler")
			if err := ctx.Err(); err != nil {
				t.Fatalf("调度器不应复用已经超时的 HTTP context: %v", err)
			}
		},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("HTTP 关闭错误应原样返回，实际 %v", err)
	}
	if want := []string{"http", "async", "scheduler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("关闭顺序错误：want %v, got %v", want, order)
	}
}

func TestFinishServerStillClosesComponentsAfterListenerFailure(t *testing.T) {
	listenErr := errors.New("bind failed")
	var order []string
	err := finishServer(listenErr, time.Second,
		func(context.Context) error {
			order = append(order, "http")
			return nil
		},
		func(context.Context) error {
			order = append(order, "async")
			return nil
		},
		func(context.Context) { order = append(order, "scheduler") },
	)
	if !errors.Is(err, listenErr) {
		t.Fatalf("listener error must be preserved: %v", err)
	}
	if want := []string{"http", "async", "scheduler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("listener failure skipped cleanup: want %v, got %v", want, order)
	}
}
