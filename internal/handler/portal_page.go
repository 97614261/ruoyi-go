package handler

import (
	"encoding/xml"
	"github.com/gin-gonic/gin"
	"html/template"
	"net/http"
	"net/url"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/portalview"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (p *PortalUI) SetOrigin(origin string) { p.origin = strings.TrimRight(origin, "/") }
func cardRows(rows []model.PortalCard) []portalview.Card {
	out := []portalview.Card{}
	for _, r := range rows {
		out = append(out, portalview.Card{Title: r.Title, Summary: r.Summary, URL: r.URL, Kind: r.Kind, Cover: r.CoverAssetId})
	}
	return out
}
func (p *PortalUI) render(c *gin.Context, status int, name string, d portalview.PageData) {
	cfg, siteErr := p.siteConfig(c)
	theme := p.theme
	if siteErr == nil {
		p.decorate(&d, cfg)
		theme = cfg.Theme
	} else {
		status = 503
		name = "error"
		d.SiteName = p.siteName
		d.NoIndex = true
	}
	if d.Canonical == "" {
		d.Canonical = p.origin + c.Request.URL.EscapedPath()
	}
	if d.ThemeVersion != "" {
		manifest, ok := p.view.Manifest(theme)
		if !ok || manifest.Version != d.ThemeVersion {
			c.Status(503)
			return
		}
	}
	body, err := p.view.Render(theme, name, d)
	if err != nil {
		c.Status(500)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'self'; script-src 'none'; object-src 'none'; frame-src 'none'; base-uri 'none'; style-src 'self' 'unsafe-inline'; img-src 'self'; frame-ancestors 'none'")
	if status != 200 || d.NoIndex {
		c.Header("X-Robots-Tag", "noindex, nofollow")
	}
	c.Data(status, "text/html; charset=utf-8", body)
}
func (p *PortalUI) pageError(c *gin.Context, err error) {
	status, name, title := 500, "error", "暂时无法加载"
	if service.IsCMSNotFound(err) {
		status, name, title = 404, "notfound", "页面不存在"
	}
	p.render(c, status, name, portalview.PageData{Title: title, NoIndex: true})
}
func (p *PortalUI) Home(c *gin.Context) {
	cfg, err := p.siteConfig(c)
	if err != nil {
		c.Status(503)
		return
	}
	d, err := p.homeData(c, cfg)
	if err != nil {
		p.pageError(c, err)
		return
	}
	p.render(c, 200, "home", d)
}

func (p *PortalUI) Listing(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		pageNo := 1
		if v := c.Query("page"); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 1000 {
				c.Status(400)
				return
			}
			pageNo = n
		}
		q := strings.TrimSpace(c.Query("q"))
		if utf8.RuneCountInString(q) > 80 {
			c.Status(400)
			return
		}
		if kind != "" {
			q = ""
		}
		r, err := service.PortalResults(c.Request.Context(), kind, c.Param("slug"), q, (pageNo-1)*12, 12)
		if err != nil {
			p.pageError(c, err)
			return
		}
		if pageNo > 1 && int64((pageNo-1)*12) >= r.Total {
			p.pageError(c, service.PortalNotFound())
			return
		}
		title, name := "新闻动态", "column"
		if kind == "PRODUCT" {
			title = "产品展示"
		}
		if kind == "" {
			title, name = "搜索结果", "search"
		}
		d := portalview.PageData{Title: title, Description: title, Query: q, Cards: cardRows(r.Rows), NoIndex: kind == ""}
		for _, ch := range r.Channels {
			if ch.Kind == kind {
				prefix := "/news/c/"
				if kind == "PRODUCT" {
					prefix = "/products/c/"
				}
				d.Channels = append(d.Channels, portalview.Card{Title: ch.Name, URL: prefix + ch.Slug})
				if ch.Slug == c.Param("slug") {
					d.Title = ch.Name
					d.Description = ch.SeoDescription
					if ch.SeoTitle != "" {
						d.Title = ch.SeoTitle
					}
				}
			}
		}
		base := c.Request.URL.EscapedPath()
		link := func(n int) string {
			v := url.Values{}
			if q != "" {
				v.Set("q", q)
			}
			if n > 1 {
				v.Set("page", strconv.Itoa(n))
			}
			if len(v) > 0 {
				return base + "?" + v.Encode()
			}
			return base
		}
		d.Canonical = p.origin + link(pageNo)
		if pageNo > 1 {
			d.Prev = link(pageNo - 1)
		}
		if int64(pageNo*12) < r.Total {
			d.Next = link(pageNo + 1)
		}
		p.render(c, 200, name, d)
	}
}
func (p *PortalUI) Content(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		r, err := service.CMSPublicContent(c.Request.Context(), kind, c.Param("slug"))
		if err != nil {
			p.pageError(c, err)
			return
		}
		title := r.Title
		if r.SeoTitle != "" {
			title = r.SeoTitle
		}
		desc := r.SeoDescription
		if desc == "" {
			desc = r.Summary
		}
		name := "article"
		if kind == "PAGE" {
			name = "page"
		}
		if kind == "PRODUCT" {
			name = "product"
		}
		d := portalview.PageData{Title: title, Description: desc, BodyHTML: template.HTML(r.BodyHTML), ProductCode: r.ProductCode, ProductParams: r.ProductParams, Gallery: r.GalleryIds, CoverAlt: r.CoverAltText, Author: r.Author}
		if r.CoverAssetId != nil {
			d.Cover = *r.CoverAssetId
		}
		p.render(c, 200, name, d)
	}
}

