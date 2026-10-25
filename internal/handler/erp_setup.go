package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func ERPSetup(c *gin.Context) {
	out, e := service.ERPSetup(c.Request.Context(), middleware.CurrentUser(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPRoleTemplates(c *gin.Context) {
	out, e := service.ERPRoleTemplates(c.Request.Context(), middleware.CurrentUser(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
