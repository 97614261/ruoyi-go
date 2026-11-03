package portaltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type cmsFeatureActor struct {
	id          int64
	name, token string
	role        map[string]any
}

func cmsNewFeatureActor(c *cmsClient, name string, perms ...string) cmsFeatureActor {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	marks := []string{}
	args := []any{}
	for _, p := range perms {
		marks = append(marks, "?")
		args = append(args, p)
	}
	rows, e := c.db.QueryContext(ctx, "SELECT menu_id FROM sys_menu WHERE perms IN ("+strings.Join(marks, ",")+")", args...)
	if e != nil {
		c.t.Fatal(e)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			c.t.Fatal(e)
		}
		ids = append(ids, id)
	}
	rows.Close()
	role := map[string]any{"roleName": name, "roleKey": name, "roleSort": 1, "status": "0", "dataScope": "1", "menuCheckStrictly": true, "deptCheckStrictly": true, "menuIds": ids}
	c.ok("POST", "/system/role", role)
	var rid, id int64
	if e = c.db.QueryRowContext(ctx, "SELECT role_id FROM sys_role WHERE role_key=?", name).Scan(&rid); e != nil {
		c.t.Fatal(e)
	}
	role["roleId"] = rid
	c.t.Cleanup(func() {
		role["status"] = "1"
		role["menuIds"] = []int64{}
		r := c.call("PUT", "/system/role", c.admin, role)
		if r["code"] != float64(200) {
			c.t.Error("fixture role retirement failed")
		}
	})
	c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "资料预设专用验收", "password": "Feature_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e = c.db.QueryRowContext(ctx, "SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&id); e != nil {
		c.t.Fatal(e)
	}
	c.t.Cleanup(func() {
		r := c.call("PUT", "/system/user", c.admin, map[string]any{"userId": id, "userName": name, "nickName": "资料预设测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		if r["code"] != float64(200) {
			c.t.Error("fixture user retirement failed")
		}
	})
	token := c.login(name, "Feature_975!")
	c.t.Cleanup(func() { c.call("POST", "/logout", token, nil) })
	return cmsFeatureActor{id, name, token, role}
}
func cmsFeatureUpload(c *cmsClient, token, name string, b []byte) map[string]any {
	c.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	f, e := form.CreateFormFile("file", name)
	if e != nil {
		c.t.Fatal(e)
	}
	f.Write(b)
	form.Close()
	req, _ := http.NewRequest("POST", portalTestURL()+"/cms/download/files/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r, e := c.http.Do(req)
	if e != nil {
		c.t.Fatal("upload request failed")
	}
	defer r.Body.Close()
	var out map[string]any
	if e = json.NewDecoder(r.Body).Decode(&out); e != nil {
		c.t.Fatal("upload response invalid")
	}
	return out
}
func TestPortalDownloads(t *testing.T) {
	c := cmsSetup(t)
	key := "dl" + strconv.FormatInt(time.Now().UnixNano(), 36)
	editor := cmsNewFeatureActor(c, key+"e", "cms:download:list", "cms:download:query", "cms:download:add", "cms:download:edit", "cms:download:upload", "cms:download:category")
	publisher := cmsNewFeatureActor(c, key+"p", "cms:download:list", "cms:download:query", "cms:download:publish")
	none := cmsNewFeatureActor(c, key+"n", "cms:page:list")
	var id int64
	files := []map[string]any{}
	used := map[int64]bool{}
	t.Cleanup(func() {
		if id > 0 {
			p := fmt.Sprintf("/cms/download/%d", id)
			r := c.call("GET", p, c.admin, nil)
			if r["code"] == float64(200) {
				c.call("DELETE", p, c.admin, map[string]any{"version": cmsData(r)["version"]})
			}
		}
		for _, f := range files {
			fid := cmsID(f, "fileId")
			if !used[fid] {
				c.call("DELETE", fmt.Sprintf("/cms/download/files/%d", fid), c.admin, map[string]any{"version": f["version"]})
			}
		}
	})
	cat := cmsID(cmsData(c.ok("POST", "/cms/download/categories", map[string]any{"name": key, "orderNum": 3, "enabled": true})), "categoryId")
	category := func() map[string]any {
		r := c.ok("GET", "/cms/download/categories?name="+key, nil)
		for _, v := range r["rows"].([]any) {
			row := v.(map[string]any)
			if cmsID(row, "categoryId") == cat {
				return row
			}
		}
		t.Fatal("fixture category missing")
		return nil
	}
	t.Cleanup(func() { r := category(); r["enabled"] = false; c.call("POST", "/cms/download/categories", c.admin, r) })
	pdfA, pdfB := []byte("%PDF-1.4\nprivate-old-"+key+"\n%%EOF"), []byte("%PDF-1.4\nprivate-new-"+key+"\n%%EOF")
	cmsCode(t, cmsFeatureUpload(c, "", "x.pdf", pdfA), 401)
	cmsCode(t, cmsFeatureUpload(c, none.token, "x.pdf", pdfA), 403)
	for _, bad := range []struct {
		name string
		b    []byte
	}{{"x.exe", pdfA}, {"x.pdf", []byte("not pdf")}, {"x.txt", []byte{0, 1, 2}}, {"x.docx", []byte("PK fake")}, {"x.zip", []byte("not zip")}} {
		cmsCode(t, cmsFeatureUpload(c, editor.token, bad.name, bad.b), 500)
	}
	tooLarge := bytes.Repeat([]byte("a"), 20<<20+1)
	cmsCode(t, cmsFeatureUpload(c, editor.token, "large.txt", tooLarge), 500)
	upload := func(name string, b []byte) map[string]any {
		out := cmsFeatureUpload(c, editor.token, name, b)
		cmsCode(t, out, 200)
		f := cmsData(out)
		files = append(files, f)
		if f["storageKey"] != nil {
			t.Fatal("private storage key leaked")
		}
		return f
	}
	fa, fb := upload("产品说明-"+key+".PDF", pdfA), upload("更新说明-"+key+".pdf", pdfB)
	rawFile := func(method, token, extra string) (int, []byte, http.Header) {
		req, _ := http.NewRequest(method, portalTestURL()+fmt.Sprintf("/downloads/%d/file", id), nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if extra != "" {
			req.Header.Set("If-Modified-Since", extra)
		}
		r, e := c.http.Do(req)
		if e != nil {
			t.Fatal("public file request failed")
		}
		defer r.Body.Close()
		b, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		return r.StatusCode, b, r.Header
	}
	payload := map[string]any{"title": key + "-old", "summary": "public description", "categoryId": cat, "fileId": fa["fileId"]}
	out := c.call("POST", "/cms/download", editor.token, payload)
	cmsCode(t, out, 200)
	id = cmsID(cmsData(out), "downloadId")
	used[cmsID(fa, "fileId")] = true
	p := fmt.Sprintf("/cms/download/%d", id)
	get := func() map[string]any { return cmsData(c.ok("GET", p, nil)) }
	if status, _, _ := rawFile("GET", "", ""); status != 404 {
		t.Fatal("draft file leaked")
	}
	if status, b, _ := c.public("/downloads?title="+url.QueryEscape(key), ""); status != 200 || strings.Contains(b, key+"-old") {
		t.Fatal("draft metadata leaked")
	}
	cmsCode(t, c.call("POST", p+"/publish", editor.token, map[string]any{"version": get()["version"]}), 403)
	cmsCode(t, c.call("POST", p+"/publish", publisher.token, map[string]any{"version": get()["version"]}), 200)
	status, b, h := rawFile("GET", "", "")
	if status != 200 || !bytes.Equal(b, pdfA) || !strings.Contains(h.Get("Content-Disposition"), "attachment") || !strings.Contains(h.Get("Cache-Control"), "no-store") || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("public file content/headers")
	}
	oldETag := h.Get("ETag")
	if oldETag == "" {
		t.Fatal("version ETag missing")
	}
	conditional := func(tag string) (int, []byte) {
		req, _ := http.NewRequest("GET", portalTestURL()+fmt.Sprintf("/downloads/%d/file", id), nil)
		req.Header.Set("If-None-Match", tag)
		resp, e := c.http.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			t.Fatal(e)
		}
		return resp.StatusCode, b
	}
	if status, b := conditional(oldETag); status != 304 || len(b) != 0 {
		t.Fatal("current file ETag did not revalidate")
	}
	if status, b, _ := rawFile("HEAD", "", ""); status != 200 || len(b) != 0 {
		t.Fatal("HEAD file handling")
	}
	d := get()
	payload["downloadId"] = id
	payload["version"] = d["version"]
	payload["title"] = key + "-new"
	payload["fileId"] = fb["fileId"]
	cmsCode(t, c.call("PUT", "/cms/download", editor.token, payload), 200)
	used[cmsID(fb, "fileId")] = true
	if status, b, _ := rawFile("GET", "", ""); status != 200 || !bytes.Equal(b, pdfA) {
		t.Fatal("saving draft changed public bytes")
	}
	_, listing, _ := c.public("/downloads?title="+url.QueryEscape(key), "")
	if !strings.Contains(listing, key+"-old") || strings.Contains(listing, key+"-new") {
		t.Fatal("draft title replaced public title")
	}
	cmsCode(t, c.call("PUT", "/cms/download", editor.token, payload), 500)
	publisher.role["menuIds"] = []int64{}
	c.ok("PUT", "/system/role", publisher.role)
	cmsCode(t, c.call("POST", p+"/publish", publisher.token, map[string]any{"version": get()["version"]}), 403)
	c.ok("POST", p+"/publish", map[string]any{"version": get()["version"]})
	if status, b, _ := rawFile("GET", "", ""); status != 200 || !bytes.Equal(b, pdfB) {
		t.Fatal("explicit publication did not switch bytes")
	}
	if status, b := conditional(oldETag); status != 200 || !bytes.Equal(b, pdfB) {
		t.Fatal("old file condition returned stale 304 after publication")
	}
	r := category()
	r["enabled"] = false
	c.ok("POST", "/cms/download/categories", r)
	if status, _, _ := rawFile("GET", "", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)); status != 404 {
		t.Fatal("conditional response bypassed disabled category")
	}
	r = category()
	r["enabled"] = true
	c.ok("POST", "/cms/download/categories", r)
	cmsCode(t, c.call("DELETE", fmt.Sprintf("/cms/download/files/%v", fa["fileId"]), c.admin, map[string]any{"version": fa["version"]}), 500)
	cmsCode(t, c.call("DELETE", fmt.Sprintf("/cms/download/categories/%d", cat), c.admin, map[string]any{"version": category()["version"]}), 500)
	hist := c.ok("GET", p+"/history?pageSize=1000", nil)
	if hist["total"] != float64(2) || len(hist["rows"].([]any)) > 100 {
		t.Fatal("immutable download history")
	}
	cmsCode(t, c.call("GET", "/cms/download/files", none.token, nil), 403)
	cmsCode(t, c.call("GET", fmt.Sprintf("/cms/download/files/%v/file", fa["fileId"]), none.token, nil), 403)
	c.ok("POST", p+"/unpublish", map[string]any{"version": get()["version"]})
	if status, _, _ := rawFile("GET", "", h.Get("Last-Modified")); status != 404 {
		t.Fatal("unpublished file leaked")
	}
}
func TestPortalIndustryPresets(t *testing.T) {
	c := cmsSetup(t)
	key := "pr" + strconv.FormatInt(time.Now().UnixNano(), 36)
	before := cmsData(c.ok("GET", "/cms/site", nil))
	ids := []int64{}
	t.Cleanup(func() {
		s := cmsData(c.ok("GET", "/cms/site", nil))
		if s["publishedReleaseId"] != before["publishedReleaseId"] {
			t.Error("preset changed published appearance")
		}
		c.ok("PUT", "/cms/site", map[string]any{"version": s["version"], "config": before["config"]})
		for _, id := range ids {
			p := fmt.Sprintf("/cms/page/%d", id)
			r := c.call("GET", p, c.admin, nil)
			if r["code"] == float64(200) {
				c.call("DELETE", p, c.admin, map[string]any{"version": cmsData(r)["version"]})
			}
		}
	})
	operator := cmsNewFeatureActor(c, key+"o", "cms:preset:list", "cms:preset:apply", "cms:site:query", "cms:site:edit", "cms:page:add", "cms:page:query", "cms:asset:query")
	style := cmsNewFeatureActor(c, key+"s", "cms:preset:list", "cms:preset:apply", "cms:site:query", "cms:site:edit", "cms:asset:query")
	none := cmsNewFeatureActor(c, key+"n", "cms:site:query")
	list := c.call("GET", "/cms/presets/list?pageSize=2", operator.token, nil)
	cmsCode(t, list, 200)
	if list["total"] != float64(3) || len(list["rows"].([]any)) != 2 {
		t.Fatal("preset catalog pagination")
	}
	cmsCode(t, c.call("GET", "/cms/presets/list", none.token, nil), 403)
	for _, id := range []string{"manufacturing", "business-services", "news-content"} {
		r := c.call("GET", "/cms/presets/"+id+"/preview", operator.token, nil)
		cmsCode(t, r, 200)
		if !strings.Contains(r["data"].(string), "预设示意") {
			t.Fatal("trusted preview missing")
		}
	}
	input := map[string]any{"presetId": "manufacturing", "siteVersion": before["version"], "slugPrefix": key, "pageKeys": []string{"about", "services", "contact"}}
	cmsCode(t, c.call("POST", "/cms/presets/plan", style.token, input), 403)
	plan := cmsData(c.ok("POST", "/cms/presets/plan", input))
	if len(plan["conflicts"].([]any)) != 0 || len(plan["pages"].([]any)) != 3 {
		t.Fatal("unexpected initial preset plan")
	}
	if cmsData(c.ok("GET", "/cms/site", nil))["version"] != before["version"] {
		t.Fatal("preview modified site")
	}
	body := map[string]any{}
	for k, v := range input {
		body[k] = v
	}
	body["planSha256"] = plan["planSha256"]
	body["requestId"] = uuid.NewString()
	body["reason"] = key + " dedicated fixture"
	cmsCode(t, c.call("POST", "/cms/presets/apply", style.token, body), 403)
	bad := map[string]any{}
	for k, v := range body {
		bad[k] = v
	}
	bad["planSha256"] = strings.Repeat("0", 64)
	cmsCode(t, c.call("POST", "/cms/presets/apply", operator.token, bad), 500)
	var wg sync.WaitGroup
	results := make(chan map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- c.call("POST", "/cms/presets/apply", operator.token, body) }()
	}
	wg.Wait()
	close(results)
	var app int64
	for r := range results {
		cmsCode(t, r, 200)
		a := cmsData(r)
		if app == 0 {
			app = cmsID(a, "applicationId")
			for _, id := range a["pageIds"].([]any) {
				ids = append(ids, int64(id.(float64)))
			}
		} else if cmsID(a, "applicationId") != app {
			t.Fatal("idempotent request made two applications")
		}
	}
	if len(ids) != 3 {
		t.Fatal("wrong generated page count")
	}
	state := cmsData(c.ok("GET", "/cms/site", nil))
	if cmsID(state, "version") != cmsID(before, "version")+1 || state["publishedReleaseId"] != before["publishedReleaseId"] {
		t.Fatal("draft/public pointer separation")
	}
	config := state["config"].(map[string]any)
	oldConfig := before["config"].(map[string]any)
	for _, field := range []string{"siteName", "phone", "email", "logoAssetId", "iconAssetId", "footer"} {
		if config[field] != oldConfig[field] {
			t.Fatal("preset overwrote identity", field)
		}
	}
	for _, id := range ids {
		d := cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil))
		if d["isPublished"] != float64(0) || d["publishedRevisionId"] != nil {
			t.Fatal("generated page auto-published")
		}
		status, _, _ := c.public("/p/"+d["slug"].(string), "")
		if status != 404 {
			t.Fatal("preset draft publicly visible")
		}
		var author int64
		if e := c.db.QueryRow("SELECT author_user_id FROM cms_draft_actor WHERE kind='PAGE' AND object_id=? AND snapshot_key=?", id, strconv.FormatInt(cmsID(d["draft"].(map[string]any), "revisionId"), 10)).Scan(&author); e != nil || author != operator.id {
			t.Fatal("generated author attribution", e)
		}
	}
	body["reason"] = "different request body"
	cmsCode(t, c.call("POST", "/cms/presets/apply", operator.token, body), 500)
	body["reason"] = key + " dedicated fixture"
	body["requestId"] = uuid.NewString()
	cmsCode(t, c.call("POST", "/cms/presets/apply", operator.token, body), 500)
	input["siteVersion"] = state["version"]
	conflicts := cmsData(c.ok("POST", "/cms/presets/plan", input))
	if len(conflicts["conflicts"].([]any)) != 3 {
		t.Fatal("same addresses should conflict")
	}
	for k, v := range input {
		body[k] = v
	}
	body["requestId"] = uuid.NewString()
	body["planSha256"] = conflicts["planSha256"]
	cmsCode(t, c.call("POST", "/cms/presets/apply", operator.token, body), 500)
	input["slugPrefix"] = key + "-style"
	input["pageKeys"] = []string{}
	input["presetId"] = "news-content"
	plan = cmsData(c.ok("POST", "/cms/presets/plan", input))
	for k, v := range input {
		body[k] = v
	}
	body["planSha256"] = plan["planSha256"]
	body["requestId"] = uuid.NewString()
	r := c.call("POST", "/cms/presets/apply", style.token, body)
	cmsCode(t, r, 200)
	if len(cmsData(r)["pageIds"].([]any)) != 0 {
		t.Fatal("style-only application created pages")
	}
	hist := c.ok("GET", "/cms/presets/history?pageSize=1000", nil)
	encoded, _ := json.Marshal(hist)
	if strings.Contains(string(encoded), "beforeConfig") || strings.Contains(string(encoded), "requestSha256") || len(hist["rows"].([]any)) > 100 {
		t.Fatal("history projection or cap")
	}
	cmsCode(t, c.call("POST", "/cms/site/publish", operator.token, map[string]any{"version": cmsData(c.ok("GET", "/cms/site", nil))["version"]}), 403)
}
