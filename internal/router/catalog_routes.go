package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

func init() { generatedRouteRegistrars = append(generatedRouteRegistrars, registerCatalog) }
func registerCatalog(g *gin.RouterGroup) {
	p := g.Group("/erp/product", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	p.GET("/list", middleware.HasPermission("erp:product:list"), handler.CatalogList(false))
	p.GET("/template", middleware.HasPermission("erp:product:import"), handler.CatalogTemplate)
	p.POST("/import", middleware.HasPermission("erp:product:import"), handler.CatalogImport)
	p.POST("/export", middleware.ExportLimit(), middleware.HasPermission("erp:product:export"), handler.CatalogExport)
	p.POST("/image", middleware.HasPermission("erp:product:image"), handler.CatalogImageUpload)
	p.GET("/:id", middleware.HasPermission("erp:product:query"), handler.CatalogGet)
	p.POST("", middleware.HasPermission("erp:product:add"), handler.CatalogWrite(false))
	p.PUT("/:id", middleware.HasPermission("erp:product:edit"), handler.CatalogWrite(false))
	c := g.Group("/erp/productCategory", middleware.CustomerWorkspaceOnly(), func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	c.GET("/list", middleware.HasPermission("erp:productCategory:list"), handler.CatalogList(true))
	c.POST("", middleware.HasPermission("erp:productCategory:add"), handler.CatalogWrite(true))
	c.PUT("/:id", middleware.HasPermission("erp:productCategory:edit"), handler.CatalogWrite(true))
}
