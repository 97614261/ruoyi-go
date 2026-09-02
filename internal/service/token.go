package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	jwtSigner          *jwtx.Signer
	jwtExpire          time.Duration
	jwtRefreshWindow   time.Duration
	errStopLoginScan   = errors.New("停止扫描登录会话")
	errSessionChanged  = errors.New("登录状态已变更，请重新登录")
	errSessionConflict = errors.New("登录会话已被并发更新")
)

const maxSessionRefreshRetries = 3

var createSessionScript = redis.NewScript(`
local current = tonumber(redis.call('get', KEYS[3]) or '0')
local expected = tonumber(ARGV[4])
if current ~= expected then
    return 0
end
redis.call('set', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('sadd', KEYS[2], ARGV[3])
redis.call('pexpire', KEYS[2], ARGV[2])
redis.call('set', KEYS[3], tostring(current), 'PX', ARGV[2])
return 1
`)

var renewSessionScript = redis.NewScript(`
local current = tonumber(redis.call('get', KEYS[3]) or '0')
local expected = tonumber(ARGV[4])
if current ~= expected then
    return -1
end
local raw = redis.call('get', KEYS[1])
if not raw then
    return 0
end
local ok, session = pcall(cjson.decode, raw)
local updatedOK = pcall(cjson.decode, ARGV[1])
if not ok or not updatedOK then
    return -2
end
local currentRevision = tonumber(session['sessionRevision'] or '0')
local expectedRevision = tonumber(ARGV[5])
if currentRevision ~= expectedRevision then
    return 2
end
redis.call('set', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('sadd', KEYS[2], ARGV[3])
redis.call('pexpire', KEYS[2], ARGV[2])
redis.call('set', KEYS[3], tostring(current), 'PX', ARGV[2])
return 1
`)

var updateSessionPermissionsScript = redis.NewScript(`
local currentGeneration = tonumber(redis.call('get', KEYS[3]) or '0')
local expectedGeneration = tonumber(ARGV[3])
if currentGeneration ~= expectedGeneration then
    return -1
end
local raw = redis.call('get', KEYS[1])
if not raw then
    return 0
end
local sessionOK, session = pcall(cjson.decode, raw)
local updatedOK = pcall(cjson.decode, ARGV[4])
if not sessionOK or not updatedOK then
    return -2
end
local currentRevision = tonumber(session['sessionRevision'] or '0')
local expectedRevision = tonumber(ARGV[5])
if currentRevision ~= expectedRevision then
    return 2
end
redis.call('set', KEYS[1], ARGV[4], 'PX', ARGV[1])
redis.call('sadd', KEYS[2], ARGV[2])
redis.call('pexpire', KEYS[2], ARGV[1])
redis.call('set', KEYS[3], tostring(currentGeneration), 'PX', ARGV[1])
return 1
`)

var deleteSessionScript = redis.NewScript(`
local raw = redis.call('get', KEYS[1])
redis.call('del', KEYS[1])
if not raw then
    return 0
end
local ok, session = pcall(cjson.decode, raw)
if ok and session['userId'] then
    local indexKey = ARGV[2] .. tostring(session['userId'])
    redis.call('srem', indexKey, ARGV[1])
    if redis.call('scard', indexKey) == 0 then
        redis.call('del', indexKey)
    end
end
return 1
`)

var revokeSessionsScript = redis.NewScript(`
redis.call('incr', KEYS[1])
local members = redis.call('smembers', KEYS[2])
for _, tokenId in ipairs(members) do
    redis.call('del', ARGV[1] .. tokenId)
end
redis.call('del', KEYS[2])
redis.call('pexpire', KEYS[1], ARGV[2])
return #members
`)

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
	if err := writeNewSession(ctx, loginUser); err != nil {
		return "", err
	}
	return jwtSigner.Sign(loginUser.Token, loginUser.User.UserName)
}

