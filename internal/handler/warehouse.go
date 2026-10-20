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

func ErpWarehouseList(c *gin.Context) {
	var query model.ErpWarehouseQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, "查询参数错误")
		return
	}
	list, total, err := service.ListErpWarehousePage(c.Request.Context(), query, page.Parse(c, model.ErpWarehouseSortColumns))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

func ErpWarehouseExport(c *gin.Context) {
	var query model.ErpWarehouseQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}
	list, err := service.ListErpWarehouseExport(c.Request.Context(), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "仓库数据.xlsx", "仓库数据", list); err != nil {
		failDownload(c, err)
	}
}

func ErpWarehouseGet(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.Fail(c, "参数格式错误")
		return
	}
	target, err := service.GetErpWarehouse(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, target)
}

func ErpWarehouseAdd(c *gin.Context) {
	var target model.ErpWarehouse
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateErpWarehouse(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func ErpWarehouseEdit(c *gin.Context) {
	var target model.ErpWarehouse
	if err := c.ShouldBindJSON(&target); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateErpWarehouse(c.Request.Context(), &target, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func ErpWarehouseRemove(c *gin.Context) {
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
	if err := service.DeleteErpWarehouse(c.Request.Context(), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
