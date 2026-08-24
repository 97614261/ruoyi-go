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
		return fmt.Errorf("写入会话 %s 失败: %w", key, err)
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
	return redisx.ScanKeys(ctx, redisx.KeyLoginToken, 100, func(key string) error {
		raw, err := redisx.C().Get(ctx, key).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil // 迭代期间刚好过期，正常
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
		// 管理员拥有全部权限，无需刷新
		if loginUser.User == nil || loginUser.User.IsAdmin() || !hasRole(loginUser.User, roleID) {
			return nil
		}

		permissions, err := GetMenuPermission(ctx, loginUser.User)
		if err != nil {
			slog.Warn("重算权限失败，跳过", "userId", loginUser.UserID, "err", err)
			return nil
		}
		loginUser.Permissions = permissions
		if err := RefreshToken(ctx, &loginUser); err != nil {
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
	return redisx.ScanKeys(ctx, redisx.KeyLoginToken, 100, func(key string) error {
		raw, err := redisx.C().Get(ctx, key).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil
		}
		if err != nil {
			slog.Warn("读取会话失败，跳过", "key", key, "err", err)
			return nil
		}

		var loginUser model.LoginUser
		if err := json.Unmarshal(raw, &loginUser); err != nil || loginUser.UserID != userID {
			return nil
		}

		// 角色变了，要重新查库拿最新角色再算权限
		fresh, err := repository.SelectUserByID(ctx, userID)
		if err != nil || fresh == nil {
			slog.Warn("重新加载用户失败，跳过", "userId", userID, "err", err)
			return nil
		}
		permissions, err := GetMenuPermission(ctx, fresh)
		if err != nil {
			slog.Warn("重算权限失败，跳过", "userId", userID, "err", err)
			return nil
		}
		loginUser.User = fresh
		loginUser.Permissions = permissions
		if err := RefreshToken(ctx, &loginUser); err != nil {
			slog.Warn("刷新会话失败", "userId", userID, "err", err)
		}
		return nil
	})
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
