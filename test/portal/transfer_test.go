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
	"testing"
	"time"
)

func TestCMSContentTransfer(t *testing.T) {
	c := cmsSetup(t)
	prefix := "transfer-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	owned := map[int64]string{}
	channels := []int64{}
	t.Cleanup(func() {
		for id, kind := range owned {
			p := fmt.Sprintf("/cms/%s/%d", kind, id)
			out := c.call("GET", p, c.admin, nil)
			if out["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(out)["version"]})
			}
		}
		for _, id := range channels {
			p := fmt.Sprintf("/cms/channel/%d", id)
			out := c.call("GET", p, c.admin, nil)
			if out["code"] == float64(200) {
				row := cmsData(out)
				row["status"] = "1"
				c.ok("PUT", "/cms/channel", row)
			}
		}
	})
	for _, kind := range []string{"page", "article", "product"} {
		root := "/cms/" + kind + "/transfer"
		template := cmsData(c.ok("GET", root+"/template", nil))
		if template["kind"] != strings.ToUpper(kind) {
			t.Fatal("template kind mismatch")
		}
		slug := prefix + "-" + kind
		channel := int64(0)
		if kind != "page" {
			channel = cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": slug, "slug": slug, "kind": strings.ToUpper(kind), "status": "0"})), "channelId")
			channels = append(channels, channel)
		}
		row := map[string]any{"title": slug, "slug": slug, "channelId": channel, "bodyHtml": "<p>导入草稿测试</p>"}
		if kind == "product" {
			row["productCode"] = "TRANSFER-01"
			row["productParams"] = "材质：测试"
		}
		envelope := map[string]any{"format": "portal-content-v1", "kind": strings.ToUpper(kind), "rows": []any{row}}
		check := cmsData(c.ok("POST", root+"/check", envelope))
		if check["failureCount"] != float64(0) || check["checkOnly"] != true {
			t.Fatal("valid check failed", check)
		}
		var n int
		if e := c.db.QueryRow("SELECT COUNT(*) FROM cms_content WHERE slug=? AND kind=?", slug, strings.ToUpper(kind)).Scan(&n); e != nil || n != 0 {
			t.Fatal("dry run wrote data", e)
		}
		bad := map[string]any{"title": "", "slug": slug + "-bad", "bodyHtml": "<p>bad</p>"}
		envelope["rows"] = []any{row, bad}
		applied := cmsData(c.ok("POST", root+"/import", envelope))
		if applied["successCount"] != float64(1) || applied["failureCount"] != float64(1) {
			t.Fatal("partial import counts wrong", applied)
		}
		newRow := applied["rows"].([]any)[0].(map[string]any)
		id := cmsID(newRow, "contentId")
		owned[id] = kind
		detail := cmsData(c.ok("GET", fmt.Sprintf("/cms/%s/%d", kind, id), nil))
		if detail["isPublished"] != float64(0) {
			t.Fatal("import published content")
		}
		if kind == "product" && detail["draft"].(map[string]any)["productCode"] != "TRANSFER-01" {
			t.Fatal("product fields lost")
		}
		exports := cmsData(c.ok("POST", root+"/export", map[string]any{"items": []any{map[string]any{"contentId": id, "version": detail["version"]}}}))
		exportRow := exports["rows"].([]any)[0].(map[string]any)
		if exportRow["contentId"] != float64(0) || exportRow["version"] != float64(0) || exportRow["bodyHtml"] != "<p>导入草稿测试</p>" {
			t.Fatal("export must be importable new-draft DTO", exports)
		}
		cmsCode(t, c.call("POST", root+"/export", c.admin, map[string]any{"items": []any{map[string]any{"contentId": id, "version": 1}}}), 500)
		// An occupied slug and duplicate rows are never applied, including deleted history.
		envelope["rows"] = []any{row, row}
		duplicate := cmsData(c.ok("POST", root+"/import", envelope))
		if duplicate["successCount"] != float64(0) || duplicate["failureCount"] != float64(2) {
			t.Fatal("duplicate slug accepted")
		}
		row["contentId"] = id
		row["version"] = detail["version"]
		row["slug"] = slug + "-overwrite"
		envelope["rows"] = []any{row}
		overwrite := cmsData(c.ok("POST", root+"/import", envelope))
		if overwrite["successCount"] != float64(0) {
			t.Fatal("import allowed updates")
		}
		if cmsData(c.ok("GET", fmt.Sprintf("/cms/%s/%d", kind, id), nil))["version"] != detail["version"] {
			t.Fatal("existing content overwritten")
		}
		delete(row, "contentId")
		delete(row, "version")
		row["slug"] = slug + "-asset"
		row["bodyHtml"] = `<p>缺失素材</p><img src="/media/9223372036854775806">`
		check = cmsData(c.ok("POST", root+"/check", envelope))
		if check["failureCount"] != float64(1) {
			t.Fatal("missing asset accepted")
		}
		envelope["kind"] = "WRONG"
		cmsCode(t, c.call("POST", root+"/check", c.admin, envelope), 500)
		cmsCode(t, c.call("POST", root+"/import", "", envelope), 401)
	}
	// Dedicated import/export permissions are required independently of existing read/add permissions.
	roleKey := "transfer_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var rid, uid int64
	role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6030, 6031, 6032}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	if e := c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = rid
	name := "tf" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		if uid > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": name, "nickName": "导入测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "导入权限测试", "password": "Transfer_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e := c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	token := c.login(name, "Transfer_test_975!")
	defer c.call("POST", "/logout", token, nil)
	body := map[string]any{"format": "portal-content-v1", "kind": "PAGE", "rows": []any{map[string]any{"title": "test", "slug": prefix + "-role", "bodyHtml": "<p>test</p>"}}}
	cmsCode(t, c.call("POST", "/cms/page/transfer/check", token, body), 403)
	cmsCode(t, c.call("POST", "/cms/page/transfer/export", token, map[string]any{"items": []any{map[string]any{"contentId": 1, "version": 1}}}), 403)
	role["menuIds"] = []int64{6030, 6031, 6032, 6072}
	c.ok("PUT", "/system/role", role)
	check := cmsData(c.call("POST", "/cms/page/transfer/check", token, body))
	if check["failureCount"] != float64(0) {
		t.Fatal("explicit import permission not effective")
	}
	body["rows"].([]any)[0].(map[string]any)["bodyHtml"] = `<img src="/media/1">`
	check = cmsData(c.call("POST", "/cms/page/transfer/check", token, body))
	if check["failureCount"] != float64(1) {
		t.Fatal("missing asset query permission ignored")
	}
	// Unknown fields and byte limits are request errors rather than silently ignored imports.
	raw := []byte(`{"format":"portal-content-v1","kind":"PAGE","rows":[],"isPublished":1}`)
	req, _ := http.NewRequest("POST", portalTestURL()+"/cms/page/transfer/import", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.admin)
	resp, e := c.http.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	cmsCode(t, out, 500)
	req, _ = http.NewRequest("POST", portalTestURL()+"/cms/page/transfer/check", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.admin)
	resp, e = c.http.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatal("byte limit not enforced", resp.StatusCode)
	}
	t.Log("PASS three kinds, read-only check, partial apply, new drafts only, normalized export, stale/duplicate rejection, separate permissions, asset checks, strict JSON and byte cap")
}

