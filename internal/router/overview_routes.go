package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerOverview) }
func registerOverview(g *gin.RouterGroup) {
	p := g.Group("/erp/overview", middleware.CustomerWorkspaceOnly(), middleware.HasPermission("erp:overview:list"), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	for _, s := range []struct{ path, permission string }{{"orders", "erp:order:list"}, {"shipments", "erp:shipment:list"}, {"stocks", "erp:stock:list"}, {"purchases", "erp:purchase:list"}, {"payables", "erp:payable:list"}, {"receivables", "erp:receivable:list"}} {
		p.GET("/"+s.path, middleware.HasPermission(s.permission), handler.OverviewSection(s.path))
	}
}
