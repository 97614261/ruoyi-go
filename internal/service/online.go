package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
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

	err := redisx.ScanKeys(ctx, redisx.KeyLoginToken, 200, func(key string) error {
		if len(all) >= maxOnlineScan {
			truncated = true
			return nil
		}
		raw, err := redisx.C().Get(ctx, key).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil // 迭代期间刚好过期
		}
		if err != nil {
			slog.Warn("读取会话失败，跳过", "key", key, "err", err)
			return nil
		}

		var loginUser model.LoginUser
		if err := json.Unmarshal(raw, &loginUser); err != nil {
			slog.Warn("会话反序列化失败，跳过", "key", key, "err", err)
			return nil
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
