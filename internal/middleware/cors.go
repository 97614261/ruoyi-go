package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins 允许跨域的来源白名单，由 InitCORS 注入。
var allowedOrigins []string

var allowAnyOrigin bool

// InitCORS 设置跨域白名单，必须在构建路由前调用。
func InitCORS(origins []string) {
	allowedOrigins = allowedOrigins[:0]
	allowAnyOrigin = false
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			allowAnyOrigin = true
			continue
		}
		if origin != "" {
			allowedOrigins = append(allowedOrigins, origin)
		}
	}
}

// CORS 跨域处理。
//
// 【默认不放行任何来源】必须在配置里显式列出白名单。
//
// 早先的实现是回显请求方 Origin 并带 Allow-Credentials: true，
// 等价于允许任意站点携带凭证跨域访问 —— 这是明确的反模式。
//
// 而且实际上大多数部署根本用不到它：开发时前端走 Vite 代理是同源，
// 生产走 Nginx 反代也是同源。真需要跨域时再往配置里加，
// 不要为了"省事"默认全开。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && isOriginAllowed(origin) {
			if allowAnyOrigin {
				// The CORS standard forbids credentials with a wildcard origin.
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				// An echoed Origin changes the response and therefore the cache key.
				c.Header("Vary", "Origin")
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Authorization, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isOriginAllowed(origin string) bool {
	if allowAnyOrigin {
		return true
	}
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}
