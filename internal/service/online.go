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
	var (
		all       []model.UserOnline
		truncated bool
	)

	err := scanLoginUsers(ctx, 200, func(loginUser *model.LoginUser) error {
		if len(all) >= maxOnlineScan {
			truncated = true
			return errStopLoginScan
		}

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
		return nil, 0, err
	}
	if truncated {
		slog.Warn("在线会话数超过扫描上限，列表已截断", "limit", maxOnlineScan)
	}

	// 最近登录的排前面
	sort.Slice(all, func(i, j int) bool {
		return all[i].LoginTime.Std().After(all[j].LoginTime.Std())
	})

	total := int64(len(all))
	start := pg.Offset()
	if start >= len(all) {
		return []model.UserOnline{}, total, nil
	}
	end := start + pg.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// ForceLogout 强制某个会话下线。
func ForceLogout(ctx context.Context, tokenID string) error {
	return DeleteLoginUser(ctx, tokenID)
}
