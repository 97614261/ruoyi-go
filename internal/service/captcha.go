package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/redisx"
)

// captchaTTL 验证码有效期，与 Java 版 Constants.CAPTCHA_EXPIRATION 一致。
const captchaTTL = 2 * time.Minute

// consumeCaptchaScript 原子读取并删除验证码，保证同一个验证码只能被一个请求消费。
var consumeCaptchaScript = redis.NewScript(`
local answer = redis.call('get', KEYS[1])
if answer then
    redis.call('del', KEYS[1])
end
return answer
`)

// ConfigKeyCaptchaEnabled sys_config 中控制验证码开关的键。
const ConfigKeyCaptchaEnabled = "sys.account.captchaEnabled"

// captchaType 由 InitCaptcha 设置。
var captchaType = CaptchaTypeMath

// InitCaptcha 设置验证码类型。
func InitCaptcha(t string) {
	if t == CaptchaTypeChar {
		captchaType = CaptchaTypeChar
		return
	}
	captchaType = CaptchaTypeMath
}

// CaptchaEnabled 读 sys_config 判断是否启用验证码，参数缺失时默认启用。
//
// 走带 Redis 缓存的 GetConfigValueByKey —— 登录页每次刷新都会调它，
// 直接查库是无谓的开销。
func CaptchaEnabled(ctx context.Context) (bool, error) {
	value, err := GetConfigValueByKey(ctx, ConfigKeyCaptchaEnabled)
	if err != nil {
		return false, err
	}
	if value == "" {
		return true, nil
	}
	return strings.EqualFold(value, "true"), nil
}

// GenerateCaptcha 生成验证码并写入 Redis，返回 uuid 与 base64 图片。
func GenerateCaptcha(ctx context.Context) (string, string, error) {
	answer, img, err := drawCaptcha(captchaType)
	if err != nil {
		return "", "", err
	}

	id := uuid.NewString()
	key := redisx.CaptchaKey(id)
	if err := redisx.C().Set(ctx, key, answer, captchaTTL).Err(); err != nil {
		return "", "", fmt.Errorf("写入验证码失败: %w", err)
	}
	return id, img, nil
}

// VerifyCaptcha 校验验证码，无论成败都会删除 Redis 中的记录（一次性使用）。
//
// 错误信息与 Java 版保持一致，前端会直接展示。
func VerifyCaptcha(ctx context.Context, id, code string) error {
	if id == "" || code == "" {
		return errs.New("验证码错误")
	}
	key := redisx.CaptchaKey(id)

	answer, err := consumeCaptchaScript.Run(ctx, redisx.C(), []string{key}).Text()

	if errors.Is(err, redis.Nil) {
		return errs.New("验证码已失效")
	}
	if err != nil {
		return fmt.Errorf("读取验证码失败: %w", err)
	}
	if !strings.EqualFold(answer, strings.TrimSpace(code)) {
		return errs.New("验证码错误")
	}
	return nil
}
