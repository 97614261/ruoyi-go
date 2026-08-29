package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/redisx"
)

// 密码重试限制，对应 Java 版 user.password.maxRetryCount / lockTime。
const (
	maxPasswordRetry = 5
	passwordLockTime = 10 * time.Minute
)

const ConfigKeyLoginBlackIPList = "sys.login.blackIPList"

// msgPasswordNotMatch 用户不存在与密码错误必须返回同一句话，
// 否则攻击者可以据此枚举出系统里有哪些账号。
const msgPasswordNotMatch = "用户不存在/密码错误"

// passwordFailureScript 原子累加失败次数，并保持原有的滑动窗口语义：
// 每次失败都从当前时刻重新计算锁定 TTL。
var passwordFailureScript = redis.NewScript(`
local current = redis.call('incr', KEYS[1])
redis.call('pexpire', KEYS[1], ARGV[1])
return current
`)

// passwordSuccessScript 只在尚未达到锁定阈值时清除失败计数。
// 避免正确密码请求与并发失败请求交错时，把已经形成的锁定错误清掉。
var passwordSuccessScript = redis.NewScript(`
local current = tonumber(redis.call('get', KEYS[1]) or '0')
if current >= tonumber(ARGV[1]) then
    return current
end
redis.call('del', KEYS[1])
return current
`)

// Login 执行登录，成功返回 token。
//
// 每个失败分支都要写登录日志 —— 登录日志页面靠它，
// 而且"某账号连续失败 N 次"是排查撞库的主要线索。
func Login(ctx context.Context, body model.LoginBody, ip, userAgent string) (string, error) {
	browser, os := parseUserAgent(userAgent)
	// fail 统一记录失败日志并返回错误
	fail := func(err error) (string, error) {
		msg := err.Error()
		if bizErr := errs.As(err); bizErr != nil {
			msg = bizErr.Msg
		}
		RecordLogininfor(ctx, body.Username, ip, browser, os, model.LoginStatusFail, msg)
		return "", err
	}
	if err := loginPreCheck(ctx, body.Username, body.Password, ip); err != nil {
		return fail(err)
	}

	enabled, err := CaptchaEnabled(ctx)
	if err != nil {
		return "", err
	}
	if enabled {
		if err := VerifyCaptcha(ctx, body.UUID, body.Code); err != nil {
			return fail(err)
		}
	}

	user, err := repository.SelectUserAccountByUserName(ctx, body.Username)
	if err != nil {
		return "", err
	}
	// 数据库命中后使用库中保存的规范账号名。这样 admin / ADMIN 在
	// *_ci 排序规则下命中同一用户时，也必然共用同一个 Redis 计数键；
	// 在大小写敏感的库中，两个真实存在的不同账号仍各自计数。
	retryUserName := body.Username
	if user != nil {
		retryUserName = user.UserName
	}
	retryKey := redisx.PwdErrCntKey(retryUserName)
	count, err := currentRetryCount(ctx, retryKey)
	if err != nil {
		return "", err
	}
	if count >= maxPasswordRetry {
		return fail(passwordLockedError())
	}
	if user == nil || user.DelFlag == model.DelFlagDeleted {
		return fail(recordPasswordFailure(ctx, retryKey))
	}
	if user.Status == model.StatusDisable {
		return fail(errs.New("对不起，您的帐号已停用"))
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)) != nil {
		return fail(recordPasswordFailure(ctx, retryKey))
	}

	// 先记住会话代数，再回库复核一次。这样停用、删除或改密无论发生在
	// 第一次查询前后，都会被第二次查询或 CreateToken 的原子代数比较拦住。
	generation, err := SessionGeneration(ctx, user.UserID)
	if err != nil {
		return "", err
	}
	fresh, err := repository.SelectUserByID(ctx, user.UserID)
	if err != nil {
		return "", err
	}
	if fresh == nil || fresh.DelFlag == model.DelFlagDeleted || fresh.Status == model.StatusDisable ||
		bcrypt.CompareHashAndPassword([]byte(fresh.Password), []byte(body.Password)) != nil {
		return fail(errSessionChanged)
	}
	user = fresh

	// 正确密码也必须再次原子检查计数：并发失败可能在最初的 GET 之后
	// 已经把账号推到锁定阈值，不能被这次成功请求直接 DEL 掉。
	count, err = clearPasswordFailuresOnSuccess(ctx, retryKey)
	if err != nil {
		return "", err
	}
	if count >= maxPasswordRetry {
		return fail(passwordLockedError())
	}

	permissions, err := GetMenuPermission(ctx, user)
	if err != nil {
		return "", err
	}

	loginUser := &model.LoginUser{
		UserID:            user.UserID,
		DeptID:            user.DeptID,
		IPAddr:            ip,
		Browser:           browser,
		OS:                os,
		Permissions:       permissions,
		User:              user,
		SessionGeneration: generation,
	}

	token, err := CreateToken(ctx, loginUser)
	if err != nil {
		return "", err
	}

	// 记录登录信息失败不影响登录结果，只告警
	if err := repository.UpdateLoginInfo(ctx, user.UserID, ip, time.Now()); err != nil {
		slog.Warn("更新登录信息失败", "userId", user.UserID, "err", err)
	}
	RecordLogininfor(ctx, body.Username, ip, browser, os, model.LoginStatusSuccess, "登录成功")
	return token, nil
}

