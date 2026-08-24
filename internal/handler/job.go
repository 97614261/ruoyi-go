package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// JobList GET /monitor/job/list
func JobList(c *gin.Context) {
	var query model.JobQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.JobSortColumns)

	list, total, err := service.ListJobPage(c.Request.Context(), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// JobExport POST /monitor/job/export
func JobExport(c *gin.Context) {
	var query model.JobQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListJobExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "定时任务.xlsx", "定时任务", list); err != nil {
		failDownload(c, err)
		return
	}
}

// JobGet GET /monitor/job/:jobId
func JobGet(c *gin.Context) {
	id, err := parseID(c.Param("jobId"))
	if err != nil {
		fail(c, err)
		return
	}
	target, err := service.GetJob(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, target)
}

// JobAdd POST /monitor/job
func JobAdd(c *gin.Context) {
	var target model.SysJob
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateJob(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// JobEdit PUT /monitor/job
func JobEdit(c *gin.Context) {
	var target model.SysJob
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateJob(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// JobChangeStatus PUT /monitor/job/changeStatus
func JobChangeStatus(c *gin.Context) {
	var body model.JobStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.ChangeJobStatus(c.Request.Context(), body.JobID, body.Status, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// JobRun PUT /monitor/job/run
//
// 只负责把任务丢出去执行，不等结果 —— 任务可能跑几分钟，
// 同步等会把请求挂死。执行结果去调度日志里看。
func JobRun(c *gin.Context) {
	var body model.JobStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.RunJobOnce(c.Request.Context(), body.JobID); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// JobRemove DELETE /monitor/job/:jobIds
func JobRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("jobIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteJobs(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
