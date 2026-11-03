package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPReplenishmentList(c *gin.Context) {
	var q model.ERPReplenishmentQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	rows, n, e := service.ListERPReplenishments(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}
func ERPReplenishmentCreate(c *gin.Context) {
	var in model.ERPReplenishmentWrite
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	r, e := service.CreateERPReplenishmentPurchase(c.Request.Context(), in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, r)
}
