package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// ConfigList GET /system/config/list
func ConfigList(c *gin.Context) {
	var query model.ConfigQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.ConfigSortColumns)

	list, total, err := service.ListConfigPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// ConfigExport POST /system/config/export
func ConfigExport(c *gin.Context) {
	var query model.ConfigQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListConfigExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "参数数据.xlsx", "参数数据", list); err != nil {
		failDownload(c, err)
		return
	}
}

// ConfigGetByKey GET /system/config/configKey/:configKey
//
// 不挂权限：前端在用户管理等页面要读 sys.user.initPassword 展示初始密码，
// 与 Java 一致不做鉴权（登录后即可访问）。
//
// 注意参数键名带点号（sys.user.initPassword），gin 的路径参数能正常匹配。
func ConfigGetByKey(c *gin.Context) {
	value, err := service.GetConfigValueByKey(c.Request.Context(), c.Param("configKey"))
	if err != nil {
		fail(c, err)
		return
	}
	// 返回的是纯字符串，放在 msg 里 —— 与 Java 的 success(configValue) 一致
	response.OkMsg(c, value)
}

// ConfigGet GET /system/config/:configId
func ConfigGet(c *gin.Context) {
	id, err := parseID(c.Param("configId"))
	if err != nil {
		fail(c, err)
		return
	}
	config, err := service.GetConfig(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, config)
}

// ConfigAdd POST /system/config
func ConfigAdd(c *gin.Context) {
	var config model.SysConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateConfig(c.Request.Context(), &config, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ConfigEdit PUT /system/config
func ConfigEdit(c *gin.Context) {
	var config model.SysConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateConfig(c.Request.Context(), &config, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ConfigRemove DELETE /system/config/:configIds
func ConfigRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("configIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteConfigs(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ConfigRefreshCache DELETE /system/config/refreshCache
func ConfigRefreshCache(c *gin.Context) {
	if err := service.ClearConfigCache(c.Request.Context(), ""); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
