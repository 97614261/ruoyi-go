package redisx

import (
	"context"
	"fmt"
	"testing"
)

func TestScanKeyBatchesStopsWithoutScanningNextCursor(t *testing.T) {
	called := 0
	err := scanKeyBatches(context.Background(), "prefix:", 10,
		func(_ context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
			called++
			if cursor != 0 || pattern != "prefix:*" || count != 10 {
				t.Fatalf("unexpected scan arguments: cursor=%d pattern=%q count=%d", cursor, pattern, count)
			}
			return []string{"prefix:1"}, 99, nil
		},
		func([]string) error { return ErrStopScan },
	)
	if err != nil {
		t.Fatalf("early stop should be successful: %v", err)
	}
	if called != 1 {
		t.Fatalf("SCAN called %d times, want 1", called)
	}
}

func TestScanKeyBatchesBoundedCropsBeforeRead(t *testing.T) {
	calls := 0
	delivered := 0
	maxDelivered := 0
	truncated, err := scanKeyBatchesBounded(context.Background(), "login_tokens:", 200, 5000,
		func(_ context.Context, cursor uint64, _ string, _ int64) ([]string, uint64, error) {
			calls++
			size := 4900
			next := uint64(1)
			if cursor == 1 {
				size = 200
				next = 2
			}
			keys := make([]string, size)
			for i := range keys {
				keys[i] = fmt.Sprintf("login_tokens:%d", delivered+i)
			}
			return keys, next, nil
		},
		func(keys []string) error {
			delivered += len(keys)
			maxDelivered = max(maxDelivered, len(keys))
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || calls != 2 || delivered != 5000 {
		t.Fatalf("边界扫描错误: truncated=%v calls=%d delivered=%d", truncated, calls, delivered)
	}
	if maxDelivered != 4900 {
		t.Fatalf("首批输入未保留: maxDelivered=%d", maxDelivered)
	}
}

func TestScanKeyBatchesBoundedStopsDamagedKeySpace(t *testing.T) {
	const total = 6000
	cursor := 0
	read := 0
	truncated, err := scanKeyBatchesBounded(context.Background(), "login_tokens:", 200, 5000,
		func(_ context.Context, _ uint64, _ string, _ int64) ([]string, uint64, error) {
			remaining := total - cursor
			size := min(200, remaining)
			keys := make([]string, size)
			cursor += size
			if cursor < total {
				return keys, uint64(cursor), nil
			}
			return keys, 0, nil
		},
		func(keys []string) error {
			// 即使这些 key 对应的 value 全部损坏，读取量也由 key 数限制。
			read += len(keys)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || read != 5000 {
		t.Fatalf("损坏 key 空间未被硬截断: truncated=%v read=%d", truncated, read)
	}
}
