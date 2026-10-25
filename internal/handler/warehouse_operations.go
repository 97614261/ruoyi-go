package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
	"strconv"
)

func ERPTransferList(c *gin.Context) {
	var q model.ERPTransferQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	out, n, e := service.ERPTransferList(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, out, n)
}
func ERPTransferGet(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	out, e := service.ERPTransferGet(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPTransferCreate(c *gin.Context) {
	var in model.ERPTransferInput
	if e := c.ShouldBindJSON(&in); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	out, e := service.ERPTransferCreate(c.Request.Context(), in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPTransferChange(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var in model.ERPTransferInput
		if action == "EDIT" {
			e = c.ShouldBindJSON(&in)
		} else {
			var op model.ERPTransferOperation
			e = c.ShouldBindJSON(&op)
			in.Version = op.Version
			in.RequestKey = op.RequestKey
		}
		if e != nil {
			response.Fail(c, bindMessage(e))
			return
		}
		out, e := service.ERPTransferChange(c.Request.Context(), id, action, in, currentUsername(c))
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, out)
	}
}
func ERPAlertGet(c *gin.Context) {
	var q model.ERPQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	out, e := service.ERPAlertGet(c.Request.Context(), q.WarehouseID, q.SkuID)
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPAlertSet(c *gin.Context) {
	var req struct {
		WarehouseID int64           `json:"warehouseId" binding:"required,min=1"`
		SkuID       int64           `json:"skuId" binding:"required,min=1"`
		Version     *int64          `json:"version" binding:"required,min=0"`
		Threshold   json.RawMessage `json:"thresholdQuantity" binding:"required"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	var value *int64
	if json.Unmarshal(req.Threshold, &value) != nil {
		response.Fail(c, "阈值必须为整数或显式null")
		return
	}
	in := model.ERPStockAlert{WarehouseID: req.WarehouseID, SkuID: req.SkuID, Version: *req.Version, ThresholdQuantity: value}
	out, e := service.ERPAlertSet(c.Request.Context(), in, currentUsername(c))
	if e != nil {
		fail(c, e)
		return
	}
	response.OkData(c, out)
}
func ERPReconcile(c *gin.Context) {
	var q model.ERPQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	out, n, e := service.ERPStockReconcile(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, out, n)
}
func ERPReservations(c *gin.Context) {
	var q model.ERPQuery
	if e := c.ShouldBindQuery(&q); e != nil {
		response.Fail(c, bindMessage(e))
		return
	}
	out, n, e := service.ERPStockReservations(c.Request.Context(), q, page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, out, n)
}

// Dedicated flat export DTOs: no private master fields or extra struct columns.
type stockExport struct {
	Warehouse string `excel:"name:仓库"`
	SKU       string `excel:"name:SKU编码"`
	Product   string `excel:"name:商品"`
	Unit      string `excel:"name:单位"`
	OnHand    int64  `excel:"name:实物"`
	Reserved  int64  `excel:"name:预占"`
	Available int64  `excel:"name:可用"`
	Threshold string `excel:"name:最低库存"`
}
type ledgerExport struct {
	Warehouse     string `excel:"name:仓库"`
	SKU           string `excel:"name:SKU编码"`
	Product       string `excel:"name:商品"`
	Unit          string `excel:"name:单位"`
	Document      string `excel:"name:来源单号"`
	Event         string `excel:"name:事件"`
	HandDelta     int64  `excel:"name:实物变化"`
	ReservedDelta int64  `excel:"name:预占变化"`
	HandAfter     int64  `excel:"name:实物结余"`
	ReservedAfter int64  `excel:"name:预占结余"`
	Time          string `excel:"name:时间"`
}

func ERPInventoryExport(ledger bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.ERPQuery
		if e := c.ShouldBind(&q); e != nil {
			response.FailDownload(c, response.CodeError, bindMessage(e))
			return
		}
		if ledger {
			out, e := service.ERPLedgerExport(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			rows := make([]ledgerExport, 0, len(out))
			for _, r := range out {
				rows = append(rows, ledgerExport{r.WarehouseName, r.SkuCode, r.ProductName, r.Unit, r.DocumentNo, r.Event, r.OnHandDelta, r.ReservedDelta, r.OnHandAfter, r.ReservedAfter, r.CreateTime.String()})
			}
			if e = excelx.WriteResponse(c, "库存流水.xlsx", "流水", rows); e != nil {
				failDownload(c, e)
			}
		} else {
			out, e := service.ERPStockExport(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			rows := make([]stockExport, 0, len(out))
			for _, r := range out {
				v := "未配置"
				if r.ThresholdQuantity != nil {
					v = strconv.FormatInt(*r.ThresholdQuantity, 10)
				}
				rows = append(rows, stockExport{r.WarehouseName, r.SkuCode, r.ProductName, r.Unit, r.OnHand, r.Reserved, r.Available, v})
			}
			if e = excelx.WriteResponse(c, "库存余额.xlsx", "库存", rows); e != nil {
				failDownload(c, e)
			}
		}
	}
}
