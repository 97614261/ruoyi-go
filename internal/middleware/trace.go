package middleware

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/logx"
)

// maxIncomingTraceID 外部传入的 traceId 长度上限。
//
// 反代（Nginx / 网关）常会带一个 X-Request-Id 进来，沿用它可以把
// 网关日志和应用日志串起来。但**不能无条件信任外部输入**：
// 超长或带控制字符的值会污染日志、撑爆存储，甚至伪造出别的请求的 ID。
const maxIncomingTraceID = 64

// Trace 给每个请求分配 traceId。
//
// 必须**排在所有中间件最前面** —— 排在 Recovery 后面的话，
// panic 那条日志就没有 traceId，而那恰恰是最需要能追溯的一条。
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := sanitizeTraceID(c.GetHeader("X-Request-Id"))
		if id == "" {
			id = logx.NewTraceID()
		}

		// 塞进 request 的 ctx：业务层用 slog.XxxContext(ctx, ...) 就能自动带出，
		// 不需要一层层传参数
		c.Request = c.Request.WithContext(logx.WithTraceID(c.Request.Context(), id))
		c.Header(logx.HeaderTraceID, id)

		c.Next()
	}
}

// sanitizeTraceID 过滤外部传入的 traceId，只保留字母数字和连字符。
func sanitizeTraceID(raw string) string {
	if len(raw) == 0 || len(raw) > maxIncomingTraceID {
		return ""
	}
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return ""
		}
	}
	return raw
}
