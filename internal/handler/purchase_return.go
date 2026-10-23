package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPPurchaseReturnableList(c *gin.Context) {
	var q model.ERPPurchaseReceiptQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPPurchaseReturnables(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPPurchaseReturnableGet(c *gin.Context) {
	id, e := parseID(c.Param("receiptId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPPurchaseReturnable(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseReturnPost(c *gin.Context) {
	id, e := parseID(c.Param("receiptId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPPurchaseReturnInput
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	v, e := service.ReturnERPPurchaseReceipt(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseReturnList(c *gin.Context) {
	var q model.ERPPurchaseReturnQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPPurchaseReturns(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPPurchaseReturnGet(c *gin.Context) {
	id, e := parseID(c.Param("returnId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPPurchaseReturn(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
