package portaltest

import (
	"bytes"
	"context"
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

func (c *cmsClient) public(path string, modified string) (int, string, string) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", "http://127.0.0.1:8085"+path, nil)
	if modified != "" {
		req.Header.Set("If-Modified-Since", modified)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal("public request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		c.t.Fatal(err)
	}
	return resp.StatusCode, string(body), resp.Header.Get("Last-Modified")
}
func TestCMSContentLifecycle(t *testing.T) {
	c := cmsSetup(t)
	prefix := "content-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 30, 20)))
	asset := cmsData(c.upload(prefix+".png", buf.Bytes()))
	assetID := cmsID(asset, "assetId")
	media := fmt.Sprintf("/media/%d", assetID)
	channel := map[string]any{"name": prefix, "slug": prefix, "kind": "ARTICLE", "status": "0", "parentId": 0, "coverAssetId": 0, "orderNum": 0}
	channelID := cmsID(cmsData(c.ok("POST", "/cms/channel", channel)), "channelId")
	channelPath := fmt.Sprintf("/cms/channel/%d", channelID)
	draft := map[string]any{"slug": prefix, "channelId": channelID, "title": "初始公开标题", "bodyHtml": `<h2>初始正文</h2><img src="` + media + `"><script>alert('xss')</script>`, "coverAssetId": assetID, "publishedRevisionId": 999, "isPublished": 1}
	id := cmsID(cmsData(c.ok("POST", "/cms/article", draft)), "contentId")
	path := fmt.Sprintf("/cms/article/%d", id)
	pub := "/news/" + prefix
	var pageID int64
	t.Cleanup(func() {
		for _, item := range []struct {
			kind string
			id   int64
		}{{"article", id}, {"page", pageID}} {
			if item.id > 0 {
				p := fmt.Sprintf("/cms/%s/%d", item.kind, item.id)
				r := c.call("GET", p, c.admin, nil)
				if r["code"] == float64(200) {
					c.call("DELETE", p, c.admin, map[string]any{"version": cmsData(r)["version"]})
				}
			}
		}
		row := cmsData(c.ok("GET", channelPath, nil))
		row["status"] = "1"
		c.ok("PUT", "/cms/channel", row)
	})
	get := func() map[string]any { return cmsData(c.ok("GET", path, nil)) }
	assertPublic := func(want int, contains string) {
		t.Helper()
		status, body, _ := c.public(pub, "")
		if status != want || (contains != "" && !strings.Contains(body, contains)) {
			t.Fatalf("public state got status=%d expected=%d", status, want)
		}
	}
	assertPublic(404, "")
	if status, _, _ := c.public(media, ""); status != 404 {
		t.Fatal("draft media leaked")
	}
	first := get()
	if first["isPublished"] != float64(0) {
		t.Fatal("draft mass assignment published content")
	}
	initial := cmsID(first["draft"].(map[string]any), "revisionId")
	c.ok("POST", path+"/publish", map[string]any{"version": first["version"]})
	assertPublic(200, "初始正文")
	status, _, modified := c.public(media, "")
	if status != 200 {
		t.Fatal("published image missing")
	}
	row := get()
	draft["contentId"] = id
	draft["version"] = row["version"]
	draft["title"] = "未发布新稿标题"
	marker := "PRIVATE_DRAFT_" + prefix
	draft["bodyHtml"] = "<p>" + marker + "</p>"
	draft["coverAssetId"] = 0
	c.ok("PUT", "/cms/article", draft)
	assertPublic(200, "初始正文")
	_, body, _ := c.public(pub, "")
	if strings.Contains(body, marker) {
		t.Fatal("new draft changed published body")
	}
	cmsCode(t, c.call("PUT", "/cms/article", c.admin, draft), 500)
	row = get()
	draft["version"] = row["version"]
	draft["slug"] = prefix + "-moved"
	cmsCode(t, c.call("PUT", "/cms/article", c.admin, draft), 500)
	draft["slug"] = prefix
	bad := map[string]any{}
	for k, v := range draft {
		bad[k] = v
	}
	bad["bodyHtml"] = `<p><img src="https://example.com/private.png"></p>`
	cmsCode(t, c.call("PUT", "/cms/article", c.admin, bad), 500)
	// Global lock + version compare: exactly one of four simultaneous saves commits.
	var wg sync.WaitGroup
	codes := make(chan float64, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := map[string]any{}
			for k, v := range draft {
				input[k] = v
			}
			input["title"] = fmt.Sprintf("并发新稿%d", i)
			b, _ := json.Marshal(input)
			req, _ := http.NewRequest("PUT", "http://127.0.0.1:8085/cms/article", bytes.NewReader(b))
			req.Header.Set("Authorization", "Bearer "+c.admin)
			req.Header.Set("Content-Type", "application/json")
			resp, e := c.http.Do(req)
			if e != nil {
				codes <- -1
				return
			}
			defer resp.Body.Close()
			out := map[string]any{}
			if json.NewDecoder(resp.Body).Decode(&out) != nil {
				codes <- -2
				return
			}
			code, ok := out["code"].(float64)
			if !ok {
				codes <- -3
				return
			}
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	wins := 0
	for code := range codes {
		if code == 200 {
			wins++
		} else if code != 500 {
			t.Fatal("unexpected concurrent result", code)
		}
	}
	if wins != 1 {
		t.Fatal("same-version content commits", wins)
	}
	assertPublic(200, "初始正文")
	row = get()
	c.ok("POST", path+"/publish", map[string]any{"version": row["version"]})
	assertPublic(200, marker)
	// Old published image loses public reachability even with a conditional request, but history prevents deletion.
	if status, _, _ := c.public(media, modified); status != 404 {
		t.Fatal("unpublished revision image/304 leaked")
	}
	cmsCode(t, c.call("DELETE", fmt.Sprintf("/cms/asset/%d", assetID), c.admin, map[string]any{"version": 1}), 500)
	channelRow := cmsData(c.ok("GET", channelPath, nil))
	cmsCode(t, c.call("DELETE", channelPath, c.admin, map[string]any{"version": channelRow["version"]}), 500)
	channelRow["kind"] = "PRODUCT"
	cmsCode(t, c.call("PUT", "/cms/channel", c.admin, channelRow), 500)
	channelRow["kind"] = "ARTICLE"
	hist := c.ok("GET", path+"/history?pageSize=100", nil)
	if hist["total"] != float64(3) {
		t.Fatal("immutable revision count", hist["total"])
	}
	row = get()
	c.ok("POST", path+"/restore", map[string]any{"version": row["version"], "revisionId": initial})
	assertPublic(200, marker)
	row = get()
	if row["revisionSeq"] != float64(4) {
		t.Fatal("restore did not append revision")
	}
	c.ok("POST", path+"/publish", map[string]any{"version": row["version"]})
	assertPublic(200, "初始正文")
	// Channel disable/enable controls both detail and public media.
	channelRow["status"] = "1"
	c.ok("PUT", "/cms/channel", channelRow)
	assertPublic(404, "")
	if status, _, _ := c.public(media, modified); status != 404 {
		t.Fatal("disabled channel media leaked")
	}
	channelRow = cmsData(c.ok("GET", channelPath, nil))
	channelRow["status"] = "0"
	c.ok("PUT", "/cms/channel", channelRow)
	// Article-only editor may query/edit drafts, but cannot publish or access single-page content.
	roleKey := "content_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	username := "ct" + strconv.FormatInt(time.Now().UnixNano(), 36)
	password := "Content_test_7426!"
	role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int{6000, 6020, 6021, 6022, 6023, 6010, 6011}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	var roleID, userID int64
	if e := c.db.QueryRowContext(ctx, "SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&roleID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if userID > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": userID, "userName": username, "nickName": "内容测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["roleId"] = roleID
		role["menuIds"] = []int{}
		role["status"] = "1"
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": username, "nickName": "内容权限测试", "password": password, "deptId": 103, "status": "0", "roleIds": []int64{roleID}, "postIds": []int64{}})
	if e := c.db.QueryRowContext(ctx, "SELECT user_id FROM sys_user WHERE user_name=?", username).Scan(&userID); e != nil {
		t.Fatal(e)
	}
	editor := c.login(username, password)
	defer c.call("POST", "/logout", editor, nil)
	cmsCode(t, c.call("GET", path, editor, nil), 200)
	row = get()
	draft["version"] = row["version"]
	draft["title"] = "受限编辑员新稿"
	cmsCode(t, c.call("PUT", "/cms/article", editor, draft), 200)
	assertPublic(200, "初始正文")
	row = get()
	cmsCode(t, c.call("POST", path+"/publish", editor, map[string]any{"version": row["version"]}), 403)
	cmsCode(t, c.call("GET", "/cms/page/list", editor, nil), 403)
	cmsCode(t, c.call("GET", "/cms/product/list", editor, nil), 403)
	cmsCode(t, c.call("GET", "/cms/site", editor, nil), 403)
	cmsCode(t, c.call("GET", "/cms/site/preview", editor, nil), 403)
	cmsCode(t, c.call("GET", path+"/check", editor, nil), 200)
	cmsCode(t, c.call("POST", "/cms/article/batch", editor, map[string]any{"action": "publish", "items": []any{map[string]any{"contentId": id, "version": row["version"]}}}), 403)
	cmsCode(t, c.call("GET", "/cms/site/inspect", editor, nil), 403)
	cmsCode(t, c.call("GET", path, "", nil), 401)
	pageDraft := map[string]any{"slug": prefix + "-page", "title": "单页测试", "bodyHtml": "<p>单页正文</p><img src=\"" + media + "\">", "channelId": 0}
	pageID = cmsID(cmsData(c.ok("POST", "/cms/page", pageDraft)), "contentId")
	cmsCode(t, c.call("GET", fmt.Sprintf("/cms/page/%d", id), c.admin, nil), 500)
	pageRow := cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", pageID), nil))
	c.ok("POST", fmt.Sprintf("/cms/page/%d/publish", pageID), map[string]any{"version": pageRow["version"]})
	if status, body, _ := c.public("/p/"+prefix+"-page", ""); status != 200 || !strings.Contains(body, "单页正文") {
		t.Fatal("page publication failed")
	}
	row = get()
	c.ok("POST", path+"/unpublish", map[string]any{"version": row["version"]})
	assertPublic(404, "")
	if status, _, _ := c.public(media, modified); status != 200 && status != 304 {
		t.Fatal("shared image disappeared while another page remains published")
	}
	pageRow = cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", pageID), nil))
	c.ok("POST", fmt.Sprintf("/cms/page/%d/unpublish", pageID), map[string]any{"version": pageRow["version"]})
	if status, _, _ := c.public(media, modified); status != 404 {
		t.Fatal("unpublished media leaked")
	}
	listing := c.call("GET", "/portal/articles", "", nil)
	cmsCode(t, listing, 200)
	for _, item := range listing["rows"].([]any) {
		entry := item.(map[string]any)
		if entry["slug"] == prefix {
			t.Fatal("unpublished article remained in public summary")
		}
		if _, exists := entry["workingRevisionId"]; exists {
			t.Fatal("private pointer leaked into public list")
		}
	}
	role["roleId"] = roleID
	role["menuIds"] = []int{}
	c.ok("PUT", "/system/role", role)
	cmsCode(t, c.call("GET", path, editor, nil), 403)

	// No private body in asynchronous audit records.
	time.Sleep(300 * time.Millisecond)
	var leaked int
	if e := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys_oper_log WHERE oper_param LIKE ? OR json_result LIKE ?", "%"+marker+"%", "%"+marker+"%").Scan(&leaked); e != nil || leaked != 0 {
		t.Fatal("private draft reached audit log", e)
	}
	t.Log("PASS: draft isolation, CAS, immutable restore, media/304 gating, channel gating, separate publication permission, article/page boundary and audit privacy")
}
