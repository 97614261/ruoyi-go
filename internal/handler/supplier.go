package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ErpSupplierList(c *gin.Context) {
	var in model.ErpSupplierQuery
	if err := c.ShouldBindQuery(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	rows, total, err := service.ListErpSupplierPage(c.Request.Context(), in, page.Parse(c, model.ErpSupplierSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func ErpSupplierGet(c *gin.Context) {
	id, err := parseID(c.Param("supplierId"))
	if err != nil {
		fail(c, err)
		return
	}
	out, err := service.GetErpSupplier(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ErpSupplierAdd(c *gin.Context) {
	var in model.ERPSupplierWrite
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	out, err := service.CreateErpSupplier(c.Request.Context(), in, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
func ErpSupplierEdit(c *gin.Context) {
	id, err := parseID(c.Param("supplierId"))
	if err != nil {
		fail(c, err)
		return
	}
	var in model.ERPSupplierEdit
	if err = c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	out, err := service.UpdateErpSupplier(c.Request.Context(), id, in, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, out)
}
