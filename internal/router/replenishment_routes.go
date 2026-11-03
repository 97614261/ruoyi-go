package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPReplenishment) }
func registerERPReplenishment(g *gin.RouterGroup) {
	p := g.Group("/erp/replenishment", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }, middleware.HasPermission("erp:stock:list"), middleware.HasPermission("erp:purchase:list"))
	p.GET("/list", handler.ERPReplenishmentList)
	p.POST("", middleware.HasPermission("erp:purchase:add"), handler.ERPReplenishmentCreate)
}
