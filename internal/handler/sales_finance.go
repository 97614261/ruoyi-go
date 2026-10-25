package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// Each route binds a dedicated whitelist DTO. No generic body OperLog and no
// frontend persistent repeat-submit cache is used for financial payloads.
func SalesFinanceList(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.SalesFinQuery
		if e := c.ShouldBindQuery(&q); e != nil {
			response.Fail(c, bindMessage(e))
			return
		}
		pg := page.Parse(c, nil)
		var rows any
		var n int64
		var e error
		switch kind {
		case "source":
			rows, n, e = service.ListSalesFinanceSources(c.Request.Context(), q, pg)
		case "receivable":
			rows, n, e = service.ListSalesFinanceReceivables(c.Request.Context(), q, pg)
		case "collection":
			rows, n, e = service.ListSalesFinanceCollections(c.Request.Context(), q, pg)
		case "statement":
			rows, n, e = service.ListSalesFinanceStatements(c.Request.Context(), q, pg)
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func SalesFinanceGet(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var out any
		switch kind {
		case "source":
			out, e = service.GetSalesFinanceSource(c.Request.Context(), id)
		case "receivable":
			out, e = service.GetSalesFinanceReceivable(c.Request.Context(), id)
		case "collection":
			out, e = service.GetSalesFinanceCollection(c.Request.Context(), id)
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, out)
	}
}
func SalesFinanceWrite(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		var e error
		if c.Param("id") != "" {
			id, e = parseID(c.Param("id"))
			if e != nil {
				fail(c, e)
				return
			}
		}
		var out *model.FinResult
		var bindErr error
		switch action {
		case "RECEIVABLE_CREATE":
			var in model.SalesFinReceivableCreate
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.CreateSalesFinanceReceivable(c.Request.Context(), in, currentUsername(c))
			}
		case "RECEIVABLE_ADJUST":
			var in model.SalesFinAdjust
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.AdjustSalesFinanceReceivable(c.Request.Context(), id, in, currentUsername(c))
			}
		case "COLLECTION_CREATE":
			var in model.SalesFinCollectionWrite
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.CreateSalesFinanceCollection(c.Request.Context(), in, currentUsername(c))
			}
		case "COLLECTION_REVERSE":
			var in model.FinReverse
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.ReverseSalesFinanceCollection(c.Request.Context(), id, in, currentUsername(c))
			}
		}
		if bindErr != nil {
			response.Fail(c, bindMessage(bindErr))
			return
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, out)
	}
}
func SalesFinanceReturns(c *gin.Context) {
	id, e := parseID(c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	pending := c.Query("pending")
	if pending != "" && pending != "true" && pending != "false" {
		response.Fail(c, "pending须为true/false")
		return
	}
	rows, n, e := service.ListSalesFinanceReturns(c.Request.Context(), id, pending != "false", page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}

type salesSalesFinExportRow struct {
	Number       string `excel:"name:应收单号"`
	Receipt      string `excel:"name:已发货订单"`
	Shop         string `excel:"name:店铺快照"`
	Currency     string `excel:"name:币种"`
	Gross        string `excel:"name:登记毛应收"`
	Credit       string `excel:"name:已登记退货抵减"`
	Paid         string `excel:"name:有效已收款"`
	Due          string `excel:"name:待收款"`
	Overreceived string `excel:"name:超收待处理"`
}

func SalesFinanceExport(statement bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.SalesFinQuery
		if e := c.ShouldBind(&q); e != nil {
			response.FailDownload(c, response.CodeError, bindMessage(e))
			return
		}
		rows := []salesSalesFinExportRow{}
		if statement {
			out, e := service.ExportSalesFinanceStatements(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			for _, r := range out {
				rows = append(rows, salesSalesFinExportRow{Shop: r.ShopName, Currency: r.Currency, Gross: r.GrossAmount, Credit: r.CreditAmount, Paid: r.ReceivedAmount, Due: r.DueAmount, Overreceived: r.OverreceivedAmount})
			}
		} else {
			out, e := service.ExportSalesFinanceReceivables(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			for _, r := range out {
				rows = append(rows, salesSalesFinExportRow{Number: r.ReceivableNo, Receipt: r.OrderNo, Shop: r.ShopName, Currency: r.Currency, Gross: r.GrossAmount, Credit: r.CreditAmount, Paid: r.ReceivedAmount, Due: r.DueAmount, Overreceived: r.OverreceivedAmount})
			}
		}
		if e := excelx.WriteResponse(c, "销售财务登记账.xlsx", "登记账", rows); e != nil {
			failDownload(c, e)
		}
	}
}
