package apitest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
)

// TestSessionGenerationRejectsStaleCreate 固定“认证完成 -> 撤销 -> 写会话”的顺序，
// 证明并发中的旧认证结果不能在撤销完成后重新落回 Redis。
func TestSessionGenerationRejectsStaleCreate(t *testing.T) {
	body := newUserPayload("session_generation")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	ctx := context.Background()
	loginUser, err := service.GetLoginUser(ctx, token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取撤销前会话失败：loginUser=%v err=%v", loginUser, err)
	}
	if err := service.RevokeUserSessions(ctx, id); err != nil {
		t.Fatalf("撤销会话失败：%v", err)
	}
	if _, err := service.CreateToken(ctx, loginUser); err == nil || !strings.Contains(err.Error(), "登录状态已变更") {
		t.Fatalf("旧代数不应再创建会话，实际 err=%v", err)
	}
	assertTokenUnauthorized(t, token, "代数递增")
}

// TestRevokeUserSessionsRejectsUnindexedSessionByGeneration 证明撤销不需要兼容 SCAN：
// 即使存在部署时本应清理的无索引残余，代数递增也会让它无法继续鉴权。
func TestRevokeUserSessionsRejectsUnindexedSessionByGeneration(t *testing.T) {
	body := newUserPayload("legacy_session")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	ctx := context.Background()
	if err := redisx.C().Del(ctx,
		redisx.LoginUserSessionsKey(id),
		redisx.LoginUserGenerationKey(id),
	).Err(); err != nil {
		t.Fatalf("构造旧格式会话失败：%v", err)
	}
	if err := service.RevokeUserSessions(ctx, id); err != nil {
		t.Fatalf("撤销无索引会话失败：%v", err)
	}
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("解析测试 token 失败：%v", err)
	}
	remaining, err := redisx.C().Exists(ctx, redisx.LoginTokenKey(claims.LoginUserKey)).Result()
	if err != nil {
		t.Fatalf("检查残余会话失败：%v", err)
	}
	if remaining != 1 {
		t.Fatal("撤销不应全量扫描无索引会话；该残余应由会话代数拒绝并按部署文档清理")
	}
	assertTokenUnauthorized(t, token, "撤销无索引会话")
}

// TestRefreshOnlineUsersByIDUsesSessionIndex 覆盖批量、多会话、重复用户 ID、
// 失效索引清理，以及不再为无索引旧会话执行全量扫描。
func TestRefreshOnlineUsersByIDUsesSessionIndex(t *testing.T) {
	first := newUserPayload("refresh_index_first")
	firstID := createUser(t, first)
	second := newUserPayload("refresh_index_second")
	secondID := createUser(t, second)

	firstIndexed, err := loginAs(fmt.Sprint(first["userName"]), "test123456")
	if err != nil {
		t.Fatalf("第一个账号首次登录失败：%v", err)
	}
	firstUnindexed, err := loginAs(fmt.Sprint(first["userName"]), "test123456")
	if err != nil {
		t.Fatalf("第一个账号再次登录失败：%v", err)
	}
	secondIndexed, err := loginAs(fmt.Sprint(second["userName"]), "test123456")
	if err != nil {
		t.Fatalf("第二个账号登录失败：%v", err)
	}

	firstIndexedID := overwriteSessionPermissions(t, firstIndexed, []string{"stale:permission"})
	firstUnindexedID := overwriteSessionPermissions(t, firstUnindexed, []string{"stale:permission"})
	overwriteSessionPermissions(t, secondIndexed, []string{"stale:permission"})

	ctx := context.Background()
	firstIndexKey := redisx.LoginUserSessionsKey(firstID)
	if err := redisx.C().SRem(ctx, firstIndexKey, firstUnindexedID).Err(); err != nil {
		t.Fatalf("构造无索引残余会话失败：%v", err)
	}
	const expiredTokenID = "expired-refresh-index-test"
	if err := redisx.C().SAdd(ctx, firstIndexKey, expiredTokenID).Err(); err != nil {
		t.Fatalf("构造失效索引失败：%v", err)
	}

	if err := service.RefreshOnlineUsersByID(ctx, firstID, secondID, firstID, 0, -1); err != nil {
		t.Fatalf("批量刷新在线用户失败：%v", err)
	}
	assertSessionHasPermission(t, firstIndexed, "stale:permission", false)
	assertSessionHasPermission(t, secondIndexed, "stale:permission", false)
	assertSessionHasPermission(t, firstUnindexed, "stale:permission", true)
	assertSessionRevision(t, firstIndexed, 1)
	assertSessionRevision(t, secondIndexed, 1)
	assertSessionRevision(t, firstUnindexed, 0)

	stale, err := redisx.C().SIsMember(ctx, firstIndexKey, expiredTokenID).Result()
	if err != nil {
		t.Fatalf("检查失效索引清理结果失败：%v", err)
	}
	if stale {
		t.Fatal("过期 token 对应的用户会话索引成员应被清理")
	}
	indexed, err := redisx.C().SIsMember(ctx, firstIndexKey, firstIndexedID).Result()
	if err != nil || !indexed {
		t.Fatalf("有效会话索引不应被误删：indexed=%v err=%v", indexed, err)
	}
}

