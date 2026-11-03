package service

import (
	"context"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestRedirectPathBoundary(t *testing.T) {
	for _, s := range []string{"/old-news/123.html", "/news/retired-slug", "/products/old-model", "/p/old-about", "/a_b/v1.2", " /old-home "} {
		if _, e := CMSRedirectSource(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"/", "https://example.com/a", "//example.com", "/a?x=1", "/a#x", "/a%2fb", "/a\\b", "/a/../b", "/a/./b", "/a//b", "/a/", "/Upper", "/中文", "/admin", "/admin/login", "/cms/redirect", "/system/user/list", "/assets/portal/x", "/media/1", "/health", "/inquiry/thanks", "/robots.txt", "/sitemap.xml", "/news", "/news/c/old", "/products/c/old", "/search", "/" + strings.Repeat("a", 200)} {
		if _, e := CMSRedirectSource(s); e == nil {
			t.Fatal("unsafe/reserved path accepted", s)
		}
	}
	for _, in := range []model.CMSRedirectStatus{{Version: 1, Status: "2"}, {Version: 0, Status: "0"}} {
		if e := CMSSetRedirectStatus(context.Background(), 1, in, "test"); e == nil {
			t.Fatal("invalid status accepted")
		}
	}
}
