package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// ---------- 登录日志 ----------

// LogininforList GET /monitor/logininfor/list
func LogininforList(c *gin.Context) {
	var query model.LogininforQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.LogininforSortColumns)

	list, total, err := service.ListLogininforPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// LogininforExport POST /monitor/logininfor/export
func LogininforExport(c *gin.Context) {
	var query model.LogininforQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListLogininforExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "登录日志.xlsx", "登录日志", list); err != nil {
		failDownload(c, err)
		return
	}
}

// LogininforRemove DELETE /monitor/logininfor/:infoIds
func LogininforRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("infoIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteLogininfor(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// LogininforClean DELETE /monitor/logininfor/clean
func LogininforClean(c *gin.Context) {
	if err := service.CleanLogininfor(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// LogininforUnlock GET /monitor/logininfor/unlock/:userName
func LogininforUnlock(c *gin.Context) {
	if err := service.UnlockAccount(c.Request.Context(), c.Param("userName")); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ---------- 操作日志 ----------

// OperLogList GET /monitor/operlog/list
func OperLogList(c *gin.Context) {
	var query model.OperLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.OperLogSortColumns)

	list, total, err := service.ListOperLogPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// OperLogExport POST /monitor/operlog/export
func OperLogExport(c *gin.Context) {
	var query model.OperLogQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListOperLogExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "操作日志.xlsx", "操作日志", list); err != nil {
		failDownload(c, err)
		return
	}
}

// OperLogRemove DELETE /monitor/operlog/:operIds
func OperLogRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("operIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteOperLog(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// OperLogClean DELETE /monitor/operlog/clean
func OperLogClean(c *gin.Context) {
	if err := service.CleanOperLog(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
