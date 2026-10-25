package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPSalesReturns) }
func registerERPSalesReturns(g *gin.RouterGroup) {
	noStore := func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }
	p := g.Group("/erp/salesReturnOrder", middleware.CustomerWorkspaceOnly(), noStore)
	p.GET("/returnable/list", middleware.HasPermission("erp:salesReturnOrder:list"), handler.ERPSalesReturnableList)
	p.GET("/list", middleware.HasPermission("erp:salesReturnOrder:list"), handler.ERPSalesReturnList)
	p.GET("/:returnId", middleware.HasPermission("erp:salesReturnOrder:query"), handler.ERPSalesReturnGet)
	r := g.Group("/erp/package", middleware.CustomerWorkspaceOnly(), noStore)
	r.GET("/:packageId/returnable", middleware.HasPermission("erp:salesReturnOrder:return"), handler.ERPSalesReturnableGet)
	r.POST("/:packageId/returns", middleware.HasPermission("erp:salesReturnOrder:return"), handler.ERPSalesReturnPost)
}
