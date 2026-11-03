package portaltest

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalWidgetAssetBoundary(t *testing.T) {
	c := cmsSetup(t)
	initial := cmsData(c.ok("GET", "/cms/site", nil))
	prefix := "widget-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	id := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": prefix, "title": prefix, "bodyHtml": "<p>组件链接目标</p>"})), "contentId")
	p := fmt.Sprintf("/cms/page/%d", id)
	d := cmsData(c.ok("GET", p, nil))
	c.ok("POST", p+"/publish", map[string]any{"version": d["version"]})
	t.Cleanup(func() {
		v := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": v["version"], "releaseId": initial["publishedReleaseId"]})
		v = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": v["version"], "config": initial["config"]})
		d := cmsData(c.ok("GET", p, nil))
		c.ok("DELETE", p, map[string]any{"version": d["version"]})
	})
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 32, 24)))
	asset := cmsID(cmsData(c.upload(prefix+".png", buf.Bytes())), "assetId")
	media := fmt.Sprintf("/media/%d", asset)
	cfg := map[string]any{}
	for k, v := range initial["config"].(map[string]any) {
		cfg[k] = v
	}
	blocks := []map[string]any{}
	for _, kind := range []string{"SERVICE", "ADVANTAGE", "CASE", "PARTNER"} {
		blocks = append(blocks, map[string]any{"kind": kind, "title": kind + prefix, "enabled": true, "layout": "grid", "items": []any{map[string]any{"title": kind, "text": "组件说明", "assetId": asset, "contentId": id}}})
	}
	cfg["blocks"] = blocks
	save := func() {
		v := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": v["version"], "config": cfg})
	}
	publish := func() {
		v := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": v["version"]})
	}
	save()
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("draft widget image exposed")
	}
	if !strings.Contains(cmsData(c.ok("GET", "/cms/site/preview", nil))["html"].(string), prefix) {
		t.Fatal("widget preview missing")
	}
	publish()
	if status, _, _ := c.public(media, ""); status != 200 {
		t.Fatal("published widget image unavailable")
	}
	_, home, _ := c.public("/", "")
	for _, kind := range []string{"SERVICE", "ADVANTAGE", "CASE", "PARTNER"} {
		if !strings.Contains(home, kind+prefix) {
			t.Fatal("widget kind missing", kind)
		}
	}
	for _, b := range blocks {
		b["enabled"] = false
	}
	save()
	publish()
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("disabled widget image public")
	}
	for _, b := range blocks {
		b["enabled"] = true
	}
	save()
	publish()
	d = cmsData(c.ok("GET", p, nil))
	c.ok("POST", p+"/unpublish", map[string]any{"version": d["version"]})
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("invalid-linked widget image public")
	}
	report := cmsData(c.ok("GET", "/cms/site/inspect", nil))
	found := false
	for _, raw := range report["issues"].([]any) {
		if raw.(map[string]any)["code"] == "SITE_CONTENT_UNAVAILABLE" {
			found = true
		}
	}
	if !found {
		t.Fatal("inspection omitted widget links")
	}
	t.Log("PASS four typed image widgets: private draft, enabled release, disabled/invalid-target revocation and inspection")
}
