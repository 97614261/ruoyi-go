package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerWarehouseOperations) }
func registerWarehouseOperations(g *gin.RouterGroup) {
	p := g.Group("/erp", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	p.POST("/stock/export", middleware.ExportLimit(), middleware.HasPermission("erp:stock:export"), handler.ERPInventoryExport(false))
	p.POST("/ledger/export", middleware.ExportLimit(), middleware.HasPermission("erp:ledger:export"), handler.ERPInventoryExport(true))
	p.GET("/stock/alert", middleware.HasPermission("erp:stock:alert"), handler.ERPAlertGet)
	p.PUT("/stock/alert", middleware.HasPermission("erp:stock:alert"), handler.ERPAlertSet)
	p.GET("/stock/reconcile", middleware.HasPermission("erp:stock:reconcile"), handler.ERPReconcile)
	p.GET("/stock/reservations", middleware.HasPermission("erp:stock:list"), middleware.HasPermission("erp:sales:query"), handler.ERPReservations)
	t := p.Group("/transfer")
	t.GET("/list", middleware.HasPermission("erp:transfer:list"), handler.ERPTransferList)
	t.GET("/:id", middleware.HasPermission("erp:transfer:query"), handler.ERPTransferGet)
	t.POST("", middleware.HasPermission("erp:transfer:add"), handler.ERPTransferCreate)
	t.PUT("/:id", middleware.HasPermission("erp:transfer:edit"), handler.ERPTransferChange("EDIT"))
	t.POST("/:id/dispatch", middleware.HasPermission("erp:transfer:dispatch"), handler.ERPTransferChange("DISPATCH"))
	t.POST("/:id/receive", middleware.HasPermission("erp:transfer:receive"), handler.ERPTransferChange("RECEIVE"))
	t.POST("/:id/cancel", middleware.HasPermission("erp:transfer:cancel"), handler.ERPTransferChange("CANCEL"))
}
