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
	if want := []string{"http", "scheduler"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("关闭顺序错误：want %v, got %v", want, order)
	}
}
