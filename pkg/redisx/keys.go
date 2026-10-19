package redisx

import (
	"context"
	"ruoyi-go/pkg/tenantctx"
	"strconv"
)

// 多租户分支使用独立 ruoyi_mt: 命名空间，不与 Java / main 共用会话和缓存。
// 用户索引按全局唯一 userId 定位；配置、字典、验证码等额外包含 tenantId。
//
// 禁止在业务代码里裸写这些字符串。
const (
	// KeyLoginToken 登录会话，后接 uuid。value 是序列化的 LoginUser。
	KeyLoginToken = "ruoyi_mt:login_tokens:"
	// KeyLoginUserSessions 按用户维护其全部登录 uuid，供停用、删除和改密时立即撤销。
	KeyLoginUserSessions = "ruoyi_mt:login_user_sessions:"
	// KeyLoginUserGeneration 防止撤销会话时，并发登录把旧认证结果重新写回。
	KeyLoginUserGeneration = "ruoyi_mt:login_user_generation:"
	// KeyLoginUserPermissionVersion 标记用户权限版本，跨进程重启识别旧权限快照。
	KeyLoginUserPermissionVersion = "ruoyi_mt:login_user_permission_version:"
	// KeyCaptchaCode 验证码，后接 uuid，TTL 2 分钟。
	KeyCaptchaCode = "ruoyi_mt:captcha_codes:"
	// KeySysConfig 参数缓存，后接 configKey。
	KeySysConfig = "ruoyi_mt:sys_config:"
	// KeySysDict 字典缓存，后接 dictType。
	KeySysDict = "ruoyi_mt:sys_dict:"
	// KeyRepeatSubmit 防重复提交。
	KeyRepeatSubmit = "ruoyi_mt:repeat_submit:"
	// KeyRateLimit 限流。
	KeyRateLimit = "ruoyi_mt:rate_limit:"
	// KeyPwdErrCnt 密码错误次数，后接 username，5 次锁 10 分钟。
	KeyPwdErrCnt = "ruoyi_mt:pwd_err_cnt:"
)

// LoginTokenKey 拼接登录会话 key。
func LoginTokenKey(uuid string) string { return KeyLoginToken + uuid }

// LoginUserSessionsKey 拼接用户会话索引 key。
func LoginUserSessionsKey(userID int64) string {
	return KeyLoginUserSessions + strconv.FormatInt(userID, 10)
}

// LoginUserGenerationKey 拼接用户会话代数 key。
func LoginUserGenerationKey(userID int64) string {
	return KeyLoginUserGeneration + strconv.FormatInt(userID, 10)
}

// LoginUserPermissionVersionKey 拼接用户权限版本 key。
func LoginUserPermissionVersionKey(userID int64) string {
	return KeyLoginUserPermissionVersion + strconv.FormatInt(userID, 10)
}

// CaptchaKey 拼接验证码 key。
func CaptchaKey(uuid string) string { return KeyCaptchaCode + uuid }

// SysConfigKey 拼接参数缓存 key。
func SysConfigKey(configKey string) string { return KeySysConfig + configKey }

// SysDictKey 拼接字典缓存 key。
func SysDictKey(dictType string) string { return KeySysDict + dictType }

// PwdErrCntKey 拼接密码错误次数 key。
func PwdErrCntKey(username string) string { return KeyPwdErrCnt + username }

func TenantKey(ctx context.Context, prefix, suffix string) string {
	return prefix + strconv.FormatInt(tenantctx.ID(ctx), 10) + ":" + suffix
}
func TenantConfigKey(ctx context.Context, key string) string {
	return TenantKey(ctx, KeySysConfig, key)
}
func TenantDictKey(ctx context.Context, key string) string { return TenantKey(ctx, KeySysDict, key) }
func TenantPasswordKey(ctx context.Context, name string) string {
	return TenantKey(ctx, KeyPwdErrCnt, name)
}

// RateLimitKey 拼接限流 key。
func RateLimitKey(suffix string) string { return KeyRateLimit + suffix }

// RepeatSubmitKey 拼接防重复提交 key。
func RepeatSubmitKey(suffix string) string { return KeyRepeatSubmit + suffix }
