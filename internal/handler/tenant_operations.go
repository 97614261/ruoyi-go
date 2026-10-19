package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func TenantDetail(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	value, err := service.TenantDetail(c.Request.Context(), middleware.CurrentUser(c).User, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, value)
}
func TenantQuota(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	value, err := service.TenantQuota(c.Request.Context(), middleware.CurrentUser(c).User, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, value)
}
func TenantQuotaSave(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	var body service.TenantQuotaInput
	if err = c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "配额信息不完整或格式错误")
		return
	}
	if err = service.SaveTenantQuota(c.Request.Context(), middleware.CurrentUser(c).User, id, body, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, "配额已更新，该企业旧会话已失效")
}
func TenantLifecycle(c *gin.Context) {
	id, err := parseID(c.Param("tenantId"))
	if err != nil {
		fail(c, err)
		return
	}
	var body service.TenantLifecycleInput
	if err = c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "生命周期信息不完整或格式错误")
		return
	}
	if err = service.ChangeTenantLifecycle(c.Request.Context(), middleware.CurrentUser(c).User, id, body, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, "状态已变更，数据保留，旧会话已失效；恢复后须另行启用")
}
func OwnWorkspace(c *gin.Context) {
	value, err := service.OwnWorkspace(c.Request.Context(), middleware.CurrentUser(c).User)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, value)
}
func TenantCancellation(c *gin.Context) {
	var body service.TenantLifecycleInput
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "注销申请信息不完整或格式错误")
		return
	}
	if body.Action != "requestCancellation" {
		response.Fail(c, "企业端仅允许申请注销")
		return
	}
	u := middleware.CurrentUser(c).User
	if err := service.ChangeTenantLifecycle(c.Request.Context(), u, u.TenantID, body, c.ClientIP()); err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, "注销申请已提交，企业已暂停使用，数据保留等待平台处理")
}
