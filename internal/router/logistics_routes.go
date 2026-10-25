package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerLogistics) }
func registerLogistics(g *gin.RouterGroup) {
	p := g.Group("/erp/logistics", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	for _, s := range []struct {
		path   string
		method bool
	}{{"/carriers", false}, {"/shippingMethods", true}} {
		m := p.Group(s.path)
		m.GET("", middleware.HasPermission("erp:logistics:list"), handler.LogisticsMasterList(s.method))
		m.GET("/:id", middleware.HasPermission("erp:logistics:query"), handler.LogisticsMasterGet(s.method))
		m.POST("", middleware.HasPermission("erp:logistics:add"), handler.LogisticsMasterWrite(s.method))
		m.PUT("/:id", middleware.HasPermission("erp:logistics:edit"), handler.LogisticsMasterWrite(s.method))
	}
	p.GET("/options", middleware.HasPermission("erp:package:edit"), handler.LogisticsOptions)
	p.POST("/check-tracking", middleware.HasPermission("erp:package:edit"), handler.LogisticsTrackingCheck)
	p.GET("/shipments", middleware.HasPermission("erp:shipment:list"), handler.LogisticsShipments)
	p.POST("/shipments/export", middleware.ExportLimit(), middleware.HasPermission("erp:shipment:export"), handler.LogisticsShipmentsExport)
	s := g.Group("/erp/package", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	// Gin requires the same wildcard name as the existing package routes.
	s.GET("/:packageId/shipping", middleware.HasPermission("erp:package:query"), handler.PackageShippingGet)
	s.PUT("/:packageId/shipping", middleware.HasPermission("erp:package:edit"), handler.PackageShippingSave)
}
