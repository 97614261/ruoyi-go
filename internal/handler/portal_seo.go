package handler

import (
	"encoding/json"
	"html/template"
	"ruoyi-go/pkg/portalview"
	"strconv"
)

// Only marshal fixed schema properties, never raw script or administrator-supplied JSON.
func decorateSEO(d *portalview.PageData) {
	if d.ShareTitle == "" {
		d.ShareTitle = d.Title
	}
	if d.ShareDescription == "" {
		d.ShareDescription = d.Description
	}
	if d.ShareType == "" {
		d.ShareType = "website"
	}
	if d.NoIndex || d.Canonical == "" {
		return
	}
	schema := map[string]any{"@context": "https://schema.org", "@type": "WebPage", "name": d.Title, "description": d.Description, "url": d.Canonical, "isPartOf": map[string]any{"@type": "WebSite", "name": d.SiteName}}
	if d.ShareType == "article" {
		schema["@type"] = "Article"
		schema["headline"] = d.Title
	}
	if d.ShareImage != "" {
		schema["image"] = d.ShareImage
	}
	b, e := json.Marshal(schema)
	if e == nil {
		d.Structured = template.JS(b)
	}
}
func shareMedia(origin string, id int64) string {
	if id > 0 {
		return origin + "/media/" + strconv.FormatInt(id, 10)
	}
	return ""
}
