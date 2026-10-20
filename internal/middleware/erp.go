package middleware

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/tenantctx"
)

func CustomerWorkspaceOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := tenantctx.Get(c.Request.Context())
		if s.ID < 1 || s.Platform {
			response.New(response.CodeForbidden, "请使用企业账号进入ERP，平台账号不代入客户业务").JSON(c)
			c.Abort()
			return
		}
		user := CurrentUser(c)
		if user == nil || user.UserID < 1 {
			unauthorized(c)
			return
		}
		c.Request = c.Request.WithContext(erpctx.WithUser(c.Request.Context(), user.UserID))
		c.Next()
	}
}
