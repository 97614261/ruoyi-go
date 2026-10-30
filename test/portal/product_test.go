package portaltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalProductAndPublicPages(t *testing.T) {
	c := cmsSetup(t)
	slug := "product-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	img := c.upload(slug+".png", buf.Bytes())
	cmsCode(t, img, 200)
	assetID := cmsID(cmsData(img), "assetId")
	channelID := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": slug, "slug": slug, "kind": "PRODUCT", "status": "0", "orderNum": 0})), "channelId")
	cp := fmt.Sprintf("/cms/channel/%d", channelID)
	draft := map[string]any{"slug": slug, "title": slug, "channelId": channelID, "bodyHtml": "<p>产品正文验收</p>", "productCode": "SKU-2026", "productParams": "材质：钢\n重量：20kg", "galleryIds": []int64{assetID}}
	id := cmsID(cmsData(c.ok("POST", "/cms/product", draft)), "contentId")
	path := fmt.Sprintf("/cms/product/%d", id)
	retain := false
	t.Cleanup(func() {
		if retain {
			return
		}
		r := cmsData(c.ok("GET", path, nil))
		c.ok("DELETE", path, map[string]any{"version": r["version"]})
		ch := cmsData(c.ok("GET", cp, nil))
		ch["status"] = "1"
		c.ok("PUT", "/cms/channel", ch)
	})
	pub := "/products/" + slug
	media := fmt.Sprintf("/media/%d", assetID)
	if status, _, _ := c.public(pub, ""); status != 404 {
		t.Fatal("draft product leaked")
	}
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("draft gallery leaked")
	}
	get := func() map[string]any { return cmsData(c.ok("GET", path, nil)) }
	r := get()
	first := r["draft"].(map[string]any)["revisionId"]
	c.ok("POST", path+"/publish", map[string]any{"version": r["version"]})
	for _, url := range []string{pub, "/", "/products", "/products/c/" + slug, "/search?q=" + slug, "/sitemap/1.xml"} {
		status, b, _ := c.public(url, "")
		if status != 200 || !strings.Contains(b, slug) {
			t.Fatalf("published entry %s status %d", url, status)
		}
	}
	status, b, _ := c.public(pub, "")
	if status != 200 || !strings.Contains(b, "SKU-2026") || !strings.Contains(b, "20kg") || !strings.Contains(b, media) {
		t.Fatal("product revision fields missing")
	}
	if status, _, _ := c.public(media, ""); status != 200 {
		t.Fatal("published gallery missing")
	}
	if os.Getenv("PORTAL_PRODUCT_GUI") == "seed" {
		b, _ := json.Marshal(map[string]any{"id": id, "channelId": channelID, "slug": slug})
		file := "../../logs/portal-step4/gui-fixture.local.json"
		if _, e := os.Stat(file); !os.IsNotExist(e) {
			t.Fatal("GUI fixture record exists")
		}
		if e := os.WriteFile(file, b, 0600); e != nil {
			t.Fatal(e)
		}
		retain = true
		return
	}
	r = get()
	draft["contentId"] = id
	draft["version"] = r["version"]
	draft["productCode"] = "PRIVATE-NEW-SKU"
	draft["title"] = slug + "-draft"
	draft["galleryIds"] = []int64{}
	c.ok("PUT", "/cms/product", draft)
	for _, url := range []string{pub, "/products", "/search?q=" + slug} {
		_, b, _ := c.public(url, "")
		if strings.Contains(b, "PRIVATE-NEW-SKU") || strings.Contains(b, slug+"-draft") {
			t.Fatal("draft data reached public entry", url)
		}
	}
	r = get()
	c.ok("POST", path+"/restore", map[string]any{"version": r["version"], "revisionId": first})
	r = get()
	c.ok("POST", path+"/publish", map[string]any{"version": r["version"]})
	if status, _, _ := c.public(media, ""); status != 200 {
		t.Fatal("restore lost gallery reference")
	}
	cmsCode(t, c.call("DELETE", fmt.Sprintf("/cms/asset/%d", assetID), c.admin, map[string]any{"version": 1}), 500)
	cmsCode(t, c.call("GET", path, "", nil), 401)
	ch := cmsData(c.ok("GET", cp, nil))
	ch["status"] = "1"
	c.ok("PUT", "/cms/channel", ch)
	for _, url := range []string{pub, "/products/c/" + slug, media} {
		if status, _, _ := c.public(url, ""); status != 404 {
			t.Fatal("disabled product channel leaked", url)
		}
	}
	for _, url := range []string{"/", "/products", "/search?q=" + slug, "/sitemap/1.xml"} {
		status, b, _ := c.public(url, "")
		if status != 200 || strings.Contains(b, pub) {
			t.Fatal("disabled product remained in public aggregate", url)
		}
	}
	for _, url := range []string{"/search?page=-1", "/search?page=1001", "/search?q=" + strings.Repeat("a", 81)} {
		if status, _, _ := c.public(url, ""); status != 400 {
			t.Fatal("unbounded public input accepted", url)
		}
	}
	_, body, _ := c.public("/search?q=%3Cscript%3Ealert(1)%3C/script%3E", "")
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatal("search injection")
	}
	_, body, _ = c.public("/robots.txt", "")
	if !strings.Contains(body, "Disallow: /admin/") {
		t.Fatal("robots missing")
	}
	ch = cmsData(c.ok("GET", cp, nil))
	ch["status"] = "0"
	c.ok("PUT", "/cms/channel", ch)
	r = get()
	c.ok("POST", path+"/unpublish", map[string]any{"version": r["version"]})
	if status, _, _ := c.public(pub, ""); status != 404 {
		t.Fatal("unpublished product reachable")
	}
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("unpublished gallery reachable")
	}
	t.Log("PASS product/gallery snapshot, publication, restore, public aggregates, channel gates, bounds and XSS")
}

func TestPortalProductGUICleanup(t *testing.T) {
	if os.Getenv("PORTAL_PRODUCT_GUI") != "cleanup" {
		t.Skip("explicit own GUI fixture cleanup")
	}
	c := cmsSetup(t)
	b, e := os.ReadFile("../../logs/portal-step4/gui-fixture.local.json")
	if e != nil {
		t.Fatal(e)
	}
	v := map[string]any{}
	if json.Unmarshal(b, &v) != nil {
		t.Fatal("invalid fixture record")
	}
	path := fmt.Sprintf("/cms/product/%d", cmsID(v, "id"))
	d := cmsData(c.ok("GET", path, nil))
	if d["slug"] != v["slug"] {
		t.Fatal("fixture identity mismatch")
	}
	c.ok("DELETE", path, map[string]any{"version": d["version"]})
	p := fmt.Sprintf("/cms/channel/%d", cmsID(v, "channelId"))
	ch := cmsData(c.ok("GET", p, nil))
	ch["status"] = "1"
	c.ok("PUT", "/cms/channel", ch)
}
