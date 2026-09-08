package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPermissionRevocationRetriesWithFreshContext(t *testing.T) {
	var calls atomic.Int32
	recovered := make(chan struct{})
	allowRecovery := make(chan struct{})
	q := newPermissionRevocationQueue(func(ctx context.Context, userIDs []int64) map[int64]error {
		if len(userIDs) != 1 || userIDs[0] != 42 {
			t.Errorf("userIds=%v，期望 [42]", userIDs)
		}
		if ctx.Err() != nil {
			t.Errorf("补偿使用了已失效 context: %v", ctx.Err())
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("补偿 context 必须有独立 deadline")
		}
		if calls.Add(1) == 1 {
			return map[int64]error{42: errors.New("redis 暂时不可用")}
		}
		select {
		case <-allowRecovery:
		case <-ctx.Done():
			return map[int64]error{42: ctx.Err()}
		}
		select {
		case <-recovered:
		default:
			close(recovered)
		}
		return map[int64]error{}
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = q.shutdown(ctx)
	})

	if err := q.compensate(42); err == nil {
		t.Fatal("首次撤销失败应返回错误")
	}
	if !q.isPending(42) {
		t.Fatal("撤销失败期间用户必须保持拒绝状态")
	}
	close(allowRecovery)
	select {
	case <-recovered:
	case <-time.After(2 * time.Second):
		t.Fatal("补偿队列未重试")
	}
	deadline := time.Now().Add(time.Second)
	for q.isPending(42) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if q.isPending(42) {
		t.Fatal("补偿成功后应清除拒绝状态")
	}
	if cap(q.wake) != 1 {
		t.Fatalf("补偿通知队列必须有界，cap=%d", cap(q.wake))
	}
}

func TestPermissionRevocationLargeRoleIsBounded(t *testing.T) {
	ids := make([]int64, 100000)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	q := newPermissionRevocationQueue(func(_ context.Context, batch []int64) map[int64]error {
		failures := make(map[int64]error, len(batch))
		for _, userID := range batch {
			failures[userID] = errors.New("redis unavailable")
		}
		return failures
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = q.shutdown(ctx)
	})

	err := q.compensate(ids...)
	if err == nil {
		t.Fatal("大角色撤销失败应返回汇总错误")
	}
	if details := strings.Count(err.Error(), "撤销用户 "); details > permissionErrorDetailLimit {
		t.Fatalf("错误详情未限流: details=%d", details)
	}
	if len(err.Error()) > 10000 {
		t.Fatalf("错误文本随用户数线性膨胀: bytes=%d", len(err.Error()))
	}
	stats := q.stats()
	if stats.MaxBatchObserved > permissionRetryBatchSize {
		t.Fatalf("补偿批次超过上限: %+v", stats)
	}
	if stats.Pending == 0 {
		t.Fatalf("失败用户应留在补偿队列: %+v", stats)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("等待重试=%d", stats.Pending)) && stats.Pending == len(ids) {
		t.Fatalf("汇总错误未体现待重试数量: %v", err)
	}
}

func TestForEachIDBatchCapsBatchSize(t *testing.T) {
	ids := make([]int64, 1001)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	seen := 0
	maxBatch := 0
	err := forEachIDBatch(ids, 200, func(batch []int64) error {
		seen += len(batch)
		maxBatch = max(maxBatch, len(batch))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(ids) || maxBatch != 200 {
		t.Fatalf("分批错误: seen=%d maxBatch=%d", seen, maxBatch)
	}
}

func TestPermissionMutationAdvancesBeforeDatabaseAndRefreshesAfter(t *testing.T) {
	steps := make([]string, 0, 3)
	err := runPermissionMutation(context.Background(), []int64{7},
		func() error {
			steps = append(steps, "database")
			return nil
		},
		func(context.Context, []int64) (map[int64]int64, error) {
			steps = append(steps, "version")
			return map[int64]int64{7: 3}, nil
		},
		func(_ context.Context, ids []int64, versions map[int64]int64) error {
			if len(ids) != 1 || ids[0] != 7 || versions[7] != 3 {
				t.Fatalf("刷新参数错误: ids=%v versions=%v", ids, versions)
			}
			steps = append(steps, "refresh")
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps, ","); got != "version,database,refresh" {
		t.Fatalf("权限写入顺序错误: %s", got)
	}
}

func TestPermissionMutationRejectsDatabaseWriteWhenVersionFails(t *testing.T) {
	databaseCalled := false
	want := errors.New("redis unavailable")
	err := runPermissionMutation(context.Background(), []int64{7},
		func() error { databaseCalled = true; return nil },
		func(context.Context, []int64) (map[int64]int64, error) { return nil, want },
		func(context.Context, []int64, map[int64]int64) error { return nil })
	if !errors.Is(err, want) || databaseCalled {
		t.Fatalf("版本推进失败必须拒绝数据库写入: err=%v databaseCalled=%v", err, databaseCalled)
	}
}
