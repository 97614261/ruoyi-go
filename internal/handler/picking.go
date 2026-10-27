package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func PickingList(c *gin.Context) {
	var q model.ERPPackageQuery
	if c.ShouldBindQuery(&q) != nil {
		response.FailCode(c, 400, "拣货查询参数不正确")
		return
	}
	rows, total, e := service.ListPickingPackages(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		pickingFail(c, e)
		return
	}
	response.Page(c, rows, total)
}
func PickingGet(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		pickingFail(c, e)
		return
	}
	out, e := service.GetPickingSheet(c.Request.Context(), id)
	if e != nil {
		pickingFail(c, e)
		return
	}
	response.OkData(c, out)
}
func PickingCheck(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		pickingFail(c, e)
		return
	}
	var in model.PickingCheckInput
	if c.ShouldBindJSON(&in) != nil {
		response.FailCode(c, 400, "扫码复核参数不正确")
		return
	}
	out, e := service.ConfirmPacking(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		pickingFail(c, e)
		return
	}
	response.OkData(c, out)
}
func PickingShip(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		pickingFail(c, e)
		return
	}
	var in model.ERPActionToken
	if c.ShouldBindJSON(&in) != nil {
		response.FailCode(c, 400, "扫码发货参数不正确")
		return
	}
	out, e := service.ShipCheckedPackage(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		pickingFail(c, e)
		return
	}
	response.OkData(c, out)
}

func pickingFail(c *gin.Context, e error) {
	if b := errs.As(e); b != nil && b.Cause == nil {
		response.FailCode(c, 400, b.Msg)
		return
	}
	fail(c, e)
}
