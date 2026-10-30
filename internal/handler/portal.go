package handler

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"ruoyi-go/pkg/portalview"
)

// PortalUI serves registered, embedded resources only. It does not query business data.
type PortalUI struct {
	view      *portalview.Engine
	resources fs.FS
	admin     fs.FS
	theme     string
	siteName  string
	origin    string
}

func NewPortalUI(resources, admin fs.FS, theme, siteName string) (*PortalUI, error) {
	if admin == nil {
		return nil, fmt.Errorf("admin resources are required")
	}
	view, err := portalview.New(resources)
	if err != nil {
		return nil, err
	}
	if _, ok := view.Manifest(theme); !ok {
		return nil, fmt.Errorf("portal theme is not registered")
	}
	index, err := fs.ReadFile(admin, "index.html")
	if err != nil || !bytes.Contains(index, []byte(`/admin/`)) {
		return nil, fmt.Errorf("build admin-web with /admin/ base before enabling portal")
	}
	return &PortalUI{view: view, resources: resources, admin: admin, theme: theme, siteName: siteName, origin: "http://127.0.0.1:8085"}, nil
}

func safeAsset(name string) bool {
	if !fs.ValidPath(name) {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".css", ".svg", ".ico", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".woff", ".woff2", ".ttf":
		return true
	}
	return false
}

func serveBytes(c *gin.Context, name string, body []byte) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(body)))
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(body))
}

func (p *PortalUI) Asset(c *gin.Context) {
	id, version := c.Param("theme"), c.Param("version")
	m, ok := p.view.Manifest(id)
	name := strings.TrimPrefix(c.Param("file"), "/")
	legacy := id == "business" && version == "v2"
	if !ok || (m.Version != version && !legacy) || !safeAsset(name) {
		c.Status(http.StatusNotFound)
		return
	}
	folder := "static"
	if legacy {
		folder = "static-v2"
	}
	body, err := fs.ReadFile(p.resources, "themes/"+id+"/"+folder+"/"+name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	serveBytes(c, name, body)
}

func (p *PortalUI) Admin(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("file"), "/")
	if name == "" || name == "index.html" {
		p.adminIndex(c)
		return
	}
	if !fs.ValidPath(name) {
		c.Status(http.StatusNotFound)
		return
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			c.Status(http.StatusNotFound)
			return
		}
	}
	if safeAsset(name) {
		body, err := fs.ReadFile(p.admin, name)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		serveBytes(c, name, body)
		return
	}
	// Only extensionless client-side navigation falls back to the SPA index.
	if path.Ext(name) != "" || name == "api" || strings.HasPrefix(name, "api/") || strings.HasPrefix(name, "static/") || strings.HasPrefix(name, "assets/") {
		c.Status(http.StatusNotFound)
		return
	}
	p.adminIndex(c)
}

func (p *PortalUI) adminIndex(c *gin.Context) {
	body, err := fs.ReadFile(p.admin, "index.html")
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	serveBytes(c, "index.html", body)
}
