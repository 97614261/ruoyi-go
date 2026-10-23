package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPPurchaseReceivables(c *gin.Context) {
	var q model.ERPPurchaseQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPPurchaseReceivables(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPPurchaseReceivable(c *gin.Context) {
	id, e := parseID(c.Param("purchaseId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPPurchaseReceivable(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseReceive(c *gin.Context) {
	id, e := parseID(c.Param("purchaseId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPPurchaseReceiveInput
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	v, e := service.ReceiveERPPurchase(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseReceiptList(c *gin.Context) {
	var q model.ERPPurchaseReceiptQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPPurchaseReceipts(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPPurchaseReceiptGet(c *gin.Context) {
	id, e := parseID(c.Param("receiptId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPPurchaseReceipt(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
