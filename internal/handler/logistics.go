package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// Explicit business refusals use400 on these new JSON endpoints. A raw DB/
// commit failure remains500 and may have an unknown outcome; the client must
// retain the original command. Legacy logistics/download responses stay intact.
func logisticsFail(c *gin.Context, e error) {
	if b := errs.As(e); b != nil {
		response.FailCode(c, 400, b.Msg)
		return
	}
	fail(c, e)
}

func LogisticsMasterList(method bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.LogisticsQuery
		if e := c.ShouldBindQuery(&q); e != nil {
			response.FailCode(c, 400, bindMessage(e))
			return
		}
		rows, n, e := service.ListLogisticsMasters(c.Request.Context(), q, page.Parse(c, nil), method)
		if e != nil {
			logisticsFail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func LogisticsMasterGet(method bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			logisticsFail(c, e)
			return
		}
		r, e := service.GetLogisticsMaster(c.Request.Context(), id, method)
		if e != nil {
			logisticsFail(c, e)
			return
		}
		response.OkData(c, r)
	}
}
func LogisticsMasterWrite(method bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		var e error
		if c.Param("id") != "" {
			id, e = parseID(c.Param("id"))
			if e != nil {
				logisticsFail(c, e)
				return
			}
		}
		var in model.LogisticsWrite
		if e = c.ShouldBindJSON(&in); e != nil {
			response.FailCode(c, 400, bindMessage(e))
			return
		}
		r, e := service.WriteLogisticsMaster(c.Request.Context(), id, in, method, currentUsername(c))
		if e != nil {
			logisticsFail(c, e)
			return
		}
		response.OkData(c, r)
	}
}
func LogisticsOptions(c *gin.Context) {
	var q model.LogisticsQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.FailCode(c, 400, bindMessage(e))
		return
	}
	r, n, e := service.ListLogisticsOptions(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		logisticsFail(c, e)
		return
	}
	response.Page(c, r, n)
}
func PackageShippingGet(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		logisticsFail(c, e)
		return
	}
	r, e := service.GetPackageShipping(c.Request.Context(), id)
	if e != nil {
		logisticsFail(c, e)
		return
	}
	response.OkData(c, r)
}
func PackageShippingSave(c *gin.Context) {
	id, e := parseID(c.Param("packageId"))
	if e != nil {
		logisticsFail(c, e)
		return
	}
	var in model.PackageShippingInput
	if e = c.ShouldBindJSON(&in); e != nil {
		response.FailCode(c, 400, bindMessage(e))
		return
	}
	r, e := service.SavePackageShipping(c.Request.Context(), id, in, currentUsername(c))
	if e != nil {
		logisticsFail(c, e)
		return
	}
	response.OkData(c, r)
}
func LogisticsTrackingCheck(c *gin.Context) {
	var in model.TrackingCheckInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.FailCode(c, 400, bindMessage(e))
		return
	}
	r, e := service.CheckLogisticsTracking(c.Request.Context(), in)
	if e != nil {
		logisticsFail(c, e)
		return
	}
	response.OkData(c, r)
}
func LogisticsShipments(c *gin.Context) {
	var q model.ShipmentQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.FailCode(c, 400, bindMessage(e))
		return
	}
	r, n, e := service.ListLogisticsShipments(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		logisticsFail(c, e)
		return
	}
	response.Page(c, r, n)
}
func LogisticsShipmentsExport(c *gin.Context) {
	var q model.ShipmentQuery
	if e := c.ShouldBind(&q); e != nil {
		response.FailDownload(c, response.CodeError, bindMessage(e))
		return
	}
	r, e := service.ExportLogisticsShipments(c.Request.Context(), q)
	if e != nil {
		failDownload(c, e)
		return
	}
	if e = excelx.WriteResponse(c, "发货记录.xlsx", "发货记录", r); e != nil {
		failDownload(c, e)
	}
}
