package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSCompare(c *gin.Context) {
	var in model.CMSCompareInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "版本比较参数错误")
		return
	}
	out, e := service.CMSCompare(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func CMSInquiryFilteredTasks(c *gin.Context) {
	var q model.CMSInquiryFilter
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "筛选参数错误")
		return
	}
	rows, n, e := service.CMSInquiryFilteredTasks(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSInquiryExport(c *gin.Context) {
	var in model.CMSInquiryExport
	if e := c.ShouldBindJSON(&in); e != nil {
		response.FailDownload(c, response.CodeError, "导出筛选参数错误")
		return
	}
	b, e := service.CMSExportInquiries(c.Request.Context(), in)
	if e != nil {
		failDownload(c, e)
		return
	}
	response.FileBytes(c, "cms-inquiries.xlsx", excelx.ContentType, b)
}
func CMSDataPolicyGet(c *gin.Context) {
	row, e := service.CMSGetDataPolicy(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSDataPolicySet(c *gin.Context) {
	var in model.CMSDataPolicyInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "保留策略参数错误")
		return
	}
	if e := service.CMSSetDataPolicy(c.Request.Context(), in); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSCleanupPreview(c *gin.Context) {
	var in model.CMSCleanupInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "请填写本次清理原因，最多300字")
		return
	}
	row, e := service.CMSPreviewCleanup(c.Request.Context(), in)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSCleanupExecute(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.CMSCleanupExecute
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, "请填写有效版本并输入确认清除")
		return
	}
	if e = service.CMSExecuteCleanup(c.Request.Context(), id, in.Version); e != nil {
		fail(c, e)
		return
	}
	response.Ok(c)
}
func CMSCleanupBatches(c *gin.Context) {
	rows, n, e := service.CMSCleanupBatches(c.Request.Context(), page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSCleanupItems(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.CMSCleanupItems(c.Request.Context(), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func CMSDataEvents(own bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, n, e := service.CMSDataEvents(c.Request.Context(), own, page.Parse(c, nil))
		if e != nil {
			fail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func CMSSearchRebuild(c *gin.Context) {
	n, e := service.CMSRebuildSearch(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, n)
}
func CMSSearchStatus(c *gin.Context) {
	row, e := service.CMSSearchStatus(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, row)
}
func CMSSearchTasks(c *gin.Context) {
	var q model.CMSSearchQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, "索引状态参数错误")
		return
	}
	rows, n, e := service.CMSSearchTasks(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
