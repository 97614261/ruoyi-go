package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/jwtx"
	"ruoyi-go/pkg/redisx"
)

var (
	jwtSigner        *jwtx.Signer
	jwtExpire        time.Duration
	jwtRefreshWindow time.Duration
	errStopLoginScan = errors.New("停止扫描登录会话")
)

// InitToken 由 main 在启动时调用一次。
func InitToken(cfg config.JWTConfig) {
	jwtSigner = jwtx.NewSigner(cfg.Secret)
	jwtExpire = cfg.ExpireTime
	jwtRefreshWindow = cfg.RefreshWindow
}

// CreateToken 生成会话并签发 token。
//
// 顺序很重要：先落 Redis 再签发，避免签发成功但会话没写进去，
// 导致前端拿到一个立刻就失效的 token。
func CreateToken(ctx context.Context, loginUser *model.LoginUser) (string, error) {
	if loginUser == nil || loginUser.User == nil {
		return "", errors.New("创建会话失败：登录用户为空")
	}
	loginUser.Token = uuid.NewString()
	if err := RefreshToken(ctx, loginUser); err != nil {
		return "", err
	}
	return jwtSigner.Sign(loginUser.Token, loginUser.User.UserName)
}

// RefreshToken 刷新会话有效期。
//
// 只延长 Redis TTL，不重新签发 token —— 前端手里的 token 始终不变。
func RefreshToken(ctx context.Context, loginUser *model.LoginUser) error {
	now := time.Now()
	loginUser.LoginTime = now.UnixMilli()
	loginUser.ExpireTime = now.Add(jwtExpire).UnixMilli()

	data, err := json.Marshal(loginUser)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}
	key := redisx.LoginTokenKey(loginUser.Token)
	if err := redisx.C().Set(ctx, key, data, jwtExpire).Err(); err != nil {
		return fmt.Errorf("写入登录会话失败: %w", err)
	}
	return nil
}

// GetLoginUser 用 token 换会话。
//
// token 无效返回 error；token 有效但会话已过期/被踢返回 (nil, nil)，
// 两种情况调用方都应按未登录处理，但日志可以区分。
func GetLoginUser(ctx context.Context, tokenStr string) (*model.LoginUser, error) {
	claims, err := jwtSigner.Parse(tokenStr)
	if err != nil {
		return nil, err
	}
	return GetLoginUserByKey(ctx, claims.LoginUserKey)
}

// ParseToken 只校验签名并取出 claims，不查 Redis。
//
// 登出场景用：会话可能已经过期，但仍要拿到 uuid 去删 key。
func ParseToken(tokenStr string) (*jwtx.Claims, error) {
	return jwtSigner.Parse(tokenStr)
}

// GetLoginUserByKey 用 login_user_key（uuid）直接取会话。
func GetLoginUserByKey(ctx context.Context, loginUserKey string) (*model.LoginUser, error) {
	raw, err := redisx.C().Get(ctx, redisx.LoginTokenKey(loginUserKey)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取会话失败: %w", err)
	}

	var loginUser model.LoginUser
	if err := json.Unmarshal(raw, &loginUser); err != nil {
		return nil, fmt.Errorf("反序列化会话失败: %w", err)
	}
	return &loginUser, nil
}

// VerifyToken 剩余有效期不足 refreshWindow 时自动续期。
func VerifyToken(ctx context.Context, loginUser *model.LoginUser) error {
	remaining := time.UnixMilli(loginUser.ExpireTime).Sub(time.Now())
	if remaining <= jwtRefreshWindow {
		return RefreshToken(ctx, loginUser)
	}
	return nil
}

