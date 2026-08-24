package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// allowedOrigins 允许跨域的来源白名单，由 InitCORS 注入。
var allowedOrigins []string

// InitCORS 设置跨域白名单，必须在构建路由前调用。
func InitCORS(origins []string) {
	allowedOrigins = origins
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
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Authorization, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400")
			// 回显的 Origin 会进缓存，必须声明按 Origin 变化，
			// 否则 CDN／代理可能把 A 站的响应头发给 B 站
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isOriginAllowed(origin string) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return true
		}
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}
