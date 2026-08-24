package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// msgPasswordNotMatch 用户不存在与密码错误必须返回同一句话，
// 否则攻击者可以据此枚举出系统里有哪些账号。
const msgPasswordNotMatch = "用户不存在/密码错误"

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

	enabled, err := CaptchaEnabled(ctx)
	if err != nil {
		return "", err
	}
	if enabled {
		if err := VerifyCaptcha(ctx, body.UUID, body.Code); err != nil {
			return fail(err)
		}
	}

	retryKey := redisx.PwdErrCntKey(body.Username)
	count, err := currentRetryCount(ctx, retryKey)
	if err != nil {
		return "", err
	}
	if count >= maxPasswordRetry {
		return fail(errs.Newf("密码输入错误%d次，帐户锁定%d分钟",
			maxPasswordRetry, int(passwordLockTime.Minutes())))
	}

	user, err := repository.SelectUserByUserName(ctx, body.Username)
	if err != nil {
		return "", err
	}
	if user == nil || user.DelFlag == model.DelFlagDeleted {
		return fail(recordPasswordFailure(ctx, retryKey, count))
	}
	if user.Status == model.StatusDisable {
		return fail(errs.New("对不起，您的帐号已停用"))
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)) != nil {
		return fail(recordPasswordFailure(ctx, retryKey, count))
	}

	// 登录成功，清空错误计数
	if err := redisx.C().Del(ctx, retryKey).Err(); err != nil {
		slog.Warn("清除密码错误计数失败", "user", body.Username, "err", err)
	}

	permissions, err := GetMenuPermission(ctx, user)
	if err != nil {
		return "", err
	}

	loginUser := &model.LoginUser{
		UserID:      user.UserID,
		DeptID:      user.DeptID,
		IPAddr:      ip,
		Browser:     browser,
		OS:          os,
		Permissions: permissions,
		User:        user,
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

// recordPasswordFailure 累加错误次数并返回统一错误。
func recordPasswordFailure(ctx context.Context, key string, current int) error {
	next := current + 1
	if err := redisx.C().Set(ctx, key, next, passwordLockTime).Err(); err != nil {
		slog.Warn("记录密码错误次数失败", "err", err)
	}
	if next >= maxPasswordRetry {
		return errs.Newf("密码输入错误%d次，帐户锁定%d分钟",
			maxPasswordRetry, int(passwordLockTime.Minutes()))
	}
	return errs.New(msgPasswordNotMatch)
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
