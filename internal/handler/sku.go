package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func ErpSkuList(c *gin.Context) {
	var query model.ErpSkuQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	list, total, err := service.ListErpSkuPage(c.Request.Context(), query, page.Parse(c, model.ErpSkuSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

func ErpSkuExport(c *gin.Context) {
	var query model.ErpSkuQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}
	list, err := service.ListErpSkuExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "商品SKU数据.xlsx", "商品SKU数据", list); err != nil {
		failDownload(c, err)
	}
}

func ErpSkuGet(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.Fail(c, "参数格式错误")
		return
	}
	target, err := service.GetErpSku(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, target)
}

func ErpSkuAdd(c *gin.Context) {
	var target model.ErpSku
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateErpSku(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func ErpSkuEdit(c *gin.Context) {
	var target model.ErpSku
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateErpSku(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func ErpSkuRemove(c *gin.Context) {
	raw := strings.Split(c.Param("ids"), ",")
	if len(raw) == 0 || len(raw) > 200 {
		response.Fail(c, "参数格式错误")
		return
	}
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		if id := strings.TrimSpace(item); id != "" {
			ids = append(ids, id)
		}
	}
	if err := service.DeleteErpSku(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
