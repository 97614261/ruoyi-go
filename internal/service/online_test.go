package service

import (
	"slices"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func TestPaginateOnlineUsersDeterministic(t *testing.T) {
	const tiedLoginTime int64 = 1_800_000_000_000
	users := []model.UserOnline{
		{TokenID: "token-c", LoginTime: types.FromUnixMilli(tiedLoginTime)},
		{TokenID: "token-old", LoginTime: types.FromUnixMilli(tiedLoginTime - 1)},
		{TokenID: "token-a", LoginTime: types.FromUnixMilli(tiedLoginTime)},
		{TokenID: "token-new", LoginTime: types.FromUnixMilli(tiedLoginTime + 1)},
		{TokenID: "token-b", LoginTime: types.FromUnixMilli(tiedLoginTime)},
	}
	want := []string{"token-new", "token-a", "token-b", "token-c", "token-old"}
	inputs := [][]model.UserOnline{
		slices.Clone(users),
		reversedOnlineUsers(users),
		{users[2], users[4], users[1], users[3], users[0]},
	}

	for inputIndex, input := range inputs {
		var got []string
		for pageNum := 1; pageNum <= 3; pageNum++ {
			items, total := paginateOnlineUsers(slices.Clone(input), page.Query{PageNum: pageNum, PageSize: 2})
			if total != int64(len(want)) {
				t.Fatalf("输入 %d 第 %d 页 total=%d，期望 %d", inputIndex, pageNum, total, len(want))
			}
			for _, item := range items {
				got = append(got, item.TokenID)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("输入 %d 分页合并结果=%v，期望 %v", inputIndex, got, want)
		}
	}
}

func TestPaginateOnlineUsersOutOfRange(t *testing.T) {
	items, total := paginateOnlineUsers([]model.UserOnline{
		{TokenID: "token-a", LoginTime: types.FromUnixMilli(1)},
	}, page.Query{PageNum: 2, PageSize: 10})
	if total != 1 {
		t.Fatalf("total=%d，期望 1", total)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("超出范围应返回非 nil 空切片，实际 %#v", items)
	}
}

func TestCollectOnlineUsersLimitsScannedSessionsBeforeFiltering(t *testing.T) {
	const total = 6000
	visited := 0
	items, truncated, err := collectOnlineUsers(
		model.OnlineQuery{UserName: "never-matches"},
		func(fn func(*model.LoginUser) error) (bool, error) {
			for i := 0; i < maxOnlineScan; i++ {
				visited++
				if err := fn(&model.LoginUser{User: &model.SysUser{UserName: "user"}}); err != nil {
					return false, err
				}
			}
			return total > maxOnlineScan, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(items) != 0 {
		t.Fatalf("过滤无结果也必须按扫描数截断: truncated=%v items=%d", truncated, len(items))
	}
	if visited != maxOnlineScan {
		t.Fatalf("最多应解码 %d 条，实际访问 %d 条", maxOnlineScan, visited)
	}
}

func reversedOnlineUsers(users []model.UserOnline) []model.UserOnline {
	result := slices.Clone(users)
	slices.Reverse(result)
	return result
}
