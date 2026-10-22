package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"strconv"
)

func ERPOrderList(c *gin.Context) {
	var q model.ERPOrderQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, "订单查询参数格式不正确")
		return
	}
	rows, total, err := service.ListERPOrders(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ERPOrderGet(c *gin.Context) {
	id, err := parseID(c.Param("orderId"))
	if err != nil {
		fail(c, err)
		return
	}
	out, err := service.GetERPOrder(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPOrderAdd(c *gin.Context) {
	var in model.ERPOrderWrite
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "订单JSON字段类型或时间格式不正确")
		return
	}
	out, err := service.CreateERPOrder(c.Request.Context(), in, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPOrderEdit(c *gin.Context) {
	id, err := parseID(c.Param("orderId"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.ERPOrderWrite
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "订单JSON字段类型或时间格式不正确")
		return
	}
	out, err := service.EditERPOrder(c.Request.Context(), id, in, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPOrderExport(c *gin.Context) {
	var q model.ERPOrderQuery
	if c.ShouldBind(&q) != nil {
		response.FailDownload(c, 500, "导出参数不正确")
		return
	}
	rows, err := service.ExportERPOrders(c.Request.Context(), q)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err = excelx.WriteResponse(c, "订单摘要.xlsx", "订单摘要", rows); err != nil {
		failDownload(c, err)
	}
}
func ERPOrderTemplate(c *gin.Context) {
	data, err := service.ERPOrderImportTemplate()
	if err != nil {
		failDownload(c, err)
		return
	}
	setAttachmentHeader(c, "订单导入模板.xlsx")
	c.Header("Cache-Control", "no-store")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
func ERPOrderImport(c *gin.Context) {
	f, err := c.FormFile("file")
	if form := c.Request.MultipartForm; form != nil {
		defer form.RemoveAll()
	}
	if err != nil {
		response.Fail(c, "请选择固定XLSX模板文件")
		return
	}
	shop, err := strconv.ParseInt(c.PostForm("shopId"), 10, 64)
	if err != nil {
		response.Fail(c, "店铺参数不正确")
		return
	}
	r, err := f.Open()
	if err != nil {
		response.Fail(c, "上传文件读取失败")
		return
	}
	defer r.Close()
	out, err := service.SubmitERPOrderImport(c.Request.Context(), shop, c.PostForm("requestKey"), r, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPImportJobList(c *gin.Context) {
	var q model.ERPImportQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, "任务查询参数不正确")
		return
	}
	rows, total, err := service.ListERPImportJobs(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ERPImportJobGet(c *gin.Context) {
	id, err := parseID(c.Param("jobId"))
	if err != nil {
		fail(c, err)
		return
	}
	out, err := service.GetERPImportJob(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPImportJobResults(c *gin.Context) {
	id, err := parseID(c.Param("jobId"))
	if err != nil {
		fail(c, err)
		return
	}
	rows, total, err := service.GetERPImportResults(c.Request.Context(), id, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ERPImportJobRetry(c *gin.Context) {
	id, err := parseID(c.Param("jobId"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.ERPImportRetry
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "重试参数不正确")
		return
	}
	out, err := service.RetryERPOrderImport(c.Request.Context(), id, in)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPImportJobFile(c *gin.Context) {
	id, err := parseID(c.Param("jobId"))
	if err != nil {
		failDownload(c, err)
		return
	}
	data, err := service.ERPImportFile(c.Request.Context(), id)
	if err != nil {
		failDownload(c, err)
		return
	}
	setAttachmentHeader(c, "订单导入原文件.xlsx")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
