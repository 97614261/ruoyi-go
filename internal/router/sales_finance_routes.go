package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerSalesFinance) }
func registerSalesFinance(g *gin.RouterGroup) {
	p := g.Group("/erp/salesFinance", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	s := p.Group("/sources", middleware.HasPermission("erp:receivable:add"))
	s.GET("", handler.SalesFinanceList("source"))
	s.GET("/:id", handler.SalesFinanceGet("source"))
	b := p.Group("/receivables")
	b.GET("", middleware.HasPermission("erp:receivable:list"), handler.SalesFinanceList("receivable"))
	b.GET("/:id", middleware.HasPermission("erp:receivable:query"), handler.SalesFinanceGet("receivable"))
	b.POST("", middleware.HasPermission("erp:receivable:add"), handler.SalesFinanceWrite("RECEIVABLE_CREATE"))
	b.GET("/:id/returns", middleware.HasPermission("erp:receivable:adjust"), handler.SalesFinanceReturns)
	b.POST("/:id/adjust", middleware.HasPermission("erp:receivable:adjust"), handler.SalesFinanceWrite("RECEIVABLE_ADJUST"))
	b.POST("/export", middleware.ExportLimit(), middleware.HasPermission("erp:receivable:export"), handler.SalesFinanceExport(false))
	m := p.Group("/collections")
	m.GET("", middleware.HasPermission("erp:collection:list"), handler.SalesFinanceList("collection"))
	m.GET("/:id", middleware.HasPermission("erp:collection:query"), handler.SalesFinanceGet("collection"))
	m.POST("", middleware.HasPermission("erp:collection:add"), handler.SalesFinanceWrite("COLLECTION_CREATE"))
	m.POST("/:id/reverse", middleware.HasPermission("erp:collection:reverse"), handler.SalesFinanceWrite("COLLECTION_REVERSE"))
	p.GET("/statements", middleware.HasPermission("erp:shopStatement:list"), handler.SalesFinanceList("statement"))
	p.POST("/statements/export", middleware.ExportLimit(), middleware.HasPermission("erp:shopStatement:export"), handler.SalesFinanceExport(true))
}
