package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/response"
	"strings"
)

func PODCreatorAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		path := c.FullPath()
		if path == "/pod/eligibility" || path == "/pod/points" || path == "/pod/workspace" {
			c.Next()
			return
		}
		write := c.Request.Method != http.MethodGet
		if strings.HasSuffix(path, "/cancel") || strings.HasSuffix(path, "/stop") {
			write = false
		}
		if err := service.PODCheckCreator(c.Request.Context(), CurrentUser(c).UserID, write); err != nil {
			code, msg := 500, "资格校验暂不可用"
			if e := errs.As(err); e != nil {
				code, msg = e.Code, e.Msg
			}
			response.FailCode(c, code, msg)
			c.Abort()
			return
		}
		c.Next()
	}
}
