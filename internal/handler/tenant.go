package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func TenantList(c *gin.Context) {
	rows, total, err := service.ListTenants(c.Request.Context(), c.Query("tenantName"), page.Parse(c, map[string]string{}))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func TenantSave(c *gin.Context) {
	var body service.TenantInput
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "租户信息不完整或格式错误")
		return
	}
	if (c.Request.Method == "POST" && body.TenantID != 0) || (c.Request.Method == "PUT" && body.TenantID <= 0) {
		response.Fail(c, "新增不能指定tenantId，修改必须指定tenantId")
		return
	}
	if err := service.SaveTenant(c.Request.Context(), body, middleware.CurrentUser(c).User, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
