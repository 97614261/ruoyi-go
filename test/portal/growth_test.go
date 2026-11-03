package portaltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPortalGrowth(t *testing.T) {
	c := cmsSetup(t)
	prefix := "growth-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	initial := cmsData(c.ok("GET", "/cms/site", nil))
	oldRelease := initial["publishedReleaseId"]
	oldDraft := initial["config"]
	var pages []struct {
		kind string
		id   int64
	}
	var channel, tagID int64
	// Restore the original public appearance AND independently saved draft before retiring fixtures.
	t.Cleanup(func() {
		cur := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": cur["version"], "releaseId": oldRelease})
		cur = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": cur["version"], "config": oldDraft})
		for _, p := range pages {
			path := fmt.Sprintf("/cms/%s/%d", p.kind, p.id)
			d := c.call("GET", path, c.admin, nil)
			if d["code"] == float64(200) {
				c.ok("DELETE", path, map[string]any{"version": cmsData(d)["version"]})
			}
		}
		if channel > 0 {
			d := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", channel), nil))
			d["status"] = "1"
			c.ok("PUT", "/cms/channel", d)
		}
		if tagID > 0 {
			d := cmsData(c.ok("GET", fmt.Sprintf("/cms/tag/%d", tagID), nil))
			d["status"] = "1"
			c.ok("PUT", "/cms/tag", d)
		}
	})
	makeContent := func(kind string, body map[string]any) int64 {
		id := cmsID(cmsData(c.ok("POST", "/cms/"+kind, body)), "contentId")
		pages = append(pages, struct {
			kind string
			id   int64
		}{kind, id})
		return id
	}
	action := func(kind string, id int64, op string) {
		p := fmt.Sprintf("/cms/%s/%d", kind, id)
		d := cmsData(c.ok("GET", p, nil))
		c.ok("POST", p+"/"+op, map[string]any{"version": d["version"]})
	}
	target := makeContent("page", map[string]any{"slug": prefix + "-target", "title": "关联单页不应允许", "bodyHtml": "<p>单页</p>"})
	channel = cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": prefix, "slug": prefix, "kind": "ARTICLE", "status": "0"})), "channelId")
	a := makeContent("article", map[string]any{"slug": prefix, "title": "运营快照文章", "summary": "原摘要", "channelId": channel, "bodyHtml": "<p>公开正文</p>"})
	related := makeContent("article", map[string]any{"slug": prefix + "-related", "title": "关联公开文章", "channelId": channel, "bodyHtml": "<p>关联正文</p>"})
	action("article", related, "publish")
	action("article", a, "publish")
	p := fmt.Sprintf("/cms/article/%d", a)
	c.ok("POST", "/cms/tag", map[string]any{"name": prefix, "status": "0", "createBy": "forged"})
	tags := c.ok("GET", "/cms/tag/list?name="+prefix+"&pageSize=999", nil)
	if len(tags["rows"].([]any)) != 1 {
		t.Fatal("tag missing")
	}
	tag := tags["rows"].([]any)[0].(map[string]any)
	tagID = cmsID(tag, "tagId")
	if tag["createBy"] == "forged" {
		t.Fatal("tag audit forged")
	}
	var imageBuf bytes.Buffer
	png.Encode(&imageBuf, image.NewRGBA(image.Rect(0, 0, 40, 30)))
	asset := cmsData(c.upload(prefix+".png", imageBuf.Bytes()))
	assetID := cmsID(asset, "assetId")
	ops := cmsData(c.ok("GET", p+"/operations", nil))
	settings := map[string]any{"tagIds": []int64{tagID}, "relatedIds": []int64{related}, "featured": true, "pinOrder": 99, "shareTitle": "分享专用标题", "shareDescription": "分享专用摘要", "shareAssetId": assetID, "tags": []any{map[string]any{"tagId": tagID, "name": "伪造标签"}}}
	input := map[string]any{"version": ops["version"], "settings": settings}
	c.ok("PUT", p+"/operations", input)
	cmsCode(t, c.call("PUT", p+"/operations", c.admin, input), 409)
	if status, _, _ := c.public(fmt.Sprintf("/media/%d", assetID), ""); status != 404 {
		t.Fatal("unpublished share image exposed")
	}
	_, html, _ := c.public("/news/"+prefix, "")
	if strings.Contains(html, "分享专用标题") {
		t.Fatal("operations draft reached public")
	}
	fresh := cmsData(c.ok("GET", p+"/operations", nil))
	snapshot := fresh["settings"].(map[string]any)
	if snapshot["tags"].([]any)[0].(map[string]any)["name"] != prefix {
		t.Fatal("tag names were not server snapshots")
	}
	// Ordinary saves inherit the extension; the original request shape remains unchanged.
	d := cmsData(c.ok("GET", p, nil))
	draft := d["draft"].(map[string]any)
	draft["contentId"] = a
	draft["version"] = d["version"]
	draft["slug"] = prefix
	draft["summary"] = "普通保存的新摘要"
	c.ok("PUT", "/cms/article", draft)
	inherited := cmsData(c.ok("GET", p+"/operations", nil))
	if inherited["settings"].(map[string]any)["shareTitle"] != "分享专用标题" {
		t.Fatal("ordinary save lost operations")
	}
	action("article", a, "publish")
	_, html, _ = c.public("/news/"+prefix, "")
	for _, text := range []string{"property=\"og:title\" content=\"分享专用标题\"", "application/ld+json", prefix, "关联公开文章"} {
		if !strings.Contains(html, text) {
			t.Fatal("public metadata/related missing", text)
		}
	}
	if status, _, _ := c.public(fmt.Sprintf("/media/%d", assetID), ""); status != 200 {
		t.Fatal("published share image missing")
	}
	_, filtered, _ := c.public("/news?tag="+strconv.FormatInt(tagID, 10), "")
	if !strings.Contains(filtered, "运营快照文章") || strings.Contains(filtered, "关联公开文章") {
		t.Fatal("published tag filter wrong")
	}
	// Rename does not rewrite published snapshots.
	tag["name"] = prefix + "-renamed"
	c.ok("PUT", "/cms/tag", tag)
	_, html, _ = c.public("/news/"+prefix, "")
	if strings.Contains(html, prefix+"-renamed") {
		t.Fatal("dictionary edit rewrote published tag")
	}
	action("article", related, "unpublish")
	_, html, _ = c.public("/news/"+prefix, "")
	if strings.Contains(html, "关联公开文章") {
		t.Fatal("unpublished related leaked")
	}
	action("article", a, "unpublish")
	if status, _, _ := c.public(fmt.Sprintf("/media/%d", assetID), ""); status != 404 {
		t.Fatal("unpublished share image remains public")
	}
	action("article", a, "publish")
	// Invalid related target/self/duplicates are rejected without altering the working version.
	for _, ids := range [][]int64{{target}, {a}, {related, related}} {
		settings["relatedIds"] = ids
		v := cmsData(c.ok("GET", p+"/operations", nil))
		out := c.call("PUT", p+"/operations", c.admin, map[string]any{"version": v["version"], "settings": settings})
		if out["code"] == float64(200) {
			t.Fatal("invalid relation accepted")
		}
	}
	settings["relatedIds"] = []int64{related}
	// Only one writer with the same content version may create the next operations snapshot.
	v := cmsData(c.ok("GET", p+"/operations", nil))
	var wg sync.WaitGroup
	codes := make(chan float64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- c.call("PUT", p+"/operations", c.admin, map[string]any{"version": v["version"], "settings": settings})["code"].(float64)
		}()
	}
	wg.Wait()
	close(codes)
	wins := 0
	for code := range codes {
		if code == 200 {
			wins++
		} else if code != 409 {
			t.Fatal("unexpected CAS failure", code)
		}
	}
	if wins != 1 {
		t.Fatal("CAS allowed competing writers")
	}
	action("article", a, "publish")
	// All three templates render the same typed widgets, and statistics only start after publication.
	cfgBytes, _ := json.Marshal(oldDraft)
	cfg := map[string]any{}
	json.Unmarshal(cfgBytes, &cfg)
	cfg["blocks"] = []any{map[string]any{"kind": "ARTICLE", "count": 6, "enabled": true, "featuredOnly": true}, map[string]any{"kind": "FAQ", "title": "验收常见问题", "enabled": true, "layout": "list", "items": []any{map[string]any{"title": "如何联系", "text": "填写留言表单", "assetId": 0, "contentId": a}}}}
	cfg["analyticsEnabled"] = true
	cfg["analyticsKeywords"] = true
	cfg["analyticsRetention"] = 30
	var viewsBefore int64
	c.db.QueryRow("SELECT COALESCE(SUM(hits),0) FROM cms_visit_daily WHERE dimension='total'").Scan(&viewsBefore)
	for _, theme := range []string{"business", "editorial", "industrial"} {
		cfg["theme"] = theme
		current := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": current["version"], "config": cfg})
		preview := cmsData(c.ok("GET", "/cms/site/preview", nil))["html"].(string)
		if !strings.Contains(preview, "验收常见问题") {
			t.Fatal("widget preview missing", theme)
		}
		current = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": current["version"]})
		status, home, _ := c.public("/", "")
		if status != 200 || !strings.Contains(home, "验收常见问题") || !strings.Contains(home, "运营快照文章") {
			t.Fatal("typed homepage failed", theme, status)
		}
		if status, _, _ := c.public("/assets/portal/"+theme+"/"+map[string]string{"business": "v3", "editorial": "v1", "industrial": "v1"}[theme]+"/growth-v1.css", ""); status != 200 {
			t.Fatal("growth stylesheet unavailable", theme)
		}
	}
	req, _ := http.NewRequest("GET", portalTestURL()+"/search?q="+prefix, nil)
	req.Header.Set("Referer", "https://source.example/private?secret=never-store")
	res, e := c.http.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	bench := cmsData(c.ok("GET", "/cms/workbench", nil))
	if bench["analyticsEnabled"] != true || bench["analyticsVisible"] != true {
		t.Fatal("real analytics not shown in workbench")
	}
	trend := bench["visits"].([]any)
	if len(trend) == 0 || trend[len(trend)-1].(map[string]any)["hits"].(float64) < 1 {
		t.Fatal("workbench trend did not reuse total visit rows")
	}
	stats := c.ok("GET", "/cms/statistics/list?dimension=keyword&pageSize=100", nil)
	found := false
	for _, raw := range stats["rows"].([]any) {
		row := raw.(map[string]any)
		if row["value"] == prefix {
			found = true
		}
	}
	if !found {
		t.Fatal("opted-in keyword not recorded")
	}
	var rawLeak int
	c.db.QueryRow("SELECT COUNT(*) FROM cms_visit_daily WHERE dimension_value LIKE '%never-store%' OR dimension_value LIKE '%/private%'").Scan(&rawLeak)
	if rawLeak != 0 {
		t.Fatal("full referrer leaked")
	}
	var viewsAfter int64
	c.db.QueryRow("SELECT COALESCE(SUM(hits),0) FROM cms_visit_daily WHERE dimension='total'").Scan(&viewsAfter)
	if viewsAfter < viewsBefore+4 {
		t.Fatal("page views not persisted")
	}
	// HEAD, static assets, redirects/errors and authenticated previews must not count as public page views.
	before := viewsAfter
	c.ok("GET", "/cms/site/preview", nil)
	head, _ := http.NewRequest("HEAD", portalTestURL()+"/news/"+prefix, nil)
	res, e = c.http.Do(head)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	c.public("/missing-"+prefix, "")
	c.db.QueryRow("SELECT COALESCE(SUM(hits),0) FROM cms_visit_daily WHERE dimension='total'").Scan(&viewsAfter)
	if viewsAfter != before {
		t.Fatal("non-page-view recorded")
	}
	cmsCode(t, c.call("GET", "/cms/statistics/list", "", nil), 401)
	cmsCode(t, c.call("GET", "/cms/tag/list", "", nil), 401)
	// Recovering a deleted published item never republishes it or bypasses CAS.
	d = cmsData(c.ok("GET", p, nil))
	c.ok("DELETE", p, map[string]any{"version": d["version"]})
	trash := c.ok("GET", "/cms/article/trash?title=运营快照文章", nil)
	var deleted map[string]any
	for _, raw := range trash["rows"].([]any) {
		row := raw.(map[string]any)
		if cmsID(row, "contentId") == a {
			deleted = row
		}
	}
	if deleted == nil {
		t.Fatal("deleted content missing from trash")
	}
	cmsCode(t, c.call("POST", p+"/recover", c.admin, map[string]any{"version": d["version"]}), 409)
	c.ok("POST", p+"/recover", map[string]any{"version": deleted["version"]})
	d = cmsData(c.ok("GET", p, nil))
	if d["isPublished"] != float64(0) || d["publishedRevisionId"] != nil {
		t.Fatal("recycle restore republished content")
	}
	if status, _, _ := c.public("/news/"+prefix, ""); status != 404 {
		t.Fatal("restored draft visible")
	}
	t.Log("PASS tags/snapshots/CAS/ordinary saves/relations/private share image/SEO/three homepage themes/opt-in statistics/trash restore")
}