func TestCMSProductTransferWithAssets(t *testing.T) {
	c := cmsSetup(t)
	slug := "transfer-images-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 16))); e != nil {
		t.Fatal(e)
	}
	upload := c.upload(slug+".png", b.Bytes())
	cmsCode(t, upload, 200)
	asset := cmsID(cmsData(upload), "assetId")
	channel := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": slug, "slug": slug, "kind": "PRODUCT", "status": "0"})), "channelId")
	var id int64
	t.Cleanup(func() {
		if id > 0 {
			p := fmt.Sprintf("/cms/product/%d", id)
			d := cmsData(c.ok("GET", p, nil))
			c.ok("DELETE", p, map[string]any{"version": d["version"]})
		}
		p := fmt.Sprintf("/cms/channel/%d", channel)
		ch := cmsData(c.ok("GET", p, nil))
		ch["status"] = "1"
		c.ok("PUT", "/cms/channel", ch)
	})
	in := map[string]any{"format": "portal-content-v1", "kind": "PRODUCT", "rows": []any{map[string]any{"title": slug, "slug": slug, "channelId": channel, "bodyHtml": fmt.Sprintf(`<p>产品图文</p><img src="/media/%d" onerror="alert(1)">`, asset), "coverAssetId": asset, "galleryIds": []int64{asset}, "productCode": "ASSET-TRANSFER", "productParams": "参数：测试"}}}
	result := cmsData(c.ok("POST", "/cms/product/transfer/import", in))
	if result["successCount"] != float64(1) {
		t.Fatal("valid asset import failed", result)
	}
	id = cmsID(result["rows"].([]any)[0].(map[string]any), "contentId")
	d := cmsData(c.ok("GET", fmt.Sprintf("/cms/product/%d", id), nil))
	data := cmsData(c.ok("POST", "/cms/product/transfer/export", map[string]any{"items": []any{map[string]any{"contentId": id, "version": d["version"]}}}))
	row := data["rows"].([]any)[0].(map[string]any)
	gallery := row["galleryIds"].([]any)
	if len(gallery) != 1 || gallery[0] != float64(asset) || row["coverAssetId"] != float64(asset) || row["productCode"] != "ASSET-TRANSFER" || strings.Contains(row["bodyHtml"].(string), "onerror") {
		t.Fatal("asset/gallery or normalized body did not round-trip", row)
	}
	c.ok("POST", fmt.Sprintf("/cms/product/%d/publish", id), map[string]any{"version": d["version"]})
	published := cmsData(c.ok("GET", fmt.Sprintf("/cms/product/%d", id), nil))
	if published["isPublished"] != float64(1) {
		t.Fatal("publication fixture failed")
	}
	body := published["draft"].(map[string]any)
	body["contentId"] = id
	body["version"] = published["version"]
	body["slug"] = slug
	body["bodyHtml"] = "<p>尚未发布的新工作稿</p>"
	c.ok("PUT", "/cms/product", body)
	current := cmsData(c.ok("GET", fmt.Sprintf("/cms/product/%d", id), nil))
	data = cmsData(c.ok("POST", "/cms/product/transfer/export", map[string]any{"items": []any{map[string]any{"contentId": id, "version": current["version"]}}}))
	if data["rows"].([]any)[0].(map[string]any)["bodyHtml"] != "<p>尚未发布的新工作稿</p>" {
		t.Fatal("export used public pointer instead of working pointer")
	}
	resp, e := c.http.Get(portalTestURL() + "/products/" + slug)
	if e != nil {
		t.Fatal(e)
	}
	html, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || resp.StatusCode != 200 || strings.Contains(string(html), "尚未发布的新工作稿") {
		t.Fatal("export/edit leaked draft online")
	}
	t.Log("PASS product gallery/cover/code, sanitized body, work/public separation; historical image remains protected")
}
