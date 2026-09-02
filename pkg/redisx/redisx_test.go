package redisx

import (
	"context"
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
