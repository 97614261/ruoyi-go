package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() {
	generatedRouteRegistrars = append(generatedRouteRegistrars, registerErpSupplierGenerated)
}
func registerErpSupplierGenerated(g *gin.RouterGroup) {
	group := g.Group("/erp/supplier", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	group.GET("/list", middleware.HasPermission("erp:supplier:list"), handler.ErpSupplierList)
	group.GET("/:supplierId", middleware.HasPermission("erp:supplier:query"), handler.ErpSupplierGet)
	// No generic OperLog or body repeat cache: private fields are never captured.
	// Writes audit only IDs/versions in the same repository transaction.
	group.POST("", middleware.HasPermission("erp:supplier:add"), handler.ErpSupplierAdd)
	group.PUT("/:supplierId", middleware.HasPermission("erp:supplier:edit"), handler.ErpSupplierEdit)
}
