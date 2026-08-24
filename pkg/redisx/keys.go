package redisx

// Redis key 前缀，必须与 Java 版 CacheConstants.java 完全一致，
// 否则两套后端不能共用同一个 Redis（迁移期需要并行验证）。
//
// 禁止在业务代码里裸写这些字符串。
const (
	// KeyLoginToken 登录会话，后接 uuid。value 是序列化的 LoginUser。
	KeyLoginToken = "login_tokens:"
	// KeyCaptchaCode 验证码，后接 uuid，TTL 2 分钟。
	KeyCaptchaCode = "captcha_codes:"
	// KeySysConfig 参数缓存，后接 configKey。
	KeySysConfig = "sys_config:"
	// KeySysDict 字典缓存，后接 dictType。
	KeySysDict = "sys_dict:"
	// KeyRepeatSubmit 防重复提交。
	KeyRepeatSubmit = "repeat_submit:"
	// KeyRateLimit 限流。
	KeyRateLimit = "rate_limit:"
	// KeyPwdErrCnt 密码错误次数，后接 username，5 次锁 10 分钟。
	KeyPwdErrCnt = "pwd_err_cnt:"
)

// LoginTokenKey 拼接登录会话 key。
func LoginTokenKey(uuid string) string { return KeyLoginToken + uuid }

// CaptchaKey 拼接验证码 key。
func CaptchaKey(uuid string) string { return KeyCaptchaCode + uuid }

// SysConfigKey 拼接参数缓存 key。
func SysConfigKey(configKey string) string { return KeySysConfig + configKey }

// SysDictKey 拼接字典缓存 key。
func SysDictKey(dictType string) string { return KeySysDict + dictType }

// PwdErrCntKey 拼接密码错误次数 key。
func PwdErrCntKey(username string) string { return KeyPwdErrCnt + username }

// RateLimitKey 拼接限流 key。
func RateLimitKey(suffix string) string { return KeyRateLimit + suffix }

// RepeatSubmitKey 拼接防重复提交 key。
func RepeatSubmitKey(suffix string) string { return KeyRepeatSubmit + suffix }