// TestRefreshTokenPreservesLatestPermissionSnapshot 固定“请求读旧会话 -> 管理员写新权限
// -> 旧请求续期”的顺序，证明续期只更新时间和 TTL，不会整体覆盖最新权限。
func TestRefreshTokenPreservesLatestPermissionSnapshot(t *testing.T) {
	body := newUserPayload("renew_perm")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	ctx := context.Background()
	stale, err := service.GetLoginUser(ctx, token)
	if err != nil || stale == nil {
		t.Fatalf("读取旧会话快照失败：loginUser=%v err=%v", stale, err)
	}
	latest, err := service.GetLoginUser(ctx, token)
	if err != nil || latest == nil {
		t.Fatalf("读取当前会话失败：loginUser=%v err=%v", latest, err)
	}
	latest.Permissions = []string{"fresh:permission"}
	latest.SessionRevision = 7
	latest.ExpireTime = time.Now().Add(time.Minute).UnixMilli()
	writeLoginUser(t, latest)

	if err := service.RefreshToken(ctx, stale); err != nil {
		t.Fatalf("旧快照续期失败：%v", err)
	}
	got, err := service.GetLoginUser(ctx, token)
	if err != nil || got == nil {
		t.Fatalf("读取续期后会话失败：loginUser=%v err=%v", got, err)
	}
	if !hasPermission(got, "fresh:permission") {
		t.Fatalf("旧快照续期覆盖了最新权限：%v", got.Permissions)
	}
	if got.SessionRevision != 7 {
		t.Fatalf("普通续期不应修改 sessionRevision，实际 %d", got.SessionRevision)
	}
	assertSessionTTLsAligned(t, got, id)
}

// TestRefreshLoginUserPermissionsRetriesRevisionConflict 固定两个刷新使用同一个旧 revision：
// 后到的刷新必须检测冲突、重新加载数据库权限后再写，不能使用调用方的旧权限快照。
func TestRefreshLoginUserPermissionsRetriesRevisionConflict(t *testing.T) {
	body := newUserPayload("rev_retry")
	createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	ctx := context.Background()
	stale, err := service.GetLoginUser(ctx, token)
	if err != nil || stale == nil {
		t.Fatalf("读取旧会话快照失败：loginUser=%v err=%v", stale, err)
	}
	first, err := service.GetLoginUser(ctx, token)
	if err != nil || first == nil {
		t.Fatalf("读取首次刷新快照失败：loginUser=%v err=%v", first, err)
	}
	if err := service.RefreshLoginUserPermissions(ctx, first); err != nil {
		t.Fatalf("首次权限刷新失败：%v", err)
	}
	if first.SessionRevision != 1 {
		t.Fatalf("首次权限刷新 revision=%d，期望 1", first.SessionRevision)
	}

	stale.Permissions = []string{"stale:must-not-return"}
	if err := service.RefreshLoginUserPermissions(ctx, stale); err != nil {
		t.Fatalf("revision 冲突后重试失败：%v", err)
	}
	if stale.SessionRevision != 2 {
		t.Fatalf("冲突重试后 revision=%d，期望 2", stale.SessionRevision)
	}
	if hasPermission(stale, "stale:must-not-return") {
		t.Fatalf("冲突重试错误写回了调用方旧权限：%v", stale.Permissions)
	}
}

