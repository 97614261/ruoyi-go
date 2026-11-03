package portaltest

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestPortalRedirectFlow(t *testing.T) {
	c := cmsSetup(t)
	prefix := "redirect-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var redirects, pages, articles, channels []int64
	t.Cleanup(func() {
		for _, id := range redirects {
			path := fmt.Sprintf("/cms/redirect/%d", id)
			v := cmsData(c.ok("GET", path, nil))
			if v["status"] == "0" {
				c.ok("POST", path+"/status", map[string]any{"status": "1", "version": v["version"]})
			}
		}
		for _, item := range []struct {
			kind string
			ids  []int64
		}{{"page", pages}, {"article", articles}} {
			for _, id := range item.ids {
				path := fmt.Sprintf("/cms/%s/%d", item.kind, id)
				v := cmsData(c.ok("GET", path, nil))
				c.ok("DELETE", path, map[string]any{"version": v["version"]})
			}
		}
		for _, id := range channels {
			v := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", id), nil))
			v["status"] = "1"
			c.ok("PUT", "/cms/channel", v)
		}
	})
	page := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": prefix, "title": "旧路径目标", "bodyHtml": "<p>目标公开正文</p>"})), "contentId")
	pages = append(pages, page)
	ch := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": prefix, "slug": prefix, "kind": "ARTICLE", "status": "0"})), "channelId")
	channels = append(channels, ch)
	contentPath := fmt.Sprintf("/cms/page/%d", page)
	publish := func(kind string, id int64, action string) {
		path := fmt.Sprintf("/cms/%s/%d", kind, id)
		v := cmsData(c.ok("GET", path, nil))
		c.ok("POST", path+"/"+action, map[string]any{"version": v["version"]})
	}
	publish("page", page, "publish")
	makeMap := func(source string, target int64) int64 {
		id := cmsID(cmsData(c.ok("POST", "/cms/redirect", map[string]any{"sourcePath": source, "targetId": target, "status": "0"})), "redirectId")
		redirects = append(redirects, id)
		return id
	}
	state := func(id int64, status string) {
		path := fmt.Sprintf("/cms/redirect/%d", id)
		v := cmsData(c.ok("GET", path, nil))
		c.ok("POST", path+"/status", map[string]any{"version": v["version"], "status": status})
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	visit := func(method, path string) (int, string, string, string) {
		t.Helper()
		req, e := http.NewRequest(method, portalTestURL()+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, res.Header.Get("Location"), string(b), res.Header.Get("Cache-Control")
	}
	alias := "/old-site/" + prefix + ".html"
	id := makeMap(alias, page)
	mapPath := fmt.Sprintf("/cms/redirect/%d", id)
	first := cmsData(c.ok("GET", mapPath, nil))
	if first["status"] != "1" || first["targetPath"] != "/p/"+prefix {
		t.Fatal("new map must be disabled; canonical target projection failed")
	}
	if code, loc, _, _ := visit("GET", alias); code != 404 || loc != "" {
		t.Fatal("disabled redirect took effect")
	}
	cmsCode(t, c.call("POST", "/cms/redirect", c.admin, map[string]any{"sourcePath": alias, "targetId": page}), 500)
	state(id, "0")
	for _, method := range []string{"GET", "HEAD"} {
		code, loc, body, cache := visit(method, alias+"?utm=discarded")
		if code != 301 || loc != "/p/"+prefix || cache != "no-store" || method == "HEAD" && body != "" {
			t.Fatal("exact one-hop redirect/query discard failed", method, code, loc)
		}
	}
	if code, loc, _, _ := visit("POST", alias); code != 404 || loc != "" {
		t.Fatal("POST must not redirect")
	}
	if code, _, body, _ := visit("HEAD", "/p/"+prefix); code != 200 || body != "" {
		t.Fatal("canonical HEAD failed")
	}
	// Canonical content paths and reserved application routes cannot become sources.
	for _, source := range []string{"/p/" + prefix, "/news", "/admin/login", "/cms/redirect/list", "/assets/portal/business/v3/site.css", "//evil.invalid/a", "/x?from=old", "/x%2fy", "/news/c/old"} {
		cmsCode(t, c.call("POST", "/cms/redirect", c.admin, map[string]any{"sourcePath": source, "targetId": page}), 500)
	}
	enabled := cmsData(c.ok("GET", mapPath, nil))
	cmsCode(t, c.call("PUT", "/cms/redirect", c.admin, enabled), 500)
	cmsCode(t, c.call("POST", mapPath+"/status", c.admin, map[string]any{"version": first["version"], "status": "1"}), 500)
	state(id, "1")
	v := cmsData(c.ok("GET", mapPath, nil))
	v["sourcePath"] = "/news/old-" + prefix
	c.ok("PUT", "/cms/redirect", v)
	state(id, "0")
	alias = v["sourcePath"].(string)
	if code, loc, _, _ := visit("GET", alias); code != 301 || loc != "/p/"+prefix {
		t.Fatal("missing content route fallback failed")
	}
	publish("page", page, "unpublish")
	if code, loc, _, _ := visit("GET", alias); code != 404 || loc != "" {
		t.Fatal("unpublished target still redirects")
	}
	state(id, "1")
	v = cmsData(c.ok("GET", mapPath, nil))
	cmsCode(t, c.call("POST", mapPath+"/status", c.admin, map[string]any{"version": v["version"], "status": "0"}), 500)
	publish("page", page, "publish")
	state(id, "0")
	// A reserved legacy canonical path cannot be claimed by a future draft.
	cmsCode(t, c.call("POST", "/cms/article", c.admin, map[string]any{"slug": "old-" + prefix, "title": "冲突内容", "bodyHtml": "<p>不能占用旧路径</p>", "channelId": ch}), 500)
	reserved := makeMap("/p/future-"+prefix, page)
	cmsCode(t, c.call("POST", "/cms/page", c.admin, map[string]any{"slug": "future-" + prefix, "title": "冲突内容", "bodyHtml": "<p>不能占用旧路径</p>"}), 500)
	// New content and source reservation race: exactly one can claim the path.
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan struct {
		kind string
		data map[string]any
	}, 2)
	go func() {
		defer wg.Done()
		results <- struct {
			kind string
			data map[string]any
		}{"content", c.call("POST", "/cms/page", c.admin, map[string]any{"slug": "race-" + prefix, "title": "并发目标", "bodyHtml": "<p>并发检查</p>"})}
	}()
	go func() {
		defer wg.Done()
		results <- struct {
			kind string
			data map[string]any
		}{"redirect", c.call("POST", "/cms/redirect", c.admin, map[string]any{"sourcePath": "/p/race-" + prefix, "targetId": page})}
	}()
	wg.Wait()
	close(results)
	wins := 0
	for r := range results {
		if r.data["code"] == float64(200) {
			wins++
			if r.kind == "content" {
				pages = append(pages, cmsID(cmsData(r.data), "contentId"))
			} else {
				redirects = append(redirects, cmsID(cmsData(r.data), "redirectId"))
			}
		}
	}
	if wins != 1 {
		t.Fatal("path reservation race must allow exactly one writer", wins)
	}
	// Referencing a draft/deleted canonical path is disallowed even though it is not publicly visible.
	draft := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": "draft-" + prefix, "title": "未公开", "bodyHtml": "<p>草稿</p>"})), "contentId")
	pages = append(pages, draft)
	cmsCode(t, c.call("POST", "/cms/redirect", c.admin, map[string]any{"sourcePath": "/p/draft-" + prefix, "targetId": page}), 500)
	cmsCode(t, c.call("POST", "/cms/redirect", c.admin, map[string]any{"sourcePath": "/old-draft-" + prefix, "targetId": draft}), 500)

	article := cmsID(cmsData(c.ok("POST", "/cms/article", map[string]any{"slug": prefix, "title": "栏目目标", "bodyHtml": "<p>栏目停用检查</p>", "channelId": ch})), "contentId")
	articles = append(articles, article)
	publish("article", article, "publish")
	aid := makeMap("/old-article-"+prefix, article)
	state(aid, "0")
	cv := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", ch), nil))
	cv["status"] = "1"
	c.ok("PUT", "/cms/channel", cv)
	if code, loc, _, _ := visit("GET", "/old-article-"+prefix); code != 404 || loc != "" {
		t.Fatal("disabled channel still redirected")
	}
	// List pagination, anonymous denial and add/edit versus publication permissions.
	list := c.ok("GET", "/cms/redirect/list?sourcePath="+url.QueryEscape("/p/future-"+prefix)+"&pageSize=1", nil)
	if list["total"] != float64(1) || len(list["rows"].([]any)) != 1 {
		t.Fatal("filtered paging failed")
	}
	cmsCode(t, c.call("GET", mapPath, "", nil), 401)
	key := "rd" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var rid, uid int64
	role := map[string]any{"roleName": key, "roleKey": key, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6080, 6081, 6082, 6083}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	if e := c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", key).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = rid
	t.Cleanup(func() {
		if uid > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": key, "nickName": "跳转测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": key, "nickName": "仅编辑跳转测试", "password": "Redirect_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e := c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", key).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	token := c.login(key, "Redirect_test_975!")
	defer c.call("POST", "/logout", token, nil)
	cmsCode(t, c.call("GET", "/cms/redirect/list", token, nil), 200)
	rv := cmsData(c.call("GET", fmt.Sprintf("/cms/redirect/%d", reserved), token, nil))
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/redirect/%d/status", reserved), token, map[string]any{"version": rv["version"], "status": "0"}), 403)
	created := c.call("POST", "/cms/redirect", token, map[string]any{"sourcePath": "/editor-" + prefix, "targetId": page, "status": "0"})
	cmsCode(t, created, 200)
	eid := cmsID(cmsData(created), "redirectId")
	redirects = append(redirects, eid)
	if d := cmsData(c.ok("GET", fmt.Sprintf("/cms/redirect/%d", eid), nil)); d["status"] != "1" {
		t.Fatal("editor bypassed explicit publish")
	}
	// Removing a target stops its mapping, yet disabling a stale mapping remains possible.
	pv := cmsData(c.ok("GET", contentPath, nil))
	c.ok("DELETE", contentPath, map[string]any{"version": pv["version"]})
	pages = pages[1:]
	if code, loc, _, _ := visit("GET", alias); code != 404 || loc != "" {
		t.Fatal("deleted target still redirects")
	}
	state(id, "1")
	t.Log("PASS disabled draft, GET/HEAD one-hop301, canonical HEAD, query discard, reserved/self/draft conflicts, CAS, reverse content reservation/race, target unpublish/delete/channel disable and publish permission isolation")
}