func writeNewSession(ctx context.Context, loginUser *model.LoginUser) error {
	setSessionTimes(loginUser)
	data, err := json.Marshal(loginUser)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}
	result, err := createSessionScript.Run(ctx, redisx.C(), sessionKeys(loginUser),
		data, jwtExpire.Milliseconds(), loginUser.Token, loginUser.SessionGeneration).Int64()
	if err != nil {
		return fmt.Errorf("写入登录会话失败: %w", err)
	}
	if result == 0 {
		return errSessionChanged
	}
	return nil
}

// RefreshToken 原地刷新 Redis 中最新会话的时间和 TTL，不覆盖权限快照。
func RefreshToken(ctx context.Context, loginUser *model.LoginUser) error {
	current := loginUser
	for attempt := 0; attempt < maxSessionRefreshRetries; attempt++ {
		refreshed := *current
		setSessionTimes(&refreshed)
		data, err := json.Marshal(&refreshed)
		if err != nil {
			return fmt.Errorf("序列化续期会话失败: %w", err)
		}
		result, err := renewSessionScript.Run(ctx, redisx.C(), sessionKeys(current),
			data, jwtExpire.Milliseconds(), current.Token, current.SessionGeneration,
			current.SessionRevision).Int64()
		if err != nil {
			return fmt.Errorf("续期登录会话失败: %w", err)
		}
		switch result {
		case 1:
			*loginUser = refreshed
			return nil
		case 2:
			latest, err := GetLoginUserByKey(ctx, current.Token)
			if err != nil {
				return err
			}
			if latest == nil {
				return errSessionChanged
			}
			current = latest
		case 0, -1:
			return errSessionChanged
		default:
			return fmt.Errorf("Redis 登录会话内容无效")
		}
	}
	return errSessionConflict
}

func setSessionTimes(loginUser *model.LoginUser) {
	now := time.Now()
	loginUser.LoginTime = now.UnixMilli()
	loginUser.ExpireTime = now.Add(jwtExpire).UnixMilli()
}

func sessionKeys(loginUser *model.LoginUser) []string {
	return []string{
		redisx.LoginTokenKey(loginUser.Token),
		redisx.LoginUserSessionsKey(loginUser.UserID),
		redisx.LoginUserGenerationKey(loginUser.UserID),
	}
}

