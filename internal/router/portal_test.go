package router

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/portal"
)

func TestPortalRoutesKeepAPIAndStaticBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<html><script src="/admin/static/app.js"></script></html>`)}, "static/app.js": &fstest.MapFile{Data: []byte(`console.log('admin')`)}, "README.md": &fstest.MapFile{Data: []byte(`private build note`)}}
	ui, err := handler.NewPortalUI(portal.Resources, admin, "business", "Test & Portal")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/system/check", func(c *gin.Context) { c.String(200, "original-api") })
	registerPortal(r, ui)
	for _, tc := range []struct {
		url      string
		code     int
		contains string
	}{

		{"/admin", 301, ""},
		{"/admin/", 200, "/admin/static/app.js"},
		{"/admin/system/user", 200, "/admin/static/app.js"},
		{"/admin/static/app.js", 200, "console.log"},
		{"/admin/static/missing.js", 404, ""},
		{"/admin/README.md", 404, ""},
		{"/admin/.env", 404, ""},
		{"/admin/api/missing", 404, ""},
		{"/assets/portal/business/v2/site.css", 200, "color-scheme"},
		{"/assets/portal/business/v4/site.css", 404, ""},
		{"/assets/portal/unknown/v1/site.css", 404, ""},
		{"/assets/portal/business/v2/../manifest.json", 404, ""},
		{"/system/check", 200, "original-api"},
		{"/system/missing", 404, ""},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("got %d %q", w.Code, w.Body.String())
			}
			if tc.code == 404 && strings.Contains(w.Body.String(), "<html>") {
				t.Fatal("missing resource fell back to admin")
			}
		})
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("HEAD", "/assets/portal/business/v2/site.css", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("HEAD asset must not send body")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/portal/business/v2/site.css", nil))
	etag := w.Header().Get("ETag")
	req := httptest.NewRequest("GET", "/assets/portal/business/v2/site.css", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if etag == "" || w.Code != 304 {
		t.Fatal("asset cache validation failed")
	}
}

func TestPortalResourcesMustBeCompleteBeforeStartup(t *testing.T) {
	for _, admin := range []fs.FS{nil, fstest.MapFS{}, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("wrong base")}}} {
		if _, err := handler.NewPortalUI(portal.Resources, admin, "business", "x"); err == nil {
			t.Fatal("incomplete admin accepted")
		}
	}
}

func TestPortalDisabledKeepsExistingRouteSet(t *testing.T) {
	cfg, err := config.Load("../../configs/application.yml")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Upload.Path = t.TempDir()
	r := NewWithPortal(cfg, nil)
	for _, route := range r.Routes() {
		if route.Path == "/" || strings.HasPrefix(route.Path, "/admin") {
			t.Fatal("disabled portal registered UI")
		}
	}
	ui, err := handler.NewPortalUI(portal.Resources, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<script src="/admin/main.js"></script>`)}}, "business", "x")
	if err != nil {
		t.Fatal(err)
	}
	with := NewWithPortal(cfg, ui)
	seen := map[string]bool{}
	for _, route := range with.Routes() {
		seen[route.Method+" "+route.Path] = true
	}
	for _, route := range r.Routes() {
		if !seen[route.Method+" "+route.Path] {
			t.Fatalf("old route lost: %s", route.Path)
		}
	}
}

func TestBuiltAdminEntryAndReferencedAssets(t *testing.T) {
	admin, err := fs.Sub(portal.Resources, "admin-dist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(admin, "index.html"); err != nil {
		t.Skip("build admin-web first")
	}
	ui, err := handler.NewPortalUI(portal.Resources, admin, "business", "x")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerPortal(r, ui)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/admin/login", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `src="/admin/static/js/`) {
		t.Fatal("built admin base invalid")
	}
	for _, fragment := range strings.Split(w.Body.String(), `"/admin/`)[1:] {
		name := strings.SplitN(fragment, `"`, 2)[0]
		if !strings.HasPrefix(name, "static/") && name != "favicon.ico" {
			continue
		}
		resource := httptest.NewRecorder()
		r.ServeHTTP(resource, httptest.NewRequest("GET", "/admin/"+name, nil))
		if resource.Code != 200 || strings.Contains(resource.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("built resource not served: %s", name)
		}
	}
}