func loginPreCheck(ctx context.Context, username, password, ip string) error {
	usernameLen := len([]rune(username))
	passwordLen := len([]rune(password))
	if usernameLen < 2 || usernameLen > 20 || passwordLen < 5 || passwordLen > 20 {
		return errs.New(msgPasswordNotMatch)
	}
	filter, err := GetConfigValueByKey(ctx, ConfigKeyLoginBlackIPList)
	if err != nil {
		return err
	}
	if ipMatchesFilter(filter, ip) {
		return errs.New("很遗憾，访问IP已被列入系统黑名单")
	}
	return nil
}

func ipMatchesFilter(filter, value string) bool {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	for _, raw := range strings.Split(filter, ";") {
		rule := strings.TrimSpace(raw)
		if rule == "" {
			continue
		}
		if exact, err := netip.ParseAddr(rule); err == nil && exact == ip {
			return true
		}
		if prefix, err := netip.ParsePrefix(rule); err == nil && prefix.Contains(ip) {
			return true
		}
		if strings.HasSuffix(rule, "*") && strings.HasPrefix(value, strings.TrimSuffix(rule, "*")) {
			return true
		}
		parts := strings.SplitN(rule, "-", 2)
		if len(parts) == 2 {
			start, startErr := netip.ParseAddr(strings.TrimSpace(parts[0]))
			end, endErr := netip.ParseAddr(strings.TrimSpace(parts[1]))
			if startErr == nil && endErr == nil && start.BitLen() == ip.BitLen() &&
				start.Compare(ip) <= 0 && end.Compare(ip) >= 0 {
				return true
			}
		}
	}
	return false
}

// Logout 删除会话。
func Logout(ctx context.Context, loginUserKey string) error {
	return DeleteLoginUser(ctx, loginUserKey)
}

func currentRetryCount(ctx context.Context, key string) (int, error) {
	count, err := redisx.C().Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("读取密码错误次数失败: %w", err)
	}
	return count, nil
}

// recordPasswordFailure 原子累加错误次数并返回统一错误。
func recordPasswordFailure(ctx context.Context, key string) error {
	next, err := passwordFailureScript.Run(ctx, redisx.C(), []string{key},
		passwordLockTime.Milliseconds()).Int64()
	if err != nil {
		return errs.Wrap(err, "记录登录失败次数失败")
	}
	if next >= maxPasswordRetry {
		return passwordLockedError()
	}
	return errs.New(msgPasswordNotMatch)
}

func clearPasswordFailuresOnSuccess(ctx context.Context, key string) (int, error) {
	count, err := passwordSuccessScript.Run(ctx, redisx.C(), []string{key}, maxPasswordRetry).Int()
	if err != nil {
		return 0, fmt.Errorf("确认密码错误次数失败: %w", err)
	}
	return count, nil
}

func passwordLockedError() error {
	return errs.Newf("密码输入错误%d次，帐户锁定%d分钟",
		maxPasswordRetry, int(passwordLockTime.Minutes()))
}

// parseUserAgent 粗略识别浏览器与操作系统。
//
// Java 版用 yauaa 做精确解析，这里只为"在线用户"列表提供可读信息，
// 不值得为此引入一个几十 MB 规则库的依赖。识别不出时返回 Unknown。
func parseUserAgent(ua string) (browser, os string) {
	browser, os = "Unknown", "Unknown"
	if ua == "" {
		return
	}
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	case strings.Contains(ua, "MSIE") || strings.Contains(ua, "Trident/"):
		browser = "IE"
	}
	switch {
	case strings.Contains(ua, "Windows NT 10"):
		os = "Windows 10/11"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Mac OS X"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	return
}
