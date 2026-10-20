package router

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerErpSkuGenerated) }

func registerErpSkuGenerated(g *gin.RouterGroup) {
	group := g.Group("/erp/sku", middleware.CustomerWorkspaceOnly())
	group.GET("/list", middleware.HasPermission("erp:sku:list"), handler.ErpSkuList)
	group.POST("/export", middleware.ExportLimit(), middleware.HasPermission("erp:sku:export"), middleware.OperLog("商品SKU", model.BusinessTypeExport), handler.ErpSkuExport)
	group.GET("/:id", middleware.HasPermission("erp:sku:query"), handler.ErpSkuGet)
	group.POST("", noRepeat(), middleware.HasPermission("erp:sku:add"), middleware.OperLog("商品SKU", model.BusinessTypeInsert), handler.ErpSkuAdd)
	group.PUT("", middleware.HasPermission("erp:sku:edit"), middleware.OperLog("商品SKU", model.BusinessTypeUpdate), handler.ErpSkuEdit)
	group.DELETE("/:ids", middleware.HasPermission("erp:sku:remove"), middleware.OperLog("商品SKU", model.BusinessTypeDelete), handler.ErpSkuRemove)
}
