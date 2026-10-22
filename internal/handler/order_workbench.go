package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

func ERPOrderWorkbench(c *gin.Context) {
	var q model.ERPOrderQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, "订单统计查询条件不正确")
		return
	}
	out, err := service.ERPOrderWorkbench(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ERPOrderReadiness(c *gin.Context) {
	id, err := parseID(c.Param("orderId"))
	if err != nil {
		fail(c, err)
		return
	}
	var q struct {
		WarehouseID int64 `form:"warehouseId" binding:"required,min=1"`
	}
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, "请选择有效的发货仓库")
		return
	}
	out, err := service.CheckERPOrderReadiness(c.Request.Context(), id, q.WarehouseID)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