// TestPermissionRefreshFailsClosedOnInvalidRevision 模拟并发写入无法解析的 revision。
// 无法确认新旧权限时必须删除会话，而不是继续保留不确定权限。
func TestPermissionRefreshFailsClosedOnInvalidRevision(t *testing.T) {
	body := newUserPayload("fail_closed")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	ctx := context.Background()
	stale, err := service.GetLoginUser(ctx, token)
	if err != nil || stale == nil {
		t.Fatalf("读取旧会话快照失败：loginUser=%v err=%v", stale, err)
	}
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("解析测试 token 失败：%v", err)
	}
	key := redisx.LoginTokenKey(claims.LoginUserKey)
	var raw map[string]any
	data, err := redisx.C().Get(ctx, key).Bytes()
	if err != nil || json.Unmarshal(data, &raw) != nil {
		t.Fatalf("读取测试会话 JSON 失败：%v", err)
	}
	raw["sessionRevision"] = "invalid"
	corrupted, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("构造异常 revision 失败：%v", err)
	}
	if err := redisx.C().Set(ctx, key, corrupted, appConfig.JWT.ExpireTime).Err(); err != nil {
		t.Fatalf("写入异常 revision 失败：%v", err)
	}

	err = service.RefreshLoginUserPermissions(ctx, stale)
	if err == nil || !strings.Contains(err.Error(), "已安全撤销会话") {
		t.Fatalf("异常 revision 应触发安全撤销，实际 err=%v", err)
	}
	if exists, err := redisx.C().Exists(ctx, key).Result(); err != nil || exists != 0 {
		t.Fatalf("异常会话应被删除：exists=%d err=%v", exists, err)
	}
	if indexed, err := redisx.C().SIsMember(ctx, redisx.LoginUserSessionsKey(id),
		claims.LoginUserKey).Result(); err != nil || indexed {
		t.Fatalf("异常会话索引应被删除：indexed=%v err=%v", indexed, err)
	}
}

func overwriteSessionPermissions(t *testing.T, token string, permissions []string) string {
	t.Helper()
	ctx := context.Background()
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("解析测试 token 失败：%v", err)
	}
	loginUser, err := service.GetLoginUser(ctx, token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取测试会话失败：loginUser=%v err=%v", loginUser, err)
	}
	loginUser.Permissions = permissions
	writeLoginUser(t, loginUser)
	return claims.LoginUserKey
}

func assertSessionHasPermission(t *testing.T, token, permission string, want bool) {
	t.Helper()
	loginUser, err := service.GetLoginUser(context.Background(), token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取刷新后的测试会话失败：loginUser=%v err=%v", loginUser, err)
	}
	got := false
	for _, actual := range loginUser.Permissions {
		if actual == permission {
			got = true
			break
		}
	}
	if got != want {
		t.Fatalf("会话权限 %q 存在状态应为 %v，实际权限=%v", permission, want, loginUser.Permissions)
	}
}

func assertSessionRevision(t *testing.T, token string, want int64) {
	t.Helper()
	loginUser, err := service.GetLoginUser(context.Background(), token)
	if err != nil || loginUser == nil {
		t.Fatalf("读取测试会话 revision 失败：loginUser=%v err=%v", loginUser, err)
	}
	if loginUser.SessionRevision != want {
		t.Fatalf("sessionRevision=%d，期望 %d", loginUser.SessionRevision, want)
	}
}

func writeLoginUser(t *testing.T, loginUser *model.LoginUser) {
	t.Helper()
	raw, err := json.Marshal(loginUser)
	if err != nil {
		t.Fatalf("序列化测试会话失败：%v", err)
	}
	if err := redisx.C().Set(context.Background(), redisx.LoginTokenKey(loginUser.Token), raw,
		appConfig.JWT.ExpireTime).Err(); err != nil {
		t.Fatalf("写入测试会话失败：%v", err)
	}
}

func hasPermission(loginUser *model.LoginUser, permission string) bool {
	return loginUser.HasPermission(permission)
}

func assertSessionTTLsAligned(t *testing.T, loginUser *model.LoginUser, userID int64) {
	t.Helper()
	ctx := context.Background()
	ttls := map[string]time.Duration{}
	for name, key := range map[string]string{
		"token":      redisx.LoginTokenKey(loginUser.Token),
		"userIndex":  redisx.LoginUserSessionsKey(userID),
		"generation": redisx.LoginUserGenerationKey(userID),
	} {
		ttl, err := redisx.C().PTTL(ctx, key).Result()
		if err != nil {
			t.Fatalf("读取 %s TTL 失败：%v", name, err)
		}
		if ttl < appConfig.JWT.ExpireTime-5*time.Second || ttl > appConfig.JWT.ExpireTime {
			t.Fatalf("%s TTL=%v，期望接近 %v", name, ttl, appConfig.JWT.ExpireTime)
		}
		ttls[name] = ttl
	}
	jsonRemaining := time.Until(time.UnixMilli(loginUser.ExpireTime))
	if (jsonRemaining - ttls["token"]).Abs() > 2*time.Second {
		t.Fatalf("JSON expireTime 与 token TTL 不一致：json=%v ttl=%v", jsonRemaining, ttls["token"])
	}
	if (ttls["userIndex"]-ttls["token"]).Abs() > 2*time.Second ||
		(ttls["generation"]-ttls["token"]).Abs() > 2*time.Second {
		t.Fatalf("会话三个 TTL 不一致：%v", ttls)
	}
}
