package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPPurchaseList(c *gin.Context) {
	var q model.ERPPurchaseQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPPurchases(c.Request.Context(), q, page.Parse(c, model.ERPPurchaseSortColumns))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPPurchaseGet(c *gin.Context) {
	id, e := parseID(c.Param("purchaseId"))
	if e != nil {
		fail(c, e)
		return
	}
	v, e := service.GetERPPurchase(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseAdd(c *gin.Context) {
	var in model.ERPPurchaseWrite
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	v, e := service.CreateERPPurchase(c.Request.Context(), in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseEdit(c *gin.Context) {
	id, e := parseID(c.Param("purchaseId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPPurchaseEdit
	if e = c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	v, e := service.EditERPPurchase(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, v)
}
func ERPPurchaseAct(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("purchaseId"))
		if e != nil {
			fail(c, e)
			return
		}
		var in model.ERPPurchaseAction
		if e = c.ShouldBindJSON(&in); e != nil {
			response.Fail(c, bindMessage(e))
			return
		}
		v, e := service.ActERPPurchase(c.Request.Context(), id, action, in, currentUsername(c))
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, v)
	}
}
func ERPPurchaseEvents(c *gin.Context) {
	id, e := parseID(c.Param("purchaseId"))
	if e != nil {
		fail(c, e)
		return
	}
	rows, n, e := service.ListERPPurchaseEvents(c.Request.Context(), id, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