// SessionGeneration 返回用户当前的 Redis 会话代数，不存在时从 0 开始。
func SessionGeneration(ctx context.Context, userID int64) (int64, error) {
	generation, err := redisx.C().Get(ctx, redisx.LoginUserGenerationKey(userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取用户会话代数失败: %w", err)
	}
	return generation, nil
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
	generation, err := SessionGeneration(ctx, loginUser.UserID)
	if err != nil {
		return nil, err
	}
	if generation != loginUser.SessionGeneration {
		_ = DeleteLoginUser(ctx, loginUserKey)
		return nil, nil
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

type permissionSnapshot struct {
	user        *model.SysUser
	permissions []string
}

// RefreshLoginUserPermissions 重新加载数据库权限，并用 revision CAS 更新指定会话。
func RefreshLoginUserPermissions(ctx context.Context, loginUser *model.LoginUser) error {
	if loginUser == nil || loginUser.Token == "" || loginUser.UserID <= 0 {
		return errors.New("刷新登录会话失败：会话为空")
	}
	snapshot, err := loadPermissionSnapshot(ctx, loginUser.UserID)
	if err != nil {
		return failClosedUserRefresh(ctx, loginUser.UserID, err)
	}
	return refreshSessionPermissions(ctx, loginUser, &snapshot)
}

func loadPermissionSnapshot(ctx context.Context, userID int64) (permissionSnapshot, error) {
	fresh, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return permissionSnapshot{}, err
	}
	if fresh == nil {
		return permissionSnapshot{}, fmt.Errorf("用户 %d 不存在", userID)
	}
	permissions, err := GetMenuPermission(ctx, fresh)
	if err != nil {
		return permissionSnapshot{}, err
	}
	if permissions == nil {
		permissions = []string{}
	}
	return permissionSnapshot{user: fresh, permissions: permissions}, nil
}

func refreshSessionPermissions(ctx context.Context, loginUser *model.LoginUser,
	snapshot *permissionSnapshot) error {
	current := loginUser
	for attempt := 0; attempt < maxSessionRefreshRetries; attempt++ {
		err := writeSessionPermissions(ctx, current, *snapshot)
		if err == nil {
			if current != loginUser {
				*loginUser = *current
			}
			return nil
		}
		if !errors.Is(err, errSessionConflict) {
			return failClosedSessionRefresh(ctx, current, err)
		}

		latest, err := GetLoginUserByKey(ctx, current.Token)
		if err != nil {
			return failClosedSessionRefresh(ctx, current, err)
		}
		if latest == nil {
			return errSessionChanged
		}
		loaded, err := loadPermissionSnapshot(ctx, latest.UserID)
		if err != nil {
			return failClosedSessionRefresh(ctx, latest, err)
		}
		current = latest
		*snapshot = loaded
	}
	return failClosedSessionRefresh(ctx, current, errSessionConflict)
}

func writeSessionPermissions(ctx context.Context, loginUser *model.LoginUser,
	snapshot permissionSnapshot) error {
	now := time.Now()
	updated := *loginUser
	updated.User = snapshot.user
	updated.DeptID = snapshot.user.DeptID
	updated.Permissions = snapshot.permissions
	updated.LoginTime = now.UnixMilli()
	updated.ExpireTime = now.Add(jwtExpire).UnixMilli()
	updated.SessionRevision++
	data, err := json.Marshal(&updated)
	if err != nil {
		return fmt.Errorf("序列化用户权限快照失败: %w", err)
	}

	result, err := updateSessionPermissionsScript.Run(ctx, redisx.C(), sessionKeys(loginUser),
		jwtExpire.Milliseconds(), loginUser.Token, loginUser.SessionGeneration,
		data, loginUser.SessionRevision).Int64()
	if err != nil {
		return fmt.Errorf("原子更新登录权限失败: %w", err)
	}
	switch result {
	case 1:
		*loginUser = updated
		return nil
	case 2:
		return errSessionConflict
	case 0, -1:
		return errSessionChanged
	default:
		return fmt.Errorf("Redis 登录会话内容无效")
	}
}

func failClosedSessionRefresh(ctx context.Context, loginUser *model.LoginUser, cause error) error {
	if loginUser == nil || loginUser.Token == "" {
		return cause
	}
	if err := DeleteLoginUser(ctx, loginUser.Token); err != nil {
		return fmt.Errorf("刷新权限失败且安全撤销会话失败: %w", errors.Join(cause, err))
	}
	return fmt.Errorf("刷新权限失败，已安全撤销会话: %w", cause)
}

func failClosedUserRefresh(ctx context.Context, userID int64, cause error) error {
	if err := RevokeUserSessions(ctx, userID); err != nil {
		return fmt.Errorf("加载用户权限失败且安全撤销会话失败: %w", errors.Join(cause, err))
	}
	return fmt.Errorf("加载用户权限失败，已安全撤销全部会话: %w", cause)
}

// RefreshOnlineUsersByRole 角色权限变更后，刷新所有持有该角色的在线用户。
//
// 对应 Java 版 TokenService.refreshPermissionByRoleId，但有三处不同：
//
//  1. 用 SCAN 而不是 KEYS 遍历会话。KEYS 是 O(N) 且阻塞整个 Redis 实例，
//     在线用户上千时一次角色调整就能让全站卡几秒。
//  2. 权限写入使用 revision CAS，冲突时重新加载，旧快照不能覆盖新权限。
//  3. 单个会话无法确认时先安全撤销；其余会话继续处理，结束后返回首个错误。
func RefreshOnlineUsersByRole(ctx context.Context, roleID int64) error {
	snapshots := make(map[int64]*permissionSnapshot)
	failedUsers := make(map[int64]struct{})
	var firstErr error

	err := scanLoginUsers(ctx, 100, func(loginUser *model.LoginUser) error {
		// 管理员拥有全部权限，无需刷新
		if loginUser.User == nil || loginUser.User.IsAdmin() || !hasRole(loginUser.User, roleID) {
			return nil
		}
		if _, failed := failedUsers[loginUser.UserID]; failed {
			return nil
		}

		snapshot, ok := snapshots[loginUser.UserID]
		if !ok {
			loaded, err := loadPermissionSnapshot(ctx, loginUser.UserID)
			if err != nil {
				failedUsers[loginUser.UserID] = struct{}{}
				err = failClosedUserRefresh(ctx, loginUser.UserID, err)
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("重新加载用户失败，已撤销会话", "userId", loginUser.UserID, "err", err)
				return nil
			}
			snapshot = &loaded
			snapshots[loginUser.UserID] = snapshot
		}
		if err := refreshSessionPermissions(ctx, loginUser, snapshot); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("刷新会话失败或已安全撤销", "userId", loginUser.UserID, "err", err)
			return nil
		}
		slog.Info("角色变更已刷新在线用户权限", "roleId", roleID, "userId", loginUser.UserID)
		return nil
	})
	if err != nil {
		return err
	}
	return firstErr
}

// RefreshOnlineUserByID 刷新指定用户的在线会话权限。
//
// 用户被改了角色后调用，让新权限立刻生效而不用等他重新登录。
// 该用户不在线时静默返回。
func RefreshOnlineUserByID(ctx context.Context, userID int64) error {
	return RefreshOnlineUsersByID(ctx, userID)
}

// RefreshOnlineUsersByID 按用户反向索引批量刷新在线会话。
//
// Java 硬切 Go 时会清理全部旧会话，因此运行期只接受新版 Go 创建的索引会话。
// 这里不再扫描 login_tokens:*；无索引残余必须按部署检查单清理。
func RefreshOnlineUsersByID(ctx context.Context, userIDs ...int64) error {
	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}

	sessions, err := indexedLoginUsers(ctx, ids)
	if err != nil {
		return err
	}
	var firstErr error
	for _, userID := range ids {
		loginUsers := sessions[userID]
		if len(loginUsers) == 0 {
			continue
		}

		snapshot, err := loadPermissionSnapshot(ctx, userID)
		if err != nil {
			err = failClosedUserRefresh(ctx, userID, err)
			slog.Warn("重新加载用户失败，已撤销会话", "userId", userID, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, loginUser := range loginUsers {
			if err := refreshSessionPermissions(ctx, loginUser, &snapshot); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("刷新会话失败或已安全撤销", "userId", userID, "err", err)
			}
		}
	}
	return firstErr
}

// indexedLoginUsers 用 Pipeline 读取用户索引，再分批 MGET 会话。
// 过期 token 留下的索引成员会被清理；token UUID 不进入日志。
func indexedLoginUsers(ctx context.Context, userIDs []int64) (map[int64][]*model.LoginUser, error) {
	type indexCommand struct {
		userID int64
		cmd    *redis.StringSliceCmd
	}
	commands := make([]indexCommand, 0, len(userIDs))
	_, err := redisx.C().Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, userID := range userIDs {
			commands = append(commands, indexCommand{
				userID: userID,
				cmd:    pipe.SMembers(ctx, redisx.LoginUserSessionsKey(userID)),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("批量读取用户会话索引失败: %w", err)
	}

	memberships := make(map[string][]int64)
	tokenIDs := make([]string, 0)
	for _, command := range commands {
		members, err := command.cmd.Result()
		if err != nil {
			return nil, fmt.Errorf("读取用户 %d 会话索引失败: %w", command.userID, err)
		}
		for _, tokenID := range members {
			if tokenID == "" {
				continue
			}
			if _, exists := memberships[tokenID]; !exists {
				tokenIDs = append(tokenIDs, tokenID)
			}
			memberships[tokenID] = append(memberships[tokenID], command.userID)
		}
	}

	result := make(map[int64][]*model.LoginUser, len(userIDs))
	stale := make(map[int64][]string)
	const batchSize = 100
	for start := 0; start < len(tokenIDs); start += batchSize {
		end := min(start+batchSize, len(tokenIDs))
		batch := tokenIDs[start:end]
		keys := make([]string, len(batch))
		for i, tokenID := range batch {
			keys[i] = redisx.LoginTokenKey(tokenID)
		}
		values, err := redisx.C().MGet(ctx, keys...).Result()
		if err != nil {
			return nil, fmt.Errorf("批量读取用户登录会话失败: %w", err)
		}
		for i, value := range values {
			tokenID := batch[i]
			raw, ok := value.(string)
			if !ok {
				for _, userID := range memberships[tokenID] {
					stale[userID] = append(stale[userID], tokenID)
				}
				continue
			}

			var loginUser model.LoginUser
			if err := json.Unmarshal([]byte(raw), &loginUser); err != nil {
				slog.Warn("会话反序列化失败，跳过", "err", err)
				continue
			}
			if loginUser.Token == "" {
				loginUser.Token = tokenID
			}
			for _, indexedUserID := range memberships[tokenID] {
				if loginUser.UserID == indexedUserID {
					result[indexedUserID] = append(result[indexedUserID], &loginUser)
				} else {
					stale[indexedUserID] = append(stale[indexedUserID], tokenID)
				}
			}
		}
	}

	if len(stale) > 0 {
		_, err := redisx.C().Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for userID, tokenIDs := range stale {
				members := make([]any, len(tokenIDs))
				for i, tokenID := range tokenIDs {
					members[i] = tokenID
				}
				pipe.SRem(ctx, redisx.LoginUserSessionsKey(userID), members...)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("清理失效用户会话索引失败: %w", err)
		}
	}
	return result, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// scanLoginUsers 用 SCAN + MGET 分批读取在线会话，避免每个 key 一次 Redis 往返。
// 单个损坏会话只记录错误类型；会话 key 含登录 UUID，禁止写入日志。
func scanLoginUsers(ctx context.Context, batch int64, fn func(*model.LoginUser) error) error {
	return scanLoginUserEntries(ctx, batch, func(_ string, loginUser *model.LoginUser) error {
		return fn(loginUser)
	})
}

func scanLoginUserEntries(ctx context.Context, batch int64, fn func(string, *model.LoginUser) error) error {
	err := redisx.ScanKeyBatches(ctx, redisx.KeyLoginToken, batch, func(keys []string) error {
		values, err := redisx.C().MGet(ctx, keys...).Result()
		if err != nil {
			slog.Warn("批量读取在线会话失败，跳过当前批次", "count", len(keys), "err", err)
			return nil
		}
		for i, value := range values {
			raw, ok := value.(string)
			if !ok {
				continue // SCAN 与 MGET 之间过期属于正常情况
			}
			var loginUser model.LoginUser
			if err := json.Unmarshal([]byte(raw), &loginUser); err != nil {
				slog.Warn("会话反序列化失败，跳过", "err", err)
				continue
			}
			tokenID := loginUser.Token
			if tokenID == "" {
				tokenID = strings.TrimPrefix(keys[i], redisx.KeyLoginToken)
				loginUser.Token = tokenID
			}
			if err := fn(tokenID, &loginUser); err != nil {
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
	if _, err := deleteSessionScript.Run(ctx, redisx.C(), []string{redisx.LoginTokenKey(loginUserKey)},
		loginUserKey, redisx.KeyLoginUserSessions).Int64(); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

// RevokeUserSessions 撤销用户的全部索引会话，并递增会话代数。
// 无索引残余即使仍在 Redis，也会因为代数不匹配而无法通过鉴权。
func RevokeUserSessions(ctx context.Context, userIDs ...int64) error {
	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}

	var revokeErrors []error
	for _, userID := range ids {
		keys := []string{
			redisx.LoginUserGenerationKey(userID),
			redisx.LoginUserSessionsKey(userID),
		}
		if _, err := revokeSessionsScript.Run(ctx, redisx.C(), keys,
			redisx.KeyLoginToken, jwtExpire.Milliseconds()).Int64(); err != nil {
			revokeErrors = append(revokeErrors, fmt.Errorf("撤销用户 %d 会话失败: %w", userID, err))
		}
	}
	return errors.Join(revokeErrors...)
}
