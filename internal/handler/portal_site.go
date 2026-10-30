package handler

import (
	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/portalview"
)

func (p *PortalUI) siteConfig(c *gin.Context) (model.SiteConfig, error) {
	if v, ok := c.Get("portal-site-config"); ok {
		return v.(model.SiteConfig), nil
	}
	cfg, err := service.CMSPublicSite(c.Request.Context())
	if err == nil {
		c.Set("portal-site-config", cfg)
	}
	return cfg, err
}
func (p *PortalUI) decorate(d *portalview.PageData, cfg model.SiteConfig) {
	d.SiteName = cfg.SiteName
	d.Icon = cfg.IconAssetId
	d.FontScale = cfg.FontScale
	d.FrameWidth = cfg.FrameWidth
	d.Radius = cfg.Radius
	d.Theme = cfg.Theme
	if cfg.Theme == "business" {
		d.ThemeVersion = "v3"
	} else {
		d.ThemeVersion = "v1"
	}
	if cfg.ThemeVersion != "" {
		d.ThemeVersion = cfg.ThemeVersion
	}
	d.Palette = cfg.Palette
	d.Density = cfg.Density
	d.Footer = cfg.Footer
	d.Phone = cfg.Phone
	d.Email = cfg.Email
	d.Logo = cfg.LogoAssetId
	for _, n := range cfg.Navigation {
		if n.Placement == "footer" {
			d.FooterLinks = append(d.FooterLinks, portalview.Card{Title: n.Label, URL: n.URL})
		} else {
			d.Links = append(d.Links, portalview.Card{Title: n.Label, URL: n.URL})
		}
	}
	for _, b := range cfg.Banners {
		d.Banners = append(d.Banners, portalview.Banner{Title: b.Title, URL: b.URL, AssetId: b.AssetId})
	}
}
func (p *PortalUI) homeData(c *gin.Context, cfg model.SiteConfig) (portalview.PageData, error) {
	d := portalview.PageData{Title: "首页", Description: cfg.Slogan}
	for _, block := range cfg.Blocks {
		if !block.Enabled {
			continue
		}
		r, err := service.PortalResults(c.Request.Context(), block.Kind, "", "", 0, block.Count)
		if err != nil {
			return d, err
		}
		if block.ChannelId > 0 {
			slug := ""
			for _, ch := range r.Channels {
				if ch.ChannelId == block.ChannelId && ch.Kind == block.Kind {
					slug = ch.Slug
				}
			}
			if slug == "" {
				continue
			}
			r, err = service.PortalResults(c.Request.Context(), block.Kind, slug, "", 0, block.Count)
			if err != nil {
				return d, err
			}
		}
		title, path := "了解我们", "/search"
		switch block.Kind {
		case "PRODUCT":
			title, path = "产品展示", "/products"
		case "ARTICLE":
			title, path = "最新动态", "/news"
		}
		d.Sections = append(d.Sections, portalview.Section{Title: title, URL: path, Cards: cardRows(r.Rows)})
	}
	return d, nil
}
