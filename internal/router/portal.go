package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
)

// NewWithPortal keeps all existing API registration and limits SPA fallback to /admin/.
func NewWithPortal(cfg *config.Config, ui *handler.PortalUI) *gin.Engine {
	r := New(cfg)
	if ui != nil {
		registerPortal(r, ui)
		registerCMS(r, cfg)
		r.GET("/cms/site/preview", middleware.Auth(), middleware.HasPermission("cms:site:query"), ui.PreviewSite)
	}
	return r
}

func registerPortal(r *gin.Engine, ui *handler.PortalUI) {
	r.GET("/", ui.Home)
	r.GET("/news", ui.Listing("ARTICLE"))
	r.GET("/news/c/:slug", ui.Listing("ARTICLE"))
	r.GET("/products", ui.Listing("PRODUCT"))
	r.GET("/products/c/:slug", ui.Listing("PRODUCT"))
	r.GET("/products/:slug", ui.Content("PRODUCT"))
	r.GET("/search", ui.Listing(""))
	r.GET("/robots.txt", ui.Robots)
	r.GET("/sitemap.xml", ui.Sitemap)
	r.GET("/sitemap/:file", ui.Sitemap)
	r.GET("/portal/articles", handler.CMSPublicList("ARTICLE"))
	r.GET("/portal/pages", handler.CMSPublicList("PAGE"))
	r.GET("/news/:slug", ui.Content("ARTICLE"))
	r.GET("/p/:slug", ui.Content("PAGE"))
	r.GET("/media/:id", handler.CMSPublicMedia)
	r.HEAD("/media/:id", handler.CMSPublicMedia)
	r.GET("/admin", func(c *gin.Context) { c.Redirect(301, "/admin/") })
	r.GET("/admin/*file", ui.Admin)
	r.HEAD("/admin/*file", ui.Admin)
	r.GET("/assets/portal/:theme/:version/*file", ui.Asset)
	r.HEAD("/assets/portal/:theme/:version/*file", ui.Asset)
}
