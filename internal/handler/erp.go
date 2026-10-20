package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ERPDocumentList(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var q model.ERPQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		rows, total, err := service.ListERPDocuments(c.Request.Context(), kind, q, page.Parse(c, nil))
		if err != nil {
			fail(c, err)
			return
		}
		response.Page(c, rows, total)
	}
}
func ERPDocumentGet(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("documentId"))
		if err != nil {
			fail(c, err)
			return
		}
		doc, err := service.GetERPDocument(c.Request.Context(), kind, id)
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, doc)
	}
}
func ERPDocumentAdd(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input model.ERPDocumentInput
		if err := c.ShouldBindJSON(&input); err != nil {
			response.Fail(c, bindMessage(err))
			return
		}
		doc, err := service.CreateERPDocument(c.Request.Context(), kind, input, currentUsername(c))
		if err != nil {
			fail(c, err)
			return
		}
		response.OkData(c, doc)
	}
}
func ERPDocumentAction(kind, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c.Param("documentId"))
		if err != nil {
			fail(c, err)
			return
		}
		if err = service.ChangeERPDocument(c.Request.Context(), kind, id, action, currentUsername(c)); err != nil {
			fail(c, err)
			return
		}
		response.Ok(c)
	}
}
func ERPStockList(c *gin.Context) {
	var q model.ERPQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	rows, total, err := service.ListERPStocks(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ERPLedgerList(c *gin.Context) {
	var q model.ERPQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	rows, total, err := service.ListERPLedger(c.Request.Context(), q, page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
