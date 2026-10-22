package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPOrders) }
func registerERPOrders(g *gin.RouterGroup) {
	// No generic OperLog: immutable transaction audits contain IDs/versions only.
	// No generic noRepeat: persisted domain keys handle retries without PII cache.
	privateHeaders := func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }
	o := g.Group("/erp/order", middleware.CustomerWorkspaceOnly(), privateHeaders)
	o.GET("/list", middleware.HasPermission("erp:order:list"), handler.ERPOrderList)
	o.GET("/workbench", middleware.HasPermission("erp:order:list"), handler.ERPOrderWorkbench)
	o.GET("/:orderId/readiness", middleware.HasPermission("erp:order:review"), handler.ERPOrderReadiness)
	o.GET("/importTemplate", middleware.HasPermission("erp:order:import"), handler.ERPOrderTemplate)
	o.POST("/import", middleware.HasPermission("erp:order:import"), handler.ERPOrderImport)
	o.POST("/export", middleware.HasPermission("erp:order:export"), handler.ERPOrderExport)
	o.GET("/:orderId", middleware.HasPermission("erp:order:query"), handler.ERPOrderGet)
	o.POST("", middleware.HasPermission("erp:order:add"), handler.ERPOrderAdd)
	o.PUT("/:orderId", middleware.HasPermission("erp:order:edit"), handler.ERPOrderEdit)
	o.PUT("/:orderId/review", middleware.HasPermission("erp:order:review"), handler.ERPOrderReview)
	o.PUT("/:orderId/cancel", middleware.HasPermission("erp:order:cancel"), handler.ERPOrderCancel)
	p := g.Group("/erp/package", middleware.CustomerWorkspaceOnly(), privateHeaders)
	p.GET("/list", middleware.HasPermission("erp:package:list"), handler.ERPPackageList)
	p.GET("/:packageId", middleware.HasPermission("erp:package:query"), handler.ERPPackageGet)
	p.PUT("/:packageId/logistics", middleware.HasPermission("erp:package:edit"), handler.ERPPackageLogistics)
	p.PUT("/:packageId/ship", middleware.HasPermission("erp:package:ship"), handler.ERPPackageShip)
	j := g.Group("/erp/importJob", middleware.CustomerWorkspaceOnly(), privateHeaders)
	j.GET("/list", middleware.HasPermission("erp:importJob:list"), handler.ERPImportJobList)
	j.GET("/:jobId", middleware.HasPermission("erp:importJob:query"), handler.ERPImportJobGet)
	j.GET("/:jobId/results", middleware.HasPermission("erp:importJob:query"), handler.ERPImportJobResults)
	j.GET("/:jobId/file", middleware.HasPermission("erp:importJob:query"), handler.ERPImportJobFile)
	j.POST("/:jobId/retry", middleware.HasPermission("erp:importJob:query"), middleware.HasPermission("erp:order:import"), handler.ERPImportJobRetry)
}
