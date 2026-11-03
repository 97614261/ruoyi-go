package portaltest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalThemeExpansion(t *testing.T) {
	c := cmsSetup(t)
	initial := cmsData(c.ok("GET", "/cms/site", nil))
	original := initial["publishedReleaseId"]
	oldDraft := initial["config"]
	defer func() {
		current := cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("POST", "/cms/site/publish", map[string]any{"version": current["version"], "releaseId": original})
		current = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": current["version"], "config": oldDraft})
	}()
	slug := "theme-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	channel := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": slug, "slug": slug, "kind": "ARTICLE", "status": "0"})), "channelId")
	id := cmsID(cmsData(c.ok("POST", "/cms/article", map[string]any{"title": "行业模板技术内容", "slug": slug, "channelId": channel, "bodyHtml": "<p>保持同一份公开内容</p>"})), "contentId")
	t.Cleanup(func() {
		d := cmsData(c.ok("GET", fmt.Sprintf("/cms/article/%d", id), nil))
		c.ok("DELETE", fmt.Sprintf("/cms/article/%d", id), map[string]any{"version": d["version"]})
		ch := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", channel), nil))
		ch["status"] = "1"
		c.ok("PUT", "/cms/channel", ch)
	})
	d := cmsData(c.ok("GET", fmt.Sprintf("/cms/article/%d", id), nil))
	c.ok("POST", fmt.Sprintf("/cms/article/%d/publish", id), map[string]any{"version": d["version"]})
	themes := c.ok("GET", "/cms/site/themes?pageNum=2&pageSize=1", nil)
	if themes["total"] != float64(3) || len(themes["rows"].([]any)) != 1 {
		t.Fatal("catalog pagination failed")
	}
	cmsCode(t, c.call("GET", "/cms/site/themes", "", nil), 401)
	cfg := initial["config"].(map[string]any)
	cfgCopy := map[string]any{}
	for k, v := range cfg {
		cfgCopy[k] = v
	}
	cfgCopy["theme"] = "industrial"
	cfgCopy["listingLayout"] = "grid"
	cfgCopy["channelLayouts"] = []any{map[string]any{"channelId": channel, "layout": "list"}}
	c.ok("PUT", "/cms/site", map[string]any{"version": initial["version"], "config": cfgCopy})
	state := cmsData(c.ok("GET", "/cms/site", nil))
	req := map[string]any{"version": state["version"], "theme": "editorial", "channelId": channel, "layout": "grid"}
	compare := cmsData(c.ok("POST", "/cms/site/compare", req))
	if compare["fromTheme"] != "industrial" || !strings.Contains(compare["beforeHtml"].(string), "listing-list") || !strings.Contains(compare["afterHtml"].(string), "listing-grid") || !strings.Contains(compare["beforeHtml"].(string), "行业模板技术内容") || !strings.Contains(compare["afterHtml"].(string), "行业模板技术内容") {
		t.Fatal("comparison did not use shared public content or layout")
	}
	if cmsData(c.ok("GET", "/cms/site", nil))["version"] != state["version"] {
		t.Fatal("comparison mutated site")
	}
	req["version"] = initial["version"]
	cmsCode(t, c.call("POST", "/cms/site/compare", c.admin, req), 500)
	req["version"] = state["version"]
	req["theme"] = "../outside"
	cmsCode(t, c.call("POST", "/cms/site/compare", c.admin, req), 500)
	// Before publishing, the industrial draft must not replace public appearance.
	_, body, _ := c.public("/", "")
	if strings.Contains(body, "industrial-intro") {
		t.Fatal("draft theme leaked online")
	}
	c.ok("POST", "/cms/site/publish", map[string]any{"version": state["version"]})
	_, body, _ = c.public("/", "")
	if !strings.Contains(body, "industrial-intro") || strings.Contains(body, `<body class="listing-`) {
		t.Fatal("industrial homepage or layout isolation failed")
	}
	status, body, _ := c.public("/news/c/"+slug, "")
	if status != 200 || !strings.Contains(body, `class="listing-list `) {
		t.Fatal("channel-specific layout not applied")
	}
	_, body, _ = c.public("/news", "")
	if !strings.Contains(body, `class="listing-grid `) {
		t.Fatal("global listing layout not applied")
	}
	for _, path := range []string{"/assets/portal/industrial/v1/site.css", "/assets/portal/industrial/v1/listing-v1.css", "/assets/portal/business/v3/site.css", "/assets/portal/business/v2/site.css", "/assets/portal/editorial/v1/site.css"} {
		if status, _, _ := c.public(path, ""); status != 200 {
			t.Fatal("new or historical asset unavailable", path, status)
		}
	}
	current := cmsData(c.ok("GET", "/cms/site", nil))
	bad := map[string]any{}
	for k, v := range cfgCopy {
		bad[k] = v
	}
	bad["channelLayouts"] = []any{map[string]any{"channelId": channel, "layout": "list"}, map[string]any{"channelId": channel, "layout": "grid"}}
	cmsCode(t, c.call("PUT", "/cms/site", c.admin, map[string]any{"version": current["version"], "config": bad}), 500)
	bad["channelLayouts"] = []any{map[string]any{"channelId": 9223372036854775806, "layout": "list"}}
	cmsCode(t, c.call("PUT", "/cms/site", c.admin, map[string]any{"version": current["version"], "config": bad}), 500)
	roleKey := "themes_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var rid, uid int64
	role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6051}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	if e := c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = rid
	name := "th" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		if uid > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": name, "nickName": "模板测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "只读模板测试", "password": "Theme_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e := c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	token := c.login(name, "Theme_test_975!")
	defer c.call("POST", "/logout", token, nil)
	cmsCode(t, c.call("GET", "/cms/site/themes", token, nil), 200)
	current = cmsData(c.ok("GET", "/cms/site", nil))
	request := map[string]any{"version": current["version"], "theme": "business", "channelId": 0}
	cmsCode(t, c.call("POST", "/cms/site/compare", token, request), 200)
	request["channelId"] = channel
	cmsCode(t, c.call("POST", "/cms/site/compare", token, request), 403)
	cmsCode(t, c.call("POST", "/cms/site/publish", token, map[string]any{"version": current["version"]}), 403)
	// Disabled channel references cannot be promoted to a new appearance release.
	ch := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", channel), nil))
	ch["status"] = "1"
	c.ok("PUT", "/cms/channel", ch)
	cmsCode(t, c.call("POST", "/cms/site/publish", c.admin, map[string]any{"version": current["version"]}), 500)
	t.Log("PASS registered industry theme, catalog paging, safe read-only comparison, global/channel layouts, legacy CSS, draft isolation, permission gates and disabled-channel publish rejection")
}
