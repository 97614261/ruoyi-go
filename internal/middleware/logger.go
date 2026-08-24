package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// slowRequestThreshold 超过这个耗时的请求单独告警，由 InitLogger 设置。
//
// 【为什么值得单独打一条】
// 正常请求也在打日志，慢的那几条淹在里面根本看不见。
// 单独提到 warn 级别，配上日志平台的等级过滤就是一份免费的性能报告 ——
// 这是上 Prometheus 之前性价比最高的可观测性手段。
var slowRequestThreshold = 500 * time.Millisecond

// InitLogger 设置慢请求阈值，<=0 时保持默认值。
func InitLogger(threshold time.Duration) {
	if threshold > 0 {
		slowRequestThreshold = threshold
	}
}

// Logger 记录请求日志。
//
// 只记录元信息，不记录请求体和响应体 —— 登录接口的密码、
// 各类 token 都在请求体里，落日志等于泄露。
//
// 【query 也必须脱敏】"不记请求体"挡不住 /system/user/profile/updatePwd ——
// 它的 oldPassword / newPassword 走的是查询串，原样打出来就是明文密码进日志。
// 操作日志那边早就做了脱敏，这里漏掉过一次。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		// 必须在 c.Next() 之后取 ctx：Trace 中间件是通过替换 c.Request 注入的，
		// 提前取会拿到没有 traceId 的旧 ctx
		ctx := c.Request.Context()
		cost := time.Since(start)

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"costMs", cost.Milliseconds(),
			"clientIP", c.ClientIP(),
		}
		if query != "" {
			attrs = append(attrs, "query", desensitize(query))
		}
		if user := CurrentUser(c); user != nil {
			attrs = append(attrs, "userId", user.UserID)
			if user.User != nil {
				attrs = append(attrs, "userName", user.User.UserName)
			}
		}

		switch {
		case len(c.Errors) > 0:
			attrs = append(attrs, "errors", c.Errors.String())
			slog.ErrorContext(ctx, "请求异常", attrs...)
		case cost >= slowRequestThreshold:
			attrs = append(attrs, "thresholdMs", slowRequestThreshold.Milliseconds())
			slog.WarnContext(ctx, "慢请求", attrs...)
		default:
			slog.InfoContext(ctx, "请求完成", attrs...)
		}
	}
}
