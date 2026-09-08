package service

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestRolePermissionMutationCapturesMembersBeforeWrite(t *testing.T) {
	steps := make([]string, 0, 3)
	err := mutateRolePermissionStateWith(context.Background(), 9,
		func(_ context.Context, roleID int64) ([]int64, error) {
			if roleID != 9 {
				t.Fatalf("roleId=%d", roleID)
			}
			steps = append(steps, "members")
			return []int64{11, 12}, nil
		},
		func(_ context.Context, userIDs []int64, mutate func() error) error {
			if !slices.Equal(userIDs, []int64{11, 12}) {
				t.Fatalf("未使用写入前固定的成员: %v", userIDs)
			}
			steps = append(steps, "version")
			return mutate()
		},
		func() error {
			steps = append(steps, "database")
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(steps, []string{"members", "version", "database"}) {
		t.Fatalf("角色权限写入顺序错误: %v", steps)
	}
}

func TestRolePermissionMutationStopsWhenMemberQueryFails(t *testing.T) {
	want := errors.New("database unavailable")
	applyCalled := false
	databaseCalled := false
	err := mutateRolePermissionStateWith(context.Background(), 9,
		func(context.Context, int64) ([]int64, error) { return nil, want },
		func(context.Context, []int64, func() error) error { applyCalled = true; return nil },
		func() error { databaseCalled = true; return nil })
	if !errors.Is(err, want) || applyCalled || databaseCalled {
		t.Fatalf("成员查询失败仍进入权限/数据库写入: err=%v apply=%v database=%v",
			err, applyCalled, databaseCalled)
	}
}
