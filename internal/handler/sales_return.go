package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPSalesReturnableList(c *gin.Context) {
	var q model.ERPSalesReturnQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPSalesReturnables(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPSalesReturnableGet(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPSalesReturnable(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPSalesReturnPost(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPSalesReturnInput
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	v, e := service.ReturnERPSalesPackage(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPSalesReturnList(c *gin.Context) {
	var q model.ERPSalesReturnQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPSalesReturns(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPSalesReturnGet(c *gin.Context) {
	id, e := parseID(c.Param("returnId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPSalesReturn(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
