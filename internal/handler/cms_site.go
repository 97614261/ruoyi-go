package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

func CMSGetSite(c *gin.Context) {
	row, err := service.CMSGetSite(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.OkData(c, row)
}
func CMSSaveSite(c *gin.Context) {
	var in model.SiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CMSSaveSite(c.Request.Context(), in, currentUsername(c), middleware.CurrentUser(c).HasPermission("cms:asset:query")); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func CMSPublishSite(c *gin.Context) {
	var in model.SiteAction
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CMSPublishSite(c.Request.Context(), in, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
func CMSSiteHistory(c *gin.Context) {
	rows, total, err := service.CMSSiteHistory(c.Request.Context(), page.Parse(c, nil))
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, rows, total)
}
func (p *PortalUI) PreviewSite(c *gin.Context) {
	state, err := service.CMSGetSite(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	if _, err = service.ValidateSite(state.Config); err != nil {
		fail(c, err)
		return
	}
	cfg, err := service.CMSResolveSite(c.Request.Context(), state.Config)
	if err != nil {
		fail(c, err)
		return
	}
	c.Set("portal-site-config", cfg)
	d, err := p.homeData(c, cfg)
	if err != nil {
		fail(c, err)
		return
	}
	d.NoIndex = true
	p.decorate(&d, cfg)
	body, err := p.view.Render(cfg.Theme, "home", d)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	response.OkData(c, map[string]any{"html": string(body)})
}