// RefreshOnlineUsersByRole 角色权限变更后，刷新所有持有该角色的在线用户。
//
// 对应 Java 版 TokenService.refreshPermissionByRoleId，但有两处不同：
//
//  1. 用 SCAN 而不是 KEYS 遍历会话。KEYS 是 O(N) 且阻塞整个 Redis 实例，
//     在线用户上千时一次角色调整就能让全站卡几秒。
//  2. 单个会话刷新失败只记日志继续，不中断整体 —— 少刷一个用户的代价是
//     他要等会话过期，比整批刷新失败小得多。
func RefreshOnlineUsersByRole(ctx context.Context, roleID int64) error {
	type permissionSnapshot struct {
		user        *model.SysUser
		permissions []string
	}
	snapshots := make(map[int64]permissionSnapshot)

	return scanLoginUsers(ctx, 100, func(loginUser *model.LoginUser) error {
		// 管理员拥有全部权限，无需刷新
		if loginUser.User == nil || loginUser.User.IsAdmin() || !hasRole(loginUser.User, roleID) {
			return nil
		}

		snapshot, ok := snapshots[loginUser.UserID]
		if !ok {
			fresh, err := repository.SelectUserByID(ctx, loginUser.UserID)
			if err != nil || fresh == nil {
				slog.Warn("重新加载用户失败，跳过", "userId", loginUser.UserID, "err", err)
				return nil
			}
			permissions, err := GetMenuPermission(ctx, fresh)
			if err != nil {
				slog.Warn("重算权限失败，跳过", "userId", loginUser.UserID, "err", err)
				return nil
			}
			snapshot = permissionSnapshot{user: fresh, permissions: permissions}
			snapshots[loginUser.UserID] = snapshot
		}
		loginUser.User = snapshot.user
		loginUser.Permissions = snapshot.permissions
		if err := RefreshToken(ctx, loginUser); err != nil {
			slog.Warn("刷新会话失败，跳过", "userId", loginUser.UserID, "err", err)
			return nil
		}
		slog.Info("角色变更已刷新在线用户权限", "roleId", roleID, "userId", loginUser.UserID)
		return nil
	})
}

// RefreshOnlineUserByID 刷新指定用户的在线会话权限。
//
// 用户被改了角色后调用，让新权限立刻生效而不用等他重新登录。
// 该用户不在线时静默返回。
func RefreshOnlineUserByID(ctx context.Context, userID int64) error {
	var (
		loaded      bool
		fresh       *model.SysUser
		permissions []string
	)
	return scanLoginUsers(ctx, 100, func(loginUser *model.LoginUser) error {
		if loginUser.UserID != userID {
			return nil
		}

		// 角色变了，要重新查库拿最新角色再算权限
		if !loaded {
			loaded = true
			var err error
			fresh, err = repository.SelectUserByID(ctx, userID)
			if err != nil || fresh == nil {
				slog.Warn("重新加载用户失败，跳过", "userId", userID, "err", err)
				return nil
			}
			permissions, err = GetMenuPermission(ctx, fresh)
			if err != nil {
				slog.Warn("重算权限失败，跳过", "userId", userID, "err", err)
				fresh = nil
				return nil
			}
		}
		if fresh == nil {
			return nil
		}
		loginUser.User = fresh
		loginUser.Permissions = permissions
		if err := RefreshToken(ctx, loginUser); err != nil {
			slog.Warn("刷新会话失败", "userId", userID, "err", err)
		}
		return nil
	})
}

// scanLoginUsers 用 SCAN + MGET 分批读取在线会话，避免每个 key 一次 Redis 往返。
// 单个损坏会话只记录错误类型；会话 key 含登录 UUID，禁止写入日志。
func scanLoginUsers(ctx context.Context, batch int64, fn func(*model.LoginUser) error) error {
	err := redisx.ScanKeyBatches(ctx, redisx.KeyLoginToken, batch, func(keys []string) error {
		values, err := redisx.C().MGet(ctx, keys...).Result()
		if err != nil {
			slog.Warn("批量读取在线会话失败，跳过当前批次", "count", len(keys), "err", err)
			return nil
		}
		for _, value := range values {
			raw, ok := value.(string)
			if !ok {
				continue // SCAN 与 MGET 之间过期属于正常情况
			}
			var loginUser model.LoginUser
			if err := json.Unmarshal([]byte(raw), &loginUser); err != nil {
				slog.Warn("会话反序列化失败，跳过", "err", err)
				continue
			}
			if err := fn(&loginUser); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errStopLoginScan) {
		return nil
	}
	return err
}

func hasRole(user *model.SysUser, roleID int64) bool {
	for _, role := range user.Roles {
		if role.RoleID == roleID {
			return true
		}
	}
	return false
}

// DeleteLoginUser 删除会话，用于登出和强制下线。
func DeleteLoginUser(ctx context.Context, loginUserKey string) error {
	if loginUserKey == "" {
		return nil
	}
	if err := redisx.C().Del(ctx, redisx.LoginTokenKey(loginUserKey)).Err(); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}
