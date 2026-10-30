package portaltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPortalSiteRelease(t *testing.T) {
	c := cmsSetup(t)
	initial := cmsData(c.ok("GET", "/cms/site", nil))
	original := initial["publishedReleaseId"]
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 80, 40)))
	imageResult := c.upload("site-test.png", buf.Bytes())
	cmsCode(t, imageResult, 200)
	asset := cmsID(cmsData(imageResult), "assetId")
	slug := "site-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	pageID := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": slug, "title": "外观测试内部链接", "bodyHtml": "<p>测试单页</p>"})), "contentId")
	pagePath := fmt.Sprintf("/cms/page/%d", pageID)
	pageRow := cmsData(c.ok("GET", pagePath, nil))
	c.ok("POST", pagePath+"/publish", map[string]any{"version": pageRow["version"]})
	keep := false
	t.Cleanup(func() {
		if keep {
			return
		}
		current := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": current["version"], "releaseId": original})
		p := cmsData(c.ok("GET", pagePath, nil))
		c.ok("DELETE", pagePath, map[string]any{"version": p["version"]})
	})
	cfg := initial["config"].(map[string]any)
	cfg["theme"] = "editorial"
	cfg["siteName"] = "外观回归测试站"
	cfg["logoAssetId"] = asset
	cfg["navigation"] = []any{map[string]any{"label": "内部导航验收", "contentId": pageID, "url": ""}}
	cfg["banners"] = []any{map[string]any{"title": "轮播验收", "assetId": asset, "contentId": pageID, "enabled": true}}
	c.ok("PUT", "/cms/site", map[string]any{"version": initial["version"], "config": cfg})
	cmsCode(t, c.call("PUT", "/cms/site", c.admin, map[string]any{"version": initial["version"], "config": cfg}), 500)
	_, body, _ := c.public("/", "")
	if strings.Contains(body, "外观回归测试站") {
		t.Fatal("site draft reached public page")
	}
	if status, _, _ := c.public(fmt.Sprintf("/media/%d", asset), ""); status != 404 {
		t.Fatal("site draft logo exposed")
	}
	preview := c.ok("GET", "/cms/site/preview", nil)
	html := cmsData(preview)["html"].(string)
	if !strings.Contains(html, "editorial") || !strings.Contains(html, "外观回归测试站") {
		t.Fatal("draft preview wrong snapshot")
	}
	cmsCode(t, c.call("GET", "/cms/site/preview", "", nil), 401)
	current := cmsData(c.ok("GET", "/cms/site", nil))
	payload := map[string]any{"version": current["version"]}
	var wg sync.WaitGroup
	results := make(chan float64, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", "http://127.0.0.1:8085/cms/site/publish", bytes.NewReader(b))
			req.Header.Set("Authorization", "Bearer "+c.admin)
			req.Header.Set("Content-Type", "application/json")
			resp, e := c.http.Do(req)
			if e != nil {
				results <- -1
				return
			}
			defer resp.Body.Close()
			out := map[string]any{}
			if json.NewDecoder(resp.Body).Decode(&out) != nil {
				results <- -2
				return
			}
			v, ok := out["code"].(float64)
			if !ok {
				results <- -3
				return
			}
			results <- v
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for code := range results {
		if code == 200 {
			wins++
		} else if code != 500 {
			t.Fatal("concurrent publication failed", code)
		}
	}
	if wins != 1 {
		t.Fatal("CAS publication commits", wins)
	}
	_, body, _ = c.public("/", "")
	if !strings.Contains(body, "/editorial/v1/") || !strings.Contains(body, "内部导航验收") || !strings.Contains(body, "轮播验收") {
		t.Fatal("published site snapshot incomplete")
	}
	if os.Getenv("PORTAL_SITE_GUI") == "seed" {
		record := map[string]any{"originalRelease": original, "pageId": pageID, "assetId": asset}
		b, _ := json.Marshal(record)
		path := "../../logs/portal-step56/gui-fixture.local.json"
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("GUI fixture already exists")
		}
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		keep = true
		return
	}
	if status, _, _ := c.public(fmt.Sprintf("/media/%d", asset), ""); status != 200 {
		t.Fatal("published logo unavailable")
	}
	cmsCode(t, c.call("DELETE", fmt.Sprintf("/cms/asset/%d", asset), c.admin, map[string]any{"version": 1}), 500)
	published := cmsData(c.ok("GET", "/cms/site", nil))["publishedReleaseId"]
	pageRow = cmsData(c.ok("GET", pagePath, nil))
	c.ok("POST", pagePath+"/unpublish", map[string]any{"version": pageRow["version"]})
	_, body, _ = c.public("/", "")
	if strings.Contains(body, "内部导航验收") || strings.Contains(body, "轮播验收") {
		t.Fatal("down content still linked by appearance")
	}
	current = cmsData(c.ok("GET", "/cms/site", nil))
	cmsCode(t, c.call("POST", "/cms/site/publish", c.admin, map[string]any{"version": current["version"], "releaseId": published}), 500)
	if cmsData(c.ok("GET", "/cms/site", nil))["publishedReleaseId"] != published {
		t.Fatal("failed restore changed release")
	}
	cfg["theme"] = "../../outside"
	cmsCode(t, c.call("PUT", "/cms/site", c.admin, map[string]any{"version": current["version"], "config": cfg}), 500)
	t.Log("PASS appearance draft isolation, preview auth, immutable release CAS, logo refs, stale navigation hiding and restore refusal")
}
func TestPortalSiteGUICleanup(t *testing.T) {
	if os.Getenv("PORTAL_SITE_GUI") != "cleanup" {
		t.Skip("explicit owned site fixture cleanup")
	}
	c := cmsSetup(t)
	b, e := os.ReadFile("../../logs/portal-step56/gui-fixture.local.json")
	if e != nil {
		t.Fatal(e)
	}
	v := map[string]any{}
	if json.Unmarshal(b, &v) != nil {
		t.Fatal("invalid fixture record")
	}
	row := cmsData(c.ok("GET", "/cms/site", nil))
	c.ok("POST", "/cms/site/publish", map[string]any{"version": row["version"], "releaseId": v["originalRelease"]})
	p := fmt.Sprintf("/cms/page/%d", cmsID(v, "pageId"))
	d := cmsData(c.ok("GET", p, nil))
	c.ok("DELETE", p, map[string]any{"version": d["version"]})
}
