package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerPicking) }
func registerPicking(g *gin.RouterGroup) {
	p := g.Group("/erp/picking", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	p.GET("/list", middleware.HasPermission("erp:picking:list"), handler.PickingList)
	p.GET("/:packageId", middleware.HasPermission("erp:picking:query"), handler.PickingGet)
	p.PUT("/:packageId/check", middleware.HasPermission("erp:picking:check"), handler.PickingCheck)
	p.PUT("/:packageId/ship", middleware.HasPermission("erp:picking:ship"), handler.PickingShip)
}
