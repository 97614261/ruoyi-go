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
	d.ListingLayout = cfg.ListingLayout
	if d.IsListing && d.ChannelId > 0 {
		for _, o := range cfg.ChannelLayouts {
			if o.ChannelId == d.ChannelId {
				d.ListingLayout = o.Layout
				break
			}
		}
	}
	if t, ok := portalview.FindBuiltinTheme(cfg.Theme); ok {
		d.ThemeVersion = t.Version
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
	rows, err := service.CMSHomeRows(c.Request.Context(), cfg.Blocks)
	if err != nil {
		return d, err
	}
	for i, b := range cfg.Blocks {
		if !b.Enabled {
			continue
		}
		title, path := b.Title, ""
		if title == "" {
			title = map[string]string{"ARTICLE": "最新动态", "PRODUCT": "产品展示", "PAGE": "了解我们", "SERVICE": "服务介绍", "ADVANTAGE": "我们的优势", "CASE": "案例展示", "PARTNER": "合作伙伴", "FAQ": "常见问题"}[b.Kind]
		}
		if b.Kind == "ARTICLE" {
			path = "/news"
		}
		if b.Kind == "PRODUCT" {
			path = "/products"
		}
		sec := portalview.Section{Title: title, URL: path, Kind: b.Kind, Layout: b.Layout, Cards: cardRows(rows[i])}
		for _, v := range b.Items {
			sec.Items = append(sec.Items, portalview.WidgetItem{Title: v.Title, Text: v.Text, URL: v.URL, AssetId: v.AssetId})
		}
		d.Sections = append(d.Sections, sec)
	}
	return d, nil
}