type siteLoc struct {
	Loc string `xml:"loc"`
}

func (p *PortalUI) Sitemap(c *gin.Context) {
	part := c.Param("file")
	offset := 0
	if part != "" {
		n, e := strconv.Atoi(strings.TrimSuffix(part, ".xml"))
		if e != nil || n < 1 || n > 50 || !strings.HasSuffix(part, ".xml") {
			c.Status(404)
			return
		}
		offset = (n - 1) * 1000
	}
	r, err := service.PortalResults(c.Request.Context(), "", "", "", offset, 1000)
	if err != nil {
		c.Status(503)
		return
	}
	if r.Total > 50000 {
		c.Status(503)
		return
	}
	c.Header("Cache-Control", "no-store")
	if part == "" {
		items := []siteLoc{}
		for i := int64(0); i*1000 < r.Total || i == 0; i++ {
			items = append(items, siteLoc{p.origin + "/sitemap/" + strconv.FormatInt(i+1, 10) + ".xml"})
		}
		b, _ := xml.Marshal(struct {
			XMLName xml.Name  `xml:"sitemapindex"`
			NS      string    `xml:"xmlns,attr"`
			Items   []siteLoc `xml:"sitemap"`
		}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", Items: items})
		c.Data(200, "application/xml; charset=utf-8", append([]byte(xml.Header), b...))
		return
	}
	if len(r.Rows) == 0 && offset > 0 {
		c.Status(404)
		return
	}
	items := []siteLoc{}
	if offset == 0 {
		for _, path := range []string{"/", "/news", "/products"} {
			items = append(items, siteLoc{p.origin + path})
		}
		for _, ch := range r.Channels {
			prefix := "/news/c/"
			if ch.Kind == "PRODUCT" {
				prefix = "/products/c/"
			}
			items = append(items, siteLoc{p.origin + prefix + ch.Slug})
		}
	}
	for _, row := range r.Rows {
		items = append(items, siteLoc{p.origin + row.URL})
	}
	b, _ := xml.Marshal(struct {
		XMLName xml.Name  `xml:"urlset"`
		NS      string    `xml:"xmlns,attr"`
		Items   []siteLoc `xml:"url"`
	}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", Items: items})
	c.Data(200, "application/xml; charset=utf-8", append([]byte(xml.Header), b...))
}
func (p *PortalUI) Robots(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("User-agent: *\nDisallow: /admin/\nDisallow: /cms/\nDisallow: /search\nSitemap: "+p.origin+"/sitemap.xml\n"))
}
