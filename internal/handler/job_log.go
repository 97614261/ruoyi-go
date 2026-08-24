package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// JobLogList GET /monitor/jobLog/list
func JobLogList(c *gin.Context) {
	var query model.JobLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.JobLogSortColumns)

	list, total, err := service.ListJobLogPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// JobLogExport POST /monitor/jobLog/export
func JobLogExport(c *gin.Context) {
	var query model.JobLogQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListJobLogExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "调度日志.xlsx", "调度日志", list); err != nil {
		failDownload(c, err)
		return
	}
}

// JobLogGet GET /monitor/jobLog/:jobLogId
func JobLogGet(c *gin.Context) {
	id, err := parseID(c.Param("jobLogId"))
	if err != nil {
		fail(c, err)
		return
	}
	record, err := service.GetJobLog(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, record)
}

// JobLogRemove DELETE /monitor/jobLog/:jobLogIds
func JobLogRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("jobLogIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteJobLogs(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// JobLogClean DELETE /monitor/jobLog/clean
func JobLogClean(c *gin.Context) {
	if err := service.CleanJobLog(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
