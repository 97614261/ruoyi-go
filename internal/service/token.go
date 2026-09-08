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
local currentPermission = tonumber(redis.call('get', KEYS[4]) or '0')
local expectedPermission = tonumber(ARGV[5])
if currentPermission ~= expectedPermission then
    return -1
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
local currentPermission = tonumber(redis.call('get', KEYS[4]) or '0')
local expectedPermission = tonumber(ARGV[6])
if currentPermission ~= expectedPermission then
    return -3
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
local currentPermission = tonumber(redis.call('get', KEYS[4]) or '0')
local expectedPermission = tonumber(ARGV[6])
if currentPermission ~= expectedPermission then
    return -3
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

const revokeSessionsLua = `
redis.call('incr', KEYS[1])
local members = redis.call('smembers', KEYS[2])
for _, tokenId in ipairs(members) do
    redis.call('del', ARGV[1] .. tokenId)
end
redis.call('del', KEYS[2])
redis.call('pexpire', KEYS[1], ARGV[2])
return #members
`

var advancePermissionVersionScript = redis.NewScript(`
return redis.call('incr', KEYS[1])
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
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	version, err := PermissionVersion(ctx, loginUser.UserID)
	if err != nil {
		return "", err
	}
	loginUser.PermissionVersion = version
	return createTokenLocked(ctx, loginUser)
}

// createTokenLocked 要求调用方持有 permissionRefreshMu，并已装入权限版本。
func createTokenLocked(ctx context.Context, loginUser *model.LoginUser) (string, error) {
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
		data, jwtExpire.Milliseconds(), loginUser.Token, loginUser.SessionGeneration,
		loginUser.PermissionVersion).Int64()
	if err != nil {
		return fmt.Errorf("写入登录会话失败: %w", err)
	}
	if result == 0 || result == -1 {
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
			current.SessionRevision, current.PermissionVersion).Int64()
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
		case 0, -1, -3:
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
		redisx.LoginUserPermissionVersionKey(loginUser.UserID),
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

// PermissionVersion 返回用户当前权限版本；从未变更过时为 0。
func PermissionVersion(ctx context.Context, userID int64) (int64, error) {
	version, err := redisx.C().Get(ctx, redisx.LoginUserPermissionVersionKey(userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取用户权限版本失败: %w", err)
	}
	return version, nil
}

// advancePermissionVersions 在数据库权限写入前推进版本。版本不设置 TTL：
// 只要旧 JWT 仍可能存在，版本就不能回退或消失。
func advancePermissionVersions(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	ids := uniquePositiveIDs(userIDs)
	versions := make(map[int64]int64, len(ids))
	const batchSize = 200
	err := forEachIDBatch(ids, batchSize, func(batch []int64) error {
		type command struct {
			userID int64
			cmd    *redis.Cmd
		}
		commands := make([]command, 0, len(batch))
		_, err := redisx.C().Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, userID := range batch {
				commands = append(commands, command{
					userID: userID,
					cmd: advancePermissionVersionScript.Run(ctx, pipe,
						[]string{redisx.LoginUserPermissionVersionKey(userID)}),
				})
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("推进用户权限版本失败: %w", err)
		}
		for _, command := range commands {
			version, err := command.cmd.Int64()
			if err != nil {
				return fmt.Errorf("读取用户 %d 权限版本失败: %w", command.userID, err)
			}
			versions[command.userID] = version
		}
		return nil
	})
	return versions, err
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
	loginUser, err := GetLoginUserByKey(ctx, claims.LoginUserKey)
	if err != nil || loginUser == nil {
		return loginUser, err
	}
	if permissionRevocations.isPending(loginUser.UserID) {
		return nil, nil
	}
	version, err := PermissionVersion(ctx, loginUser.UserID)
	if err != nil {
		return nil, err
	}
	if version == loginUser.PermissionVersion {
		return loginUser, nil
	}
	return refreshStaleLoginUser(ctx, loginUser)
}

// refreshStaleLoginUser 在鉴权路径懒刷新旧权限快照。全局权限锁同时
// 协调登录建会话和权限写入，避免新会话夹在版本推进与数据库提交之间。
func refreshStaleLoginUser(ctx context.Context, loginUser *model.LoginUser) (*model.LoginUser, error) {
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()

	latest, err := GetLoginUserByKey(ctx, loginUser.Token)
	if err != nil || latest == nil {
		return latest, err
	}
	version, err := PermissionVersion(ctx, latest.UserID)
	if err != nil {
		return nil, err
	}
	if version == latest.PermissionVersion {
		return latest, nil
	}
	snapshot, err := loadPermissionSnapshot(ctx, latest.UserID)
	if err != nil {
		return nil, failClosedUserRefresh(ctx, latest.UserID, err)
	}
	if err := refreshSessionPermissionsRaw(ctx, latest, &snapshot, version); err != nil {
		return nil, failClosedSessionRefresh(ctx, latest, err)
	}
	permissionRevocations.clear(latest.UserID)
	return latest, nil
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
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()

	if loginUser == nil || loginUser.Token == "" || loginUser.UserID <= 0 {
		return errors.New("刷新登录会话失败：会话为空")
	}
	snapshot, err := loadPermissionSnapshot(ctx, loginUser.UserID)
	if err != nil {
		return failClosedUserRefresh(ctx, loginUser.UserID, err)
	}
	version, err := PermissionVersion(ctx, loginUser.UserID)
	if err != nil {
		return failClosedUserRefresh(ctx, loginUser.UserID, err)
	}
	return refreshSessionPermissions(ctx, loginUser, &snapshot, version)
}

func loadPermissionSnapshot(ctx context.Context, userID int64) (permissionSnapshot, error) {
	snapshots, err := loadPermissionSnapshots(ctx, []int64{userID})
	if err != nil {
		return permissionSnapshot{}, err
	}
	snapshot, exists := snapshots[userID]
	if !exists {
		return permissionSnapshot{}, fmt.Errorf("用户 %d 不存在", userID)
	}
	return snapshot, nil
}

// loadPermissionSnapshots loads all affected users, relations and menu
// permissions in a fixed number of queries instead of one query group per
// online user.
func loadPermissionSnapshots(ctx context.Context, userIDs []int64) (map[int64]permissionSnapshot, error) {
	users, err := repository.SelectUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	roleIDs := make([]int64, 0)
	usersWithoutRoles := make([]int64, 0)
	for userID, user := range users {
		if user.IsAdmin() {
			continue
		}
		if len(user.Roles) == 0 {
			usersWithoutRoles = append(usersWithoutRoles, userID)
			continue
		}
		for _, role := range user.Roles {
			if role.Status == model.StatusNormal {
				roleIDs = append(roleIDs, role.RoleID)
			}
		}
	}
	rolePermissions, err := repository.SelectMenuPermsByRoleIDs(ctx, uniquePositiveIDs(roleIDs))
	if err != nil {
		return nil, err
	}
	userPermissions, err := repository.SelectMenuPermsByUserIDs(ctx, usersWithoutRoles)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]permissionSnapshot, len(users))
	for userID, user := range users {
		if user.IsAdmin() {
			result[userID] = permissionSnapshot{user: user, permissions: []string{model.AllPermission}}
			continue
		}
		permissionSet := make(map[string]struct{})
		if len(user.Roles) == 0 {
			for _, permission := range userPermissions[userID] {
				addSplit(permissionSet, permission)
			}
		} else {
			for i := range user.Roles {
				role := &user.Roles[i]
				if role.Status != model.StatusNormal {
					role.Permissions = nil
					continue
				}
				roleSet := make(map[string]struct{})
				for _, permission := range rolePermissions[role.RoleID] {
					addSplit(roleSet, permission)
					addSplit(permissionSet, permission)
				}
				role.Permissions = sortedKeys(roleSet)
			}
		}
		result[userID] = permissionSnapshot{user: user, permissions: sortedKeys(permissionSet)}
	}
	return result, nil
}

func refreshSessionPermissions(ctx context.Context, loginUser *model.LoginUser,
	snapshot *permissionSnapshot, permissionVersion int64) error {
	err := refreshSessionPermissionsRaw(ctx, loginUser, snapshot, permissionVersion)
	if err == nil {
		return nil
	}
	return failClosedSessionRefresh(ctx, loginUser, err)
}

func refreshSessionPermissionsRaw(ctx context.Context, loginUser *model.LoginUser,
	snapshot *permissionSnapshot, permissionVersion int64) error {
	current := loginUser
	for attempt := 0; attempt < maxSessionRefreshRetries; attempt++ {
		err := writeSessionPermissions(ctx, current, *snapshot, permissionVersion)
		if err == nil {
			if current != loginUser {
				*loginUser = *current
			}
			return nil
		}
		if !errors.Is(err, errSessionConflict) {
			return err
		}

		latest, err := GetLoginUserByKey(ctx, current.Token)
		if err != nil {
			return err
		}
		if latest == nil {
			return errSessionChanged
		}
		loaded, err := loadPermissionSnapshot(ctx, latest.UserID)
		if err != nil {
			return err
		}
		current = latest
		*snapshot = loaded
		permissionVersion, err = PermissionVersion(ctx, latest.UserID)
		if err != nil {
			return err
		}
	}
	return errSessionConflict
}

func writeSessionPermissions(ctx context.Context, loginUser *model.LoginUser,
	snapshot permissionSnapshot, permissionVersion int64) error {
	now := time.Now()
	updated := *loginUser
	updated.User = snapshot.user
	updated.DeptID = snapshot.user.DeptID
	updated.Permissions = snapshot.permissions
	updated.LoginTime = now.UnixMilli()
	updated.ExpireTime = now.Add(jwtExpire).UnixMilli()
	updated.SessionRevision++
	updated.PermissionVersion = permissionVersion
	data, err := json.Marshal(&updated)
	if err != nil {
		return fmt.Errorf("序列化用户权限快照失败: %w", err)
	}

	result, err := updateSessionPermissionsScript.Run(ctx, redisx.C(), sessionKeys(loginUser),
		jwtExpire.Milliseconds(), loginUser.Token, loginUser.SessionGeneration,
		data, loginUser.SessionRevision, permissionVersion).Int64()
	if err != nil {
		return fmt.Errorf("原子更新登录权限失败: %w", err)
	}
	switch result {
	case 1:
		*loginUser = updated
		return nil
	case 2, -3:
		return errSessionConflict
	case 0, -1:
		return errSessionChanged
	default:
		return fmt.Errorf("Redis 登录会话内容无效")
	}
}

func failClosedSessionRefresh(ctx context.Context, loginUser *model.LoginUser, cause error) error {
	_ = ctx
	if loginUser == nil || loginUser.Token == "" {
		return cause
	}
	if err := permissionRevocations.compensate(loginUser.UserID); err != nil {
		return fmt.Errorf("刷新权限失败且安全撤销会话失败: %w", errors.Join(cause, err))
	}
	return fmt.Errorf("刷新权限失败，已安全撤销用户全部会话: %w", cause)
}

func failClosedUserRefresh(ctx context.Context, userID int64, cause error) error {
	_ = ctx
	if err := permissionRevocations.compensate(userID); err != nil {
		return fmt.Errorf("加载用户权限失败且安全撤销会话失败: %w", errors.Join(cause, err))
	}
	return fmt.Errorf("加载用户权限失败，已安全撤销全部会话: %w", cause)
}

// RefreshOnlineUsersByRole 角色权限变更后，刷新所有持有该角色的在线用户。
//
// 先从数据库取角色成员，再走新版用户会话反向索引。这样 Redis 工作量只和
// 受影响用户数相关，也不会因为全站 SCAN/MGET 某一批失败而静默漏掉会话。
func RefreshOnlineUsersByRole(ctx context.Context, roleID int64) error {
	userIDs, err := repository.SelectUserIDsByRoleID(ctx, roleID)
	if err != nil {
		return err
	}
	return RefreshOnlineUsersByID(ctx, userIDs...)
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
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()

	ids := uniquePositiveIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	versions, err := advancePermissionVersions(ctx, ids)
	if err != nil {
		return err
	}
	return refreshOnlineUsersByIDLocked(ctx, ids, versions)
}

// mutatePermissionState 串行化“推进权限版本 -> 写数据库 -> 主动刷新会话”。
// Redis 不可用时数据库写入不会发生；数据库写失败时多推进一次版本是安全的，
// 下一次鉴权只会重新加载仍未变化的数据库权限。
func mutatePermissionState(ctx context.Context, userIDs []int64, mutate func() error) error {
	ids := uniquePositiveIDs(userIDs)
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	return runPermissionMutation(ctx, ids, mutate, advancePermissionVersions, refreshOnlineUsersByIDLocked)
}

func runPermissionMutation(
	ctx context.Context,
	ids []int64,
	mutate func() error,
	advance func(context.Context, []int64) (map[int64]int64, error),
	refresh func(context.Context, []int64, map[int64]int64) error,
) error {
	versions, err := advance(ctx, ids)
	if err != nil {
		return err
	}
	if err := mutate(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return runPostCommit(ctx, func(postCtx context.Context) error {
		return refresh(postCtx, ids, versions)
	})
}

func refreshOnlineUsersByIDLocked(ctx context.Context, ids []int64, versions map[int64]int64) error {
	permissionRevocations.mark(ids...)

	sessions, err := indexedLoginUsers(ctx, ids)
	if err != nil {
		revokeErr := permissionRevocations.compensate(ids...)
		return fmt.Errorf("读取在线会话失败: %w", errors.Join(err, revokeErr))
	}
	onlineIDs := make([]int64, 0, len(sessions))
	for _, userID := range ids {
		loginUsers := sessions[userID]
		if len(loginUsers) == 0 {
			permissionRevocations.clear(userID)
			continue
		}
		onlineIDs = append(onlineIDs, userID)
	}
	snapshots, err := loadPermissionSnapshots(ctx, onlineIDs)
	if err != nil {
		revokeErr := permissionRevocations.compensate(onlineIDs...)
		return fmt.Errorf("批量加载用户权限失败: %w", errors.Join(err, revokeErr))
	}
	var firstErr error
	failedIDs := make([]int64, 0)
	for _, userID := range onlineIDs {
		loginUsers := sessions[userID]
		snapshot, exists := snapshots[userID]
		if !exists {
			err := fmt.Errorf("用户 %d 不存在", userID)
			slog.Warn("重新加载用户失败，等待安全撤销会话", "userId", userID, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			failedIDs = append(failedIDs, userID)
			continue
		}
		failed := false
		for _, loginUser := range loginUsers {
			if err := refreshSessionPermissionsRaw(ctx, loginUser, &snapshot, versions[userID]); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				slog.Warn("刷新会话失败，等待安全撤销", "userId", userID, "err", err)
				failed = true
				failedIDs = append(failedIDs, userID)
				break
			}
		}
		if !failed {
			permissionRevocations.clear(userID)
		}
	}
	if len(failedIDs) > 0 {
		revokeErr := permissionRevocations.compensate(failedIDs...)
		if revokeErr != nil {
			firstErr = errors.Join(firstErr, revokeErr)
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
	type indexedToken struct {
		userID  int64
		tokenID string
	}

	result := make(map[int64][]*model.LoginUser, len(userIDs))
	const indexBatchSize = 200
	const tokenBatchSize = 100
	err := forEachIDBatch(userIDs, indexBatchSize, func(batch []int64) error {
		commands := make([]indexCommand, 0, len(batch))
		_, err := redisx.C().Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, userID := range batch {
				commands = append(commands, indexCommand{
					userID: userID,
					cmd:    pipe.SMembers(ctx, redisx.LoginUserSessionsKey(userID)),
				})
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("批量读取用户会话索引失败: %w", err)
		}

		stale := make(map[int64][]string, len(batch))
		tokens := make([]indexedToken, 0, tokenBatchSize)
		flushTokens := func() error {
			if len(tokens) == 0 {
				return nil
			}
			keys := make([]string, len(tokens))
			for i, token := range tokens {
				keys[i] = redisx.LoginTokenKey(token.tokenID)
			}
			values, err := redisx.C().MGet(ctx, keys...).Result()
			if err != nil {
				return fmt.Errorf("批量读取用户登录会话失败: %w", err)
			}
			for i, value := range values {
				token := tokens[i]
				raw, ok := value.(string)
				if !ok {
					stale[token.userID] = append(stale[token.userID], token.tokenID)
					continue
				}
				var loginUser model.LoginUser
				if err := json.Unmarshal([]byte(raw), &loginUser); err != nil {
					slog.Warn("会话反序列化失败，跳过", "err", err)
					stale[token.userID] = append(stale[token.userID], token.tokenID)
					continue
				}
				if loginUser.UserID != token.userID {
					stale[token.userID] = append(stale[token.userID], token.tokenID)
					continue
				}
				if loginUser.Token == "" {
					loginUser.Token = token.tokenID
				}
				result[token.userID] = append(result[token.userID], &loginUser)
			}
			tokens = tokens[:0]
			return nil
		}

		for _, command := range commands {
			members, err := command.cmd.Result()
			if err != nil {
				return fmt.Errorf("读取用户 %d 会话索引失败: %w", command.userID, err)
			}
			for _, tokenID := range members {
				if tokenID == "" {
					continue
				}
				tokens = append(tokens, indexedToken{userID: command.userID, tokenID: tokenID})
				if len(tokens) == tokenBatchSize {
					if err := flushTokens(); err != nil {
						return err
					}
				}
			}
		}
		if err := flushTokens(); err != nil {
			return err
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
				return fmt.Errorf("清理失效用户会话索引失败: %w", err)
			}
		}
		return nil
	})
	return result, err
}

func forEachIDBatch(ids []int64, size int, fn func([]int64) error) error {
	if size <= 0 {
		return errors.New("批次大小必须大于0")
	}
	for start := 0; start < len(ids); start += size {
		if err := fn(ids[start:min(start+size, len(ids))]); err != nil {
			return err
		}
	}
	return nil
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
func scanLoginUsers(ctx context.Context, batch int64, maxKeys int,
	fn func(*model.LoginUser) error) (bool, error) {
	return scanLoginUserEntries(ctx, batch, maxKeys, func(_ string, loginUser *model.LoginUser) error {
		return fn(loginUser)
	})
}

func scanLoginUserEntries(ctx context.Context, batch int64, maxKeys int,
	fn func(string, *model.LoginUser) error) (bool, error) {
	return redisx.ScanKeyBatchesBounded(ctx, redisx.KeyLoginToken, batch, maxKeys, func(keys []string) error {
		values, err := redisx.C().MGet(ctx, keys...).Result()
		if err != nil {
			return fmt.Errorf("批量读取在线会话失败(count=%d): %w", len(keys), err)
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
	failures := revokeUserSessionsBatch(ctx, ids)
	if len(failures) == 0 {
		return nil
	}
	return permissionRevokeSummary(failures, 0, 0)
}

// revokeUserSessionsBatch 用 Pipeline 分批执行每用户原子撤销脚本，避免大角色
// 在 Redis 故障时产生逐用户网络往返。返回值只保留失败用户，不创建成功项。
func revokeUserSessionsBatch(ctx context.Context, userIDs []int64) map[int64]error {
	ids := uniquePositiveIDs(userIDs)
	failures := make(map[int64]error)
	const batchSize = 200
	_ = forEachIDBatch(ids, batchSize, func(batch []int64) error {
		type command struct {
			userID int64
			cmd    *redis.Cmd
		}
		commands := make([]command, 0, len(batch))
		_, pipelineErr := redisx.C().Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, userID := range batch {
				keys := []string{
					redisx.LoginUserGenerationKey(userID),
					redisx.LoginUserSessionsKey(userID),
				}
				commands = append(commands, command{
					userID: userID,
					cmd: pipe.Eval(ctx, revokeSessionsLua, keys,
						redisx.KeyLoginToken, jwtExpire.Milliseconds()),
				})
			}
			return nil
		})
		for _, command := range commands {
			if err := command.cmd.Err(); err != nil {
				failures[command.userID] = err
			} else if pipelineErr != nil {
				// A pipeline-level transport failure can make an apparently empty
				// command result ambiguous, so keep that user pending.
				failures[command.userID] = pipelineErr
			}
		}
		return nil
	})
	return failures
}
