package handler

import (
	"encoding/json"
	"ruoyi-go/pkg/portalview"
	"strings"
	"testing"
)

func TestSEOUsesOnlyEscapedStructuredData(t *testing.T) {
	d := portalview.PageData{Title: "</script><script>alert(1)</script>", Description: "测试", Canonical: "https://example.com/news/one", SiteName: "站点", ShareType: "article"}
	decorateSEO(&d)
	if strings.Contains(string(d.Structured), "</script>") {
		t.Fatal("script terminator entered JSON-LD")
	}
	var v map[string]any
	if e := json.Unmarshal([]byte(d.Structured), &v); e != nil || v["@type"] != "Article" {
		t.Fatal("invalid schema", e)
	}
	d.NoIndex = true
	d.Structured = ""
	decorateSEO(&d)
	if d.Structured != "" {
		t.Fatal("private/error page structured metadata")
	}
}
