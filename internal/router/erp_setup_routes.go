package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerERPSetup) }
func registerERPSetup(g *gin.RouterGroup) {
	p := g.Group("/erp/setup", middleware.CustomerWorkspaceOnly(), middleware.TenantOwnerOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	p.GET("", middleware.HasPermission("erp:setup:list"), handler.ERPSetup)
	p.GET("/roleTemplates", middleware.HasPermission("system:role:add"), handler.ERPRoleTemplates)
}
