// Package middleware Gin 中间件。[L4]
package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/response"
)

// Recovery 捕获 panic，记录堆栈并返回统一错误。
//
// 【硬约束】禁止用 panic 控制业务流程。这个中间件是最后一道防线，
// 走到这里说明有 bug，不是正常路径。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// 用 ErrorContext 带出 traceId —— panic 这条恰恰是最需要能追溯的
				slog.ErrorContext(c.Request.Context(), "请求处理发生 panic",
					"err", fmt.Sprint(r),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"clientIP", c.ClientIP(),
					"stack", string(debug.Stack()),
				)
				// 堆栈只进日志，响应里只给通用提示
				response.FailCode(c, response.CodeError, "系统异常，请联系管理员")
				c.Abort()
			}
		}()
		c.Next()
	}
}
