package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func TenantMenus(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	settings, err := service.TenantMenus(c.Request.Context(), middleware.CurrentUser(c).User, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, settings)
}

func TenantMenusSave(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	var body service.TenantMenusInput
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "授权信息不完整或格式错误")
		return
	}
	if err := service.SaveTenantMenus(c.Request.Context(), middleware.CurrentUser(c).User, id, body, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, "授权已更新；该企业旧会话已失效，请重新登录")
}

func TenantOwnerPasswordReset(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	var body service.TenantOwnerPasswordInput
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "重置信息不完整或格式错误")
		return
	}
	cleared, err := service.ResetTenantOwnerPassword(c.Request.Context(), middleware.CurrentUser(c).User, id, body, c.ClientIP())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, gin.H{"retryCounterCleared": cleared})
}
