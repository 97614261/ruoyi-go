package middleware

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/response"
)

func PODAdminAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if err := service.CheckPODAdminEntry(c.Request.Context(), CurrentUser(c), c.FullPath() == "/pod-admin/bootstrap"); err != nil {
			code, msg := 500, "管理入口暂不可用，请稍后重试"
			if business := errs.As(err); business != nil {
				code, msg = business.Code, business.Msg
			}
			response.FailCode(c, code, msg)
			c.Abort()
			return
		}
		c.Next()
	}
}
