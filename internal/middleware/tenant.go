package middleware

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/pkg/response"
	"strings"
)

func PlatformOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil || u.User == nil || !u.User.IsPlatform() {
			response.FailCode(c, response.CodeForbidden, "仅平台管理员可操作")
			c.Abort()
			return
		}
		c.Next()
	}
}

func TenantOwnerOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil || u.User == nil || u.User.UserType != "01" || u.User.IsPlatform() {
			response.FailCode(c, response.CodeForbidden, "仅企业所有者可操作")
			c.Abort()
			return
		}
		c.Next()
	}
}
func PlatformBoundary() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		restricted := strings.HasPrefix(p, "/tool/gen") || strings.HasPrefix(p, "/platform/") ||
			strings.HasPrefix(p, "/monitor/cache") || strings.HasPrefix(p, "/monitor/server") || strings.HasPrefix(p, "/monitor/online")
		if strings.HasPrefix(p, "/system/menu") && !(c.Request.Method == "GET" && (p == "/system/menu/treeselect" || strings.HasPrefix(p, "/system/menu/roleMenuTreeselect/"))) {
			restricted = true
		}
		if restricted {
			PlatformOnly()(c)
			return
		}
		c.Next()
	}
}
