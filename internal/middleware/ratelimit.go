package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/response"
)

// rateLimitTimeout 限流查询本身的超时。
//
// 必须很短：限流是每个请求都要过的关卡，它慢了整个接口就跟着慢。
const rateLimitTimeout = time.Second

// RateLimitOptions 限流参数。
type RateLimitOptions struct {
	// Key 业务标识，用来区分不同接口的计数，如 "login"
	Key string
	// Count 窗口内允许的次数
	Count int
	// Window 窗口长度
	Window time.Duration
	// ByIP 为真则按来源 IP 分别计数；否则整个接口共用一个计数器
	ByIP bool
}

// RateLimit 限流中间件。
//
// 【为什么要有】`/login` 走 bcrypt，实测并发 50 就把 CPU 打满、QPS 只有 200。
// 这是全站最容易被打垮的点，也是暴力破解的入口。
// 已有的 pwd_err_cnt（5 次锁 10 分钟）只挡单个账号，挡不住拿一批账号轮着刷。
//
// Java 版定义了 @RateLimiter 注解和切面，但**全仓库零处使用**，等于摆设。
// 所以这不是"对齐 Java"，是补一个两边都缺的真实防护。
//
// 【Redis 挂了怎么办：放行】
// 限流是防护不是业务。Redis 不可用时如果一律拒绝，等于让缓存故障
// 直接升级成全站不可用 —— 那比被刷严重得多。记 error 日志后放行。
func RateLimit(opts RateLimitOptions) gin.HandlerFunc {
	if opts.Count <= 0 || opts.Window <= 0 {
		panic("middleware.RateLimit: Count 和 Window 必须为正数")
	}

	return func(c *gin.Context) {
		key := redisx.RateLimitKey(opts.Key)
		if opts.ByIP {
			key += ":" + c.ClientIP()
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), rateLimitTimeout)
		defer cancel()

		current, err := redisx.RateLimit(ctx, key, opts.Count, opts.Window)
		if err != nil {
			slog.ErrorContext(ctx, "限流查询失败，本次放行", "key", key, "err", err)
			c.Next()
			return
		}

		if current > int64(opts.Count) {
			slog.WarnContext(ctx, "请求被限流",
				"key", key, "current", current, "limit", opts.Count, "clientIP", c.ClientIP())
			// 文案与 Java 版 RateLimiterAspect 一致
			response.Fail(c, "访问过于频繁，请稍候再试")
			c.Abort()
			return
		}
		c.Next()
	}
}
