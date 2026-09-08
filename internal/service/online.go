package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// maxOnlineScan 在线用户最多扫描的会话数。
//
// 这是内存分页：全部会话都要读出来才能过滤和排序。
// 会话数极多时截断并记日志，不能无上限地把整个 Redis 读进内存。
const maxOnlineScan = 5000

// ListOnlineUsers 查在线用户。
//
// 数据来自 Redis 而不是数据库 —— 这正是 JWT + Redis 双层设计的价值：
// 服务端始终知道谁在线，也能把谁踢下线。
func ListOnlineUsers(ctx context.Context, query model.OnlineQuery, pg page.Query) ([]model.UserOnline, int64, error) {
	all, truncated, err := collectOnlineUsers(query, func(fn func(*model.LoginUser) error) (bool, error) {
		return scanLoginUsers(ctx, 200, maxOnlineScan, fn)
	})
	if err != nil {
		return nil, 0, err
	}
	if truncated {
		slog.Warn("在线会话数超过扫描上限，列表已截断", "limit", maxOnlineScan)
	}

	items, total := paginateOnlineUsers(all, pg)
	return items, total, nil
}

func collectOnlineUsers(
	query model.OnlineQuery,
	scan func(func(*model.LoginUser) error) (bool, error),
) ([]model.UserOnline, bool, error) {
	all := make([]model.UserOnline, 0, 128)
	truncated, err := scan(func(loginUser *model.LoginUser) error {
		item := model.UserOnline{
			TokenID:       loginUser.Token,
			IPAddr:        loginUser.IPAddr,
			LoginLocation: loginUser.LoginLocation,
			Browser:       loginUser.Browser,
			OS:            loginUser.OS,
			LoginTime:     types.FromUnixMilli(loginUser.LoginTime),
		}
		if loginUser.User != nil {
			item.UserName = loginUser.User.UserName
			if loginUser.User.Dept != nil {
				item.DeptName = loginUser.User.Dept.DeptName
			}
		}

		if query.UserName != "" && !strings.Contains(item.UserName, query.UserName) {
			return nil
		}
		if query.IPAddr != "" && !strings.Contains(item.IPAddr, query.IPAddr) {
			return nil
		}
		all = append(all, item)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return all, truncated, nil
}

// paginateOnlineUsers 先建立全序再切页，避免 Redis SCAN 输入顺序变化时分页重漏。
func paginateOnlineUsers(all []model.UserOnline, pg page.Query) ([]model.UserOnline, int64) {
	sort.Slice(all, func(i, j int) bool {
		ti, tj := all[i].LoginTime.Std(), all[j].LoginTime.Std()
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return all[i].TokenID < all[j].TokenID
	})

	total := int64(len(all))
	start := pg.Offset()
	if start >= len(all) {
		return []model.UserOnline{}, total
	}
	end := start + pg.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total
}

// ForceLogout 强制某个会话下线。
func ForceLogout(ctx context.Context, tokenID string) error {
	return DeleteLoginUser(ctx, tokenID)
}
