package apitest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
)

// TestUserNameCollationContract 记录当前测试库账号列的匹配语义。
// 登录错误计数必须跟随查询实际命中的规范账号，不能自行猜测数据库排序规则。
func TestUserNameCollationContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var collation string
	if err := repository.DB(ctx).Raw(`
		SELECT COLLATION_NAME
		  FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE()
		   AND TABLE_NAME = 'sys_user'
		   AND COLUMN_NAME = 'user_name'`).Scan(&collation).Error; err != nil {
		t.Fatalf("查询 sys_user.user_name 排序规则失败：%v", err)
	}
	if strings.TrimSpace(collation) == "" {
		t.Fatal("未查询到 sys_user.user_name 的排序规则")
	}

	var upperAdminMatches int64
	if err := repository.DB(ctx).Raw(
		"SELECT COUNT(*) FROM sys_user WHERE user_name = ? AND del_flag = ?", "ADMIN", "0").
		Scan(&upperAdminMatches).Error; err != nil {
		t.Fatalf("验证用户名大小写匹配语义失败：%v", err)
	}
	t.Logf("sys_user.user_name collation=%s, user_name='ADMIN' matches=%d", collation, upperAdminMatches)
}

// TestPasswordFailureLockAndCanonicalUserName 锁定阈值、滑动 TTL、大小写共用计数和解锁。
func TestPasswordFailureLockAndCanonicalUserName(t *testing.T) {
	body := newUserPayload("retry_lock")
	id := createUser(t, body)
	userName := fmt.Sprint(body["userName"])
	upperName := strings.ToUpper(userName)
	retryKey := redisx.PwdErrCntKey(userName)
	upperRetryKey := redisx.PwdErrCntKey(upperName)
	trackRedisKey(retryKey)
	trackRedisKey(upperRetryKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := redisx.C().Del(ctx, retryKey, upperRetryKey).Err(); err != nil {
		t.Fatalf("清理密码错误计数失败：%v", err)
	}

	caseInsensitive, err := userNameMatches(ctx, upperName, id)
	if err != nil {
		t.Fatal(err)
	}
	variants := []string{userName}
	unlockName := userName
	if caseInsensitive {
		variants = []string{upperName, userName}
		unlockName = upperName
	}

	firstErr := loginWithFreshCaptcha(t, ctx, variants[0], "wrong-password")
	if firstErr == nil || !strings.Contains(firstErr.Error(), "用户不存在/密码错误") {
		t.Fatalf("第一次密码错误应返回统一提示，实际=%v", firstErr)
	}
	time.Sleep(80 * time.Millisecond)
	ttlBefore, err := redisx.C().PTTL(ctx, retryKey).Result()
	if err != nil {
		t.Fatalf("读取第一次失败计数 TTL 失败：%v", err)
	}

	secondErr := loginWithFreshCaptcha(t, ctx, variants[1%len(variants)], "wrong-password")
	if secondErr == nil || !strings.Contains(secondErr.Error(), "用户不存在/密码错误") {
		t.Fatalf("第二次密码错误应返回统一提示，实际=%v", secondErr)
	}
	ttlAfter, err := redisx.C().PTTL(ctx, retryKey).Result()
	if err != nil {
		t.Fatalf("读取第二次失败计数 TTL 失败：%v", err)
	}
	if ttlAfter <= ttlBefore+40*time.Millisecond {
		t.Errorf("每次失败都应重置 10 分钟滑动窗口，失败前 TTL=%s，失败后 TTL=%s", ttlBefore, ttlAfter)
	}

	for attempt := 3; attempt <= 5; attempt++ {
		err := loginWithFreshCaptcha(t, ctx, variants[(attempt-1)%len(variants)], "wrong-password")
		if err == nil {
			t.Fatalf("第 %d 次错误密码不应登录成功", attempt)
		}
		if attempt < 5 && !strings.Contains(err.Error(), "用户不存在/密码错误") {
			t.Errorf("第 %d 次错误密码提示不符：%v", attempt, err)
		}
		if attempt == 5 && !strings.Contains(err.Error(), "帐户锁定10分钟") {
			t.Errorf("第 5 次错误后应立即提示锁定，实际=%v", err)
		}
	}

	count, err := redisx.C().Get(ctx, retryKey).Int()
	if err != nil || count != 5 {
		t.Fatalf("5 次错误后规范账号计数应为 5，实际 count=%d err=%v", count, err)
	}
	if caseInsensitive {
		if exists, err := redisx.C().Exists(ctx, upperRetryKey).Result(); err != nil || exists != 0 {
			t.Errorf("大小写变体不应创建第二个计数键，exists=%d err=%v", exists, err)
		}
	}

	if err := loginWithFreshCaptcha(t, ctx, variants[0], "test123456"); err == nil || !strings.Contains(err.Error(), "帐户锁定10分钟") {
		t.Fatalf("锁定期间即使密码正确也不能登录，实际=%v", err)
	}
	if err := service.UnlockAccount(ctx, unlockName); err != nil {
		t.Fatalf("解锁账号失败：%v", err)
	}
	if exists, err := redisx.C().Exists(ctx, retryKey).Result(); err != nil || exists != 0 {
		t.Fatalf("解锁后规范账号计数键应删除，exists=%d err=%v", exists, err)
	}

	token, err := service.Login(ctx, loginBodyWithFreshCaptcha(t, ctx, unlockName, "test123456"),
		"192.0.2.1", "Go security test")
	if err != nil {
		t.Fatalf("解锁后应能用正确密码登录：%v", err)
	}
	trackLoginToken(t, token)
}

// TestPasswordFailureCounterIsAtomic 并发错误请求不能因 GET+SET 覆盖而少计数。
func TestPasswordFailureCounterIsAtomic(t *testing.T) {
	body := newUserPayload("retry_concurrent")
	createUser(t, body)
	userName := fmt.Sprint(body["userName"])
	retryKey := redisx.PwdErrCntKey(userName)
	trackRedisKey(retryKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := redisx.C().Del(ctx, retryKey).Err(); err != nil {
		t.Fatalf("清理并发测试计数失败：%v", err)
	}

	const attempts = 5
	bodies := make([]model.LoginBody, attempts)
	for i := range bodies {
		bodies[i] = loginBodyWithFreshCaptcha(t, ctx, userName, "wrong-password")
	}

	start := make(chan struct{})
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range bodies {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			attemptCtx, attemptCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer attemptCancel()
			_, errs[index] = service.Login(attemptCtx, bodies[index], "192.0.2.1", "Go concurrent test")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			t.Errorf("第 %d 个并发错误密码请求不应成功", i+1)
		}
	}
	count, err := redisx.C().Get(ctx, retryKey).Int()
	if err != nil || count != attempts {
		t.Fatalf("%d 个并发错误请求必须原子累计为 %d，实际 count=%d err=%v",
			attempts, attempts, count, err)
	}
	ttl, err := redisx.C().PTTL(ctx, retryKey).Result()
	if err != nil || ttl < 9*time.Minute || ttl > 10*time.Minute {
		t.Errorf("并发失败后的 TTL 应接近 10 分钟，实际 ttl=%s err=%v", ttl, err)
	}
}

func userNameMatches(ctx context.Context, input string, userID int64) (bool, error) {
	var count int64
	err := repository.DB(ctx).Raw(
		"SELECT COUNT(*) FROM sys_user WHERE user_name = ? AND user_id = ? AND del_flag = ?",
		input, userID, "0").Scan(&count).Error
	return count == 1, err
}

func loginBodyWithFreshCaptcha(t *testing.T, ctx context.Context, userName, password string) model.LoginBody {
	t.Helper()
	id, _, err := service.GenerateCaptcha(ctx)
	if err != nil {
		t.Fatalf("生成登录测试验证码失败：%v", err)
	}
	trackRedisKey(redisx.CaptchaKey(id))
	answer, err := redisx.C().Get(ctx, redisx.CaptchaKey(id)).Result()
	if err != nil {
		t.Fatalf("读取登录测试验证码答案失败：%v", err)
	}
	return model.LoginBody{Username: userName, Password: password, UUID: id, Code: answer}
}

func loginWithFreshCaptcha(t *testing.T, ctx context.Context, userName, password string) error {
	t.Helper()
	_, err := service.Login(ctx, loginBodyWithFreshCaptcha(t, ctx, userName, password),
		"192.0.2.1", "Go security test")
	return err
}

func trackLoginToken(t *testing.T, token string) {
	t.Helper()
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatalf("解析测试登录 token 失败：%v", err)
	}
	trackRedisKey(redisx.LoginTokenKey(claims.LoginUserKey))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loginUser, err := service.GetLoginUser(ctx, token)
	if err != nil {
		t.Fatalf("读取测试登录会话失败：%v", err)
	}
	if loginUser != nil {
		trackRedisKey(redisx.LoginUserSessionsKey(loginUser.UserID))
		trackRedisKey(redisx.LoginUserGenerationKey(loginUser.UserID))
	}
}
