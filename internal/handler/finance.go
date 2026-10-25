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
func FinanceList(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.FinQuery
		if e := c.ShouldBindQuery(&q); e != nil {
			response.Fail(c, bindMessage(e))
			return
		}
		pg := page.Parse(c, nil)
		var rows any
		var n int64
		var e error
		switch kind {
		case "account":
			rows, n, e = service.ListFinanceAccounts(c.Request.Context(), q, pg)
		case "source":
			rows, n, e = service.ListFinanceSources(c.Request.Context(), q, pg)
		case "payable":
			rows, n, e = service.ListFinancePayables(c.Request.Context(), q, pg)
		case "payment":
			rows, n, e = service.ListFinancePayments(c.Request.Context(), q, pg)
		case "statement":
			rows, n, e = service.ListFinanceStatements(c.Request.Context(), q, pg)
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.Page(c, rows, n)
	}
}
func FinanceGet(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, e := parseID(c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		var out any
		switch kind {
		case "account":
			out, e = service.GetFinanceAccount(c.Request.Context(), id)
		case "source":
			out, e = service.GetFinanceSource(c.Request.Context(), id)
		case "payable":
			out, e = service.GetFinancePayable(c.Request.Context(), id)
		case "payment":
			out, e = service.GetFinancePayment(c.Request.Context(), id)
		}
		if e != nil {
			fail(c, e)
			return
		}
		response.OkData(c, out)
	}
}
func FinanceWrite(action string) gin.HandlerFunc {
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
		case "ACCOUNT_CREATE", "ACCOUNT_EDIT":
			var in model.FinAccountWrite
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				if action == "ACCOUNT_CREATE" {
					out, e = service.CreateFinanceAccount(c.Request.Context(), in, currentUsername(c))
				} else {
					out, e = service.EditFinanceAccount(c.Request.Context(), id, in, currentUsername(c))
				}
			}
		case "PAYABLE_CREATE":
			var in model.FinPayableCreate
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.CreateFinancePayable(c.Request.Context(), in, currentUsername(c))
			}
		case "PAYABLE_ADJUST":
			var in model.FinAdjust
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.AdjustFinancePayable(c.Request.Context(), id, in, currentUsername(c))
			}
		case "PAYMENT_CREATE":
			var in model.FinPaymentWrite
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.CreateFinancePayment(c.Request.Context(), in, currentUsername(c))
			}
		case "PAYMENT_REVERSE":
			var in model.FinReverse
			bindErr = c.ShouldBindJSON(&in)
			if bindErr == nil {
				out, e = service.ReverseFinancePayment(c.Request.Context(), id, in, currentUsername(c))
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
func FinanceReturns(c *gin.Context) {
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
	rows, n, e := service.ListFinanceReturns(c.Request.Context(), id, pending != "false", page.Parse(c, nil))
	if e != nil {
		fail(c, e)
		return
	}
	response.Page(c, rows, n)
}

type finExportRow struct {
	Number   string `excel:"name:应付单号"`
	Receipt  string `excel:"name:收货批次"`
	Supplier string `excel:"name:供应商快照"`
	Currency string `excel:"name:币种"`
	Gross    string `excel:"name:登记毛应付"`
	Credit   string `excel:"name:已登记退货抵减"`
	Paid     string `excel:"name:有效已付款"`
	Due      string `excel:"name:待付款"`
	Overpaid string `excel:"name:超付待处理"`
}

func FinanceExport(statement bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.FinQuery
		if e := c.ShouldBind(&q); e != nil {
			response.FailDownload(c, response.CodeError, bindMessage(e))
			return
		}
		rows := []finExportRow{}
		if statement {
			out, e := service.ExportFinanceStatements(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			for _, r := range out {
				rows = append(rows, finExportRow{Supplier: r.SupplierName, Currency: r.Currency, Gross: r.GrossAmount, Credit: r.CreditAmount, Paid: r.PaidAmount, Due: r.DueAmount, Overpaid: r.OverpaidAmount})
			}
		} else {
			out, e := service.ExportFinancePayables(c.Request.Context(), q)
			if e != nil {
				failDownload(c, e)
				return
			}
			for _, r := range out {
				rows = append(rows, finExportRow{Number: r.PayableNo, Receipt: r.ReceiptNo, Supplier: r.SupplierName, Currency: r.Currency, Gross: r.GrossAmount, Credit: r.CreditAmount, Paid: r.PaidAmount, Due: r.DueAmount, Overpaid: r.OverpaidAmount})
			}
		}
		if e := excelx.WriteResponse(c, "采购财务登记账.xlsx", "登记账", rows); e != nil {
			failDownload(c, e)
		}
	}
}
