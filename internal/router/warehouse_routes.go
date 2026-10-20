package router

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
)

func init() {
	generatedRouteRegistrars = append(generatedRouteRegistrars, registerErpWarehouseGenerated)
}

func registerErpWarehouseGenerated(g *gin.RouterGroup) {
	group := g.Group("/erp/warehouse", middleware.CustomerWorkspaceOnly())
	group.GET("/list", middleware.HasPermission("erp:warehouse:list"), handler.ErpWarehouseList)
	group.POST("/export", middleware.ExportLimit(), middleware.HasPermission("erp:warehouse:export"), middleware.OperLog("仓库", model.BusinessTypeExport), handler.ErpWarehouseExport)
	group.GET("/:id", middleware.HasPermission("erp:warehouse:query"), handler.ErpWarehouseGet)
	group.POST("", noRepeat(), middleware.HasPermission("erp:warehouse:add"), middleware.OperLog("仓库", model.BusinessTypeInsert), handler.ErpWarehouseAdd)
	group.PUT("", middleware.HasPermission("erp:warehouse:edit"), middleware.OperLog("仓库", model.BusinessTypeUpdate), handler.ErpWarehouseEdit)
	group.DELETE("/:ids", middleware.HasPermission("erp:warehouse:remove"), middleware.OperLog("仓库", model.BusinessTypeDelete), handler.ErpWarehouseRemove)
}
