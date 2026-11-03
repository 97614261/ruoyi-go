package portaltest

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalAssetGroups(t *testing.T) {
	c := cmsSetup(t)
	prefix := "assetgroup-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var imageBytes bytes.Buffer
	if e := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 80, 24))); e != nil {
		t.Fatal(e)
	}
	ids := []int64{}
	t.Cleanup(func() {
		for _, id := range ids {
			path := fmt.Sprintf("/cms/asset/%d", id)
			d := c.call("GET", path, c.admin, nil)
			if d["code"] == float64(200) {
				c.ok("DELETE", path, map[string]any{"version": cmsData(d)["version"]})
			}
		}
	})
	group := prefix + "组"
	for _, name := range []string{prefix + "%literal.png", prefix + "other.png"} {
		a := c.upload(name, imageBytes.Bytes(), " "+group+" ")
		cmsCode(t, a, 200)
		ids = append(ids, cmsID(cmsData(a), "assetId"))
	}
	cmsCode(t, c.upload("invalid.png", imageBytes.Bytes(), strings.Repeat("图", 41)), 500)
	cmsCode(t, c.upload("invalid.png", imageBytes.Bytes(), "../bad"), 500)
	path := fmt.Sprintf("/cms/asset/%d", ids[0])
	before := cmsData(c.ok("GET", path, nil))
	if before["groupName"] != group {
		t.Fatal("upload group not normalized")
	}
	_, _, original := c.image(ids[0], c.admin)
	query := "?groupName=" + url.QueryEscape(group) + "&minWidth=80&maxWidth=80&mimeType=image%2Fpng"
	list := c.ok("GET", "/cms/asset/list"+query, nil)
	stats := cmsData(c.ok("GET", "/cms/asset/statistics"+query, nil))
	if list["total"] != float64(2) || stats["total"] != float64(2) || stats["byteSize"] != before["byteSize"].(float64)*2 || stats["maxWidth"] != float64(80) || stats["maxHeight"] != float64(24) {
		t.Fatal("filtered stats or group mismatch")
	}
	literal := c.ok("GET", "/cms/asset/list?name="+url.QueryEscape(prefix+"%"), nil)
	if literal["total"] != float64(1) {
		t.Fatal("literal percent became wildcard")
	}
	// The optional group field must preserve metadata for older clients that omit it.
	c.ok("PUT", path, map[string]any{"name": before["name"], "version": before["version"], "altText": "old client"})
	current := cmsData(c.ok("GET", path, nil))
	if current["groupName"] != group {
		t.Fatal("omitted field cleared group")
	}
	oldVersion := current["version"]
	current["groupName"] = ""
	c.ok("PUT", path, current)
	current = cmsData(c.ok("GET", path, nil))
	if current["groupName"] != "" {
		t.Fatal("explicit clear failed")
	}
	cmsCode(t, c.call("PUT", path, c.admin, map[string]any{"name": before["name"], "version": oldVersion, "groupName": group}), 500)
	_, _, after := c.image(ids[0], c.admin)
	if !bytes.Equal(original, after) {
		t.Fatal("group edit changed stored image")
	}
	stats = cmsData(c.ok("GET", "/cms/asset/statistics"+query, nil))
	if stats["total"] != float64(1) {
		t.Fatal("group transfer not reflected")
	}
	ungrouped := c.ok("GET", "/cms/asset/list?groupName=&name="+url.QueryEscape(prefix), nil)
	if d := c.ok("GET", "/cms/asset/list?ungrouped=true&name="+url.QueryEscape(prefix), nil); d["total"] != float64(1) {
		t.Fatal("explicit ungrouped filter failed")
	}
	if d := cmsData(c.ok("GET", "/cms/asset/statistics?ungrouped=true&name="+url.QueryEscape(prefix), nil)); d["total"] != float64(1) {
		t.Fatal("explicit ungrouped aggregate failed")
	}
	cmsCode(t, c.call("GET", "/cms/asset/list?ungrouped=true&groupName="+url.QueryEscape(group), c.admin, nil), 500)
	if ungrouped["total"] != float64(1) {
		t.Fatal("empty group should select only ungrouped")
	}
	for _, suffix := range []string{"?minWidth=90&maxWidth=80", "?minBytes=20&maxBytes=10", "?mimeType=image%2Fsvg%2Bxml", "?minBytes=-1"} {
		cmsCode(t, c.call("GET", "/cms/asset/statistics"+suffix, c.admin, nil), 500)
		cmsCode(t, c.call("GET", "/cms/asset/list"+suffix, c.admin, nil), 500)
	}
	empty := cmsData(c.ok("GET", "/cms/asset/statistics?groupName="+url.QueryEscape(prefix+"-missing"), nil))
	if empty["total"] != float64(0) || empty["byteSize"] != float64(0) {
		t.Fatal("empty stats should be zero")
	}
	cmsCode(t, c.call("GET", "/cms/asset/statistics", "", nil), 401)
	key := "ag" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var rid, uid int64
	role := map[string]any{"roleName": key, "roleKey": key, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6010}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	if e := c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", key).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = rid
	t.Cleanup(func() {
		if uid > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": key, "nickName": "素材测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": key, "nickName": "素材列表测试", "password": "Asset_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e := c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", key).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	token := c.login(key, "Asset_test_975!")
	defer c.call("POST", "/logout", token, nil)
	cmsCode(t, c.call("GET", "/cms/asset/list"+query, token, nil), 200)
	cmsCode(t, c.call("GET", "/cms/asset/statistics"+query, token, nil), 200)
	cmsCode(t, c.call("GET", path, token, nil), 403)
	cmsCode(t, c.call("PUT", path, token, current), 403)
	if _, typ, _ := c.image(ids[0], token); strings.HasPrefix(typ, "image/") {
		t.Fatal("list permission exposed image")
	}
	// Deleted rows no longer contribute to aggregate size.
	current = cmsData(c.ok("GET", path, nil))
	c.ok("DELETE", path, map[string]any{"version": current["version"]})
	if d := cmsData(c.ok("GET", "/cms/asset/statistics?name="+url.QueryEscape(prefix), nil)); d["total"] != float64(1) {
		t.Fatal("deleted asset counted")
	}
	t.Log("PASS group upload/edit/CAS, optional-field compatibility, immutable image bytes, combined filters, literal wildcard, aggregate bytes and list-only permissions")
}
