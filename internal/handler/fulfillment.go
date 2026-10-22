package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPOrderReview(c *gin.Context) {
	id, e := parseID(c.Param("orderId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPReviewInput
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "审核参数不正确")
		return
	}
	out, e := service.ReviewERPOrder(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPOrderCancel(c *gin.Context) {
	id, e := parseID(c.Param("orderId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPCancelInput
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "取消参数不正确")
		return
	}
	out, e := service.CancelERPOrder(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPPackageList(c *gin.Context) {
	var q model.ERPPackageQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, "包裹查询参数不正确")
		return
	}
	rows, total, e := service.ListERPPackages(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, total)
}
func ERPPackageGet(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		fail(c, e)
		return
	}
	out, e := service.GetERPPackage(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPPackageLogistics(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPLogisticsInput
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "物流参数不正确")
		return
	}
	out, e := service.SetERPPackageLogistics(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPPackageShip(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		fail(c, e)
		return
	}
	var in model.ERPActionToken
	if c.ShouldBindJSON(&in) != nil {
		response.Fail(c, "发货参数不正确")
		return
	}
	out, e := service.ShipERPPackage(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
