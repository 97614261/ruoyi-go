package portaltest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalFirstEdition(t *testing.T) {
	c := cmsSetup(t)
	key := "fe" + strconv.FormatInt(time.Now().UnixNano(), 36)
	initial := cmsData(c.ok("GET", "/cms/site", nil))
	oldDraft := initial["config"]
	raw, _ := json.Marshal(oldDraft)
	cfg := map[string]any{}
	json.Unmarshal(raw, &cfg)
	var channels, articles, pages []int64
	t.Cleanup(func() {
		state := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": state["version"], "releaseId": initial["publishedReleaseId"]})
		state = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": state["version"], "config": oldDraft})
		for _, id := range articles {
			path := fmt.Sprintf("/cms/article/%d", id)
			d := cmsData(c.ok("GET", path, nil))
			c.ok("DELETE", path, map[string]any{"version": d["version"]})
		}
		for _, id := range pages {
			path := fmt.Sprintf("/cms/page/%d", id)
			d := c.call("GET", path, c.admin, nil)
			if d["code"] == float64(200) {
				c.ok("DELETE", path, map[string]any{"version": cmsData(d)["version"]})
			}
		}
		for _, id := range channels {
			d := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", id), nil))
			d["status"] = "1"
			c.ok("PUT", "/cms/channel", d)
		}
	})
	for i := 0; i < 2; i++ {
		slug := fmt.Sprintf("%s-%d", key, i)
		ch := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"kind": "ARTICLE", "name": slug, "slug": slug, "status": "0"})), "channelId")
		channels = append(channels, ch)
		id := cmsID(cmsData(c.ok("POST", "/cms/article", map[string]any{"title": slug, "slug": slug, "channelId": ch, "bodyHtml": "<p>独立栏目文章</p>"})), "contentId")
		articles = append(articles, id)
		d := cmsData(c.ok("GET", fmt.Sprintf("/cms/article/%d", id), nil))
		c.ok("POST", fmt.Sprintf("/cms/article/%d/publish", id), map[string]any{"version": d["version"]})
	}
	pageID := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"title": key + "_%literal", "slug": key + "-page", "bodyHtml": "<p>不返回的私有正文</p>"})), "contentId")
	pages = append(pages, pageID)
	articleReader := cmsNewFeatureActor(c, key+"a", "cms:site:query", "cms:article:list")
	noSource := cmsNewFeatureActor(c, key+"n", "cms:site:query")
	noSite := cmsNewFeatureActor(c, key+"s", "cms:article:list")
	cmsCode(t, c.call("GET", "/cms/site/references?type=content", "", nil), 401)
	cmsCode(t, c.call("GET", "/cms/site/references?type=content", noSource.token, nil), 403)
	cmsCode(t, c.call("GET", "/cms/site/references?type=content", noSite.token, nil), 403)
	cmsCode(t, c.call("GET", "/cms/site/references?type=content&kind=PAGE", articleReader.token, nil), 403)
	cmsCode(t, c.call("GET", "/cms/site/references?type=channel", articleReader.token, nil), 403)
	result := c.call("GET", "/cms/site/references?type=content&name="+key+"&pageSize=999", articleReader.token, nil)
	cmsCode(t, result, 200)
	if len(result["rows"].([]any)) != 2 {
		t.Fatal("source permission filter wrong")
	}
	for _, v := range result["rows"].([]any) {
		row := v.(map[string]any)
		if row["kind"] != "ARTICLE" || row["bodyHtml"] != nil {
			t.Fatal("reference endpoint leaked other type/body")
		}
	}
	result = c.ok("GET", "/cms/site/references?type=content&name="+url.QueryEscape(key+"_%literal"), nil)
	if len(result["rows"].([]any)) != 1 {
		t.Fatal("literal wildcard prefix mishandled")
	}
	for _, ids := range []string{"0", "1,1", "-1", "1%20OR%201=1"} {
		r := c.call("GET", "/cms/site/references?type=content&ids="+ids, c.admin, nil)
		if r["code"] == float64(200) {
			t.Fatal("bad ids accepted")
		}
	}
	cfg["navigation"] = []any{}
	cfg["banners"] = []any{}
	cfg["blocks"] = []any{map[string]any{"kind": "ARTICLE", "title": "FIRST_SECTION", "channelId": channels[0], "count": 1, "enabled": true}, map[string]any{"kind": "ARTICLE", "title": "SECOND_SECTION", "channelId": channels[1], "count": 1, "enabled": true}}
	for _, theme := range []string{"business", "editorial", "industrial"} {
		cfg["theme"] = theme
		state := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": state["version"], "config": cfg})
		state = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": state["version"]})
		status, home, _ := c.public("/", "")
		first, second := strings.Index(home, "FIRST_SECTION"), strings.Index(home, "SECOND_SECTION")
		if status != 200 || first < 0 || second <= first {
			t.Fatal("repeated sections/order failed", theme)
		}
		a, b := fmt.Sprintf("%s-0", key), fmt.Sprintf("%s-1", key)
		if !strings.Contains(home[first:second], a) || strings.Contains(home[first:second], b) || !strings.Contains(home[second:], b) || strings.Contains(home[second:], a) {
			t.Fatal("repeated sections mixed data", theme)
		}
		if !strings.Contains(home, `href="/news/c/`+a+`"`) || !strings.Contains(home, `href="/news/c/`+b+`"`) {
			t.Fatal("more links lost section scope", theme)
		}
	}
	t.Log("PASS reference permissions/metadata/prefix/IDs; repeated sections with separate data and links in all three themes")
}
