package router

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/service"
)

// NewWithPortal keeps all existing API registration and limits SPA fallback to /admin/.
func NewWithPortal(cfg *config.Config, ui *handler.PortalUI) *gin.Engine {
	r := New(cfg)
	if ui != nil {
		registerPortal(r, ui)
		r.POST("/inquiry", middleware.RequestBodyLimit(16<<10), ui.SubmitInquiry)
		registerCMS(r, cfg)
		registerDownloadUpload(r)
		r.GET("/cms/presets/:id/preview", middleware.Auth(), middleware.HasPermission("cms:preset:list"), middleware.HasPermission("cms:site:query"), func(c *gin.Context) {
			u := middleware.CurrentUser(c)
			c.Request = c.Request.WithContext(service.CMSActorContext(c.Request.Context(), u.UserID))
			c.Next()
		}, ui.PresetPreview)
		r.POST("/cms/site/compare", middleware.Auth(), middleware.RequestBodyLimit(16<<10), middleware.HasPermission("cms:site:query"), ui.CompareThemes)
		r.GET("/cms/review/:id/preview", middleware.Auth(), ui.PreviewReviewSite)
		r.GET("/cms/site/preview", middleware.Auth(), middleware.HasPermission("cms:site:query"), ui.PreviewSite)
	}
	return r
}

func registerPortal(r *gin.Engine, ui *handler.PortalUI) {
	r.NoRoute(ui.RedirectOr404)
	r.HEAD("/news/:slug", ui.Content("ARTICLE"))
	r.HEAD("/products/:slug", ui.Content("PRODUCT"))
	r.HEAD("/p/:slug", ui.Content("PAGE"))
	r.GET("/", ui.Home)
	r.GET("/inquiry", ui.Inquiry)
	r.GET("/downloads", ui.Downloads)
	r.GET("/downloads/:id/file", publicDownloadLimit(), handler.PortalDownload)
	r.HEAD("/downloads/:id/file", publicDownloadLimit(), handler.PortalDownload)
	r.GET("/inquiry/thanks", ui.InquiryThanks)
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
