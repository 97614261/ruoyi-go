package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func OverviewSection(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var out any
		var e error
		switch kind {
		case "orders":
			out, e = service.OverviewOrders(c.Request.Context())
		case "shipments":
			out, e = service.OverviewShipments(c.Request.Context())
		case "stocks":
			out, e = service.OverviewStocks(c.Request.Context())
		case "purchases":
			out, e = service.OverviewPurchases(c.Request.Context())
		case "payables", "receivables":
			var in model.OverviewMoneyQuery
			if e = c.ShouldBindQuery(&in); e != nil {
				response.Fail(c, bindMessage(e))
				return
			}
			out, e = service.OverviewMoney(c.Request.Context(), in, page.Parse(c, nil), kind == "receivables")
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, out)
	}
}
