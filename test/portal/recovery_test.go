package portaltest

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalPersonalRecovery(t *testing.T) {
	admin := cmsSetup(t)
	actorKey := "rx" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var actorRole, actorID int64
	actorMenus := []int64{}
	mr, e := admin.db.Query("SELECT menu_id FROM sys_menu WHERE perms IN ('cms:page:list','cms:page:query','cms:page:add','cms:page:edit','cms:page:publish','cms:page:remove','cms:asset:list','cms:asset:query','cms:asset:upload','cms:asset:edit','cms:asset:remove','cms:channel:query')")
	if e != nil {
		t.Fatal(e)
	}
	for mr.Next() {
		var n int64
		if e = mr.Scan(&n); e != nil {
			t.Fatal(e)
		}
		actorMenus = append(actorMenus, n)
	}
	mr.Close()
	ar := map[string]any{"roleName": actorKey, "roleKey": actorKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": actorMenus, "menuCheckStrictly": true, "deptCheckStrictly": true}
	admin.ok("POST", "/system/role", ar)
	if e = admin.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", actorKey).Scan(&actorRole); e != nil {
		t.Fatal(e)
	}
	ar["roleId"] = actorRole
	admin.ok("POST", "/system/user", map[string]any{"userName": actorKey, "nickName": "恢复副本专用测试", "password": "Recovery_actor_975!", "deptId": 103, "status": "0", "roleIds": []int64{actorRole}, "postIds": []int64{}})
	if e = admin.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", actorKey).Scan(&actorID); e != nil {
		t.Fatal(e)
	}
	actorToken := admin.login(actorKey, "Recovery_actor_975!")
	actorClient := *admin
	actorClient.admin = actorToken
	c := &actorClient
	t.Cleanup(func() {
		expiryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e := admin.db.ExecContext(expiryCtx, "UPDATE cms_content_recovery SET expires_at=DATE_SUB(NOW(),INTERVAL 8 DAY) WHERE user_id=?", actorID); e != nil {
			t.Error(e)
		}
		t.Logf("expiry fixture actor=%d", actorID)
		admin.call("POST", "/logout", actorToken, nil)
		admin.ok("PUT", "/system/user", map[string]any{"userId": actorID, "userName": actorKey, "nickName": "恢复测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		ar["status"] = "1"
		ar["menuIds"] = []int64{}
		admin.ok("PUT", "/system/role", ar)
	})
	prefix := "recovery-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var pages []int64
	tokenOf := func(v map[string]any) map[string]any {
		return map[string]any{"slotId": v["slotId"], "version": v["version"]}
	}
	cachePath := func(id int64) string { return fmt.Sprintf("/cms/page/%d/autosave", id) }
	cleanup := func(id int64) {
		path := cachePath(id)
		v := cmsData(c.ok("GET", path, nil))
		c.ok("DELETE", path, tokenOf(v))
	}
	t.Cleanup(func() {
		cleanup(0)
		for _, id := range pages {
			cleanup(id)
			p := fmt.Sprintf("/cms/page/%d", id)
			d := c.call("GET", p, c.admin, nil)
			if d["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(d)["version"]})
			}
		}
	})
	makePage := func(slug string) int64 {
		id := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": slug, "title": "正式草稿", "bodyHtml": "<p>正式正文</p>"})), "contentId")
		pages = append(pages, id)
		return id
	}
	id := makePage(prefix)
	path := cachePath(id)
	officialPath := fmt.Sprintf("/cms/page/%d", id)
	official := cmsData(c.ok("GET", officialPath, nil))
	base := official["version"]
	put := func(path, token string, v map[string]any, base any, draft map[string]any) map[string]any {
		in := tokenOf(v)
		in["baseVersion"] = base
		in["draft"] = draft
		return c.call("PUT", path, token, in)
	}
	initial := cmsData(c.ok("GET", path, nil))
	if initial["found"] != false {
		t.Fatal("new own slot not empty")
	}
	draft := map[string]any{"title": "个人副本", "slug": prefix, "bodyHtml": "<p>未提交正文</p>"}
	saved := put(path, c.admin, initial, base, draft)
	cmsCode(t, saved, 200)
	v := cmsData(saved)
	current := cmsData(c.ok("GET", officialPath, nil))
	if current["version"] != base || current["workingRevisionId"] != official["workingRevisionId"] {
		t.Fatal("autosave mutated official draft")
	}
	cmsCode(t, put(path, c.admin, initial, base, draft), 409)
	if _, body, _ := c.public("/p/"+prefix, ""); strings.Contains(body, "未提交正文") {
		t.Fatal("personal recovery leaked online")
	}
	// Explicit manual commit validates fully and atomically clears only this slot.
	in := tokenOf(v)
	in["baseVersion"] = base
	in["draft"] = map[string]any{"title": "手工确认", "slug": prefix, "bodyHtml": "<p>手工确认正文</p>"}
	c.ok("POST", path+"/commit", in)
	empty := cmsData(c.ok("GET", path, nil))
	if empty["found"] != false || empty["version"] == v["version"] {
		t.Fatal("manual commit did not leave a versioned empty slot")
	}
	cmsCode(t, put(path, c.admin, v, base, draft), 409)
	current = cmsData(c.ok("GET", officialPath, nil))
	newBase := current["version"]
	// Partial autosave does not require the full form or nonempty body.
	partial := put(path, c.admin, empty, newBase, map[string]any{"bodyHtml": "<p><br></p>"})
	cmsCode(t, partial, 200)
	pv := cmsData(partial)
	expiryCtx, cancelExpiry := context.WithTimeout(context.Background(), 3*time.Second)
	_, expiryErr := c.db.ExecContext(expiryCtx, "UPDATE cms_content_recovery SET expires_at=DATE_SUB(NOW(),INTERVAL 8 DAY) WHERE recovery_id=? AND user_id=?", pv["slotId"], actorID)
	cancelExpiry()
	if expiryErr != nil {
		t.Fatal(expiryErr)
	}
	expired := cmsData(c.ok("GET", path, nil))
	if expired["found"] != false || expired["draft"] != nil {
		t.Fatal("expired payload must not be returned")
	}
	partial = put(path, c.admin, expired, newBase, map[string]any{"bodyHtml": "<p><br></p>"})
	cmsCode(t, partial, 200)
	pv = cmsData(partial)
	invalid := tokenOf(pv)
	invalid["baseVersion"] = newBase
	invalid["draft"] = map[string]any{"slug": prefix, "title": "", "bodyHtml": ""}
	cmsCode(t, c.call("POST", path+"/commit", c.admin, invalid), 500)
	if d := cmsData(c.ok("GET", path, nil)); d["version"] != pv["version"] || d["found"] != true {
		t.Fatal("failed commit damaged recovery")
	}
	// An original explicit save or historical restore makes old autosaves stale, not destructive.
	data := current["draft"].(map[string]any)
	data["contentId"] = id
	data["version"] = newBase
	data["slug"] = prefix
	data["title"] = "另一编辑的新稿"
	c.ok("PUT", "/cms/page", data)
	cmsCode(t, put(path, c.admin, pv, newBase, draft), 409)
	stale := cmsData(c.ok("GET", path, nil))
	if stale["found"] != true || stale["baseMatches"] != false {
		t.Fatal("stale copy must remain inspectable")
	}
	c.ok("DELETE", path, tokenOf(stale))
	fresh := cmsData(c.ok("GET", path, nil))
	cur := cmsData(c.ok("GET", officialPath, nil))
	bv := cur["version"]
	resumed := put(path, c.admin, fresh, bv, draft)
	cmsCode(t, resumed, 200)
	rv := cmsData(resumed)
	history := c.ok("GET", officialPath+"/history?pageSize=100", nil)
	first := history["rows"].([]any)[0].(map[string]any)
	c.ok("POST", officialPath+"/restore", map[string]any{"version": bv, "revisionId": first["revisionId"]})
	cmsCode(t, put(path, c.admin, rv, bv, draft), 409)
	// New form tombstones reject delayed writes after successful creation.
	blank := cmsData(c.ok("GET", cachePath(0), nil))
	c.ok("DELETE", cachePath(0), tokenOf(blank))
	blank = cmsData(c.ok("GET", cachePath(0), nil))
	nd := map[string]any{"title": "自动新稿", "slug": "new-" + prefix, "bodyHtml": "<p>新表单</p>"}
	nvOut := put(cachePath(0), c.admin, blank, 0, nd)
	cmsCode(t, nvOut, 200)
	nv := cmsData(nvOut)
	commit := tokenOf(nv)
	commit["baseVersion"] = 0
	commit["draft"] = nd
	created := cmsData(c.ok("POST", cachePath(0)+"/commit", commit))
	pages = append(pages, cmsID(created, "contentId"))
	cmsCode(t, put(cachePath(0), c.admin, nv, 0, nd), 409)
	// Recovery references protect private files until clear/expiry, without changing their bytes.
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 24, 18)))
	img := c.upload(prefix+".png", buf.Bytes())
	cmsCode(t, img, 200)
	asset := cmsData(img)
	assetId := cmsID(asset, "assetId")
	assetPath := fmt.Sprintf("/cms/asset/%d", assetId)
	t.Cleanup(func() {
		cleanup(0)
		d := c.call("GET", assetPath, c.admin, nil)
		if d["code"] == float64(200) {
			c.call("DELETE", assetPath, c.admin, map[string]any{"version": cmsData(d)["version"]})
		}
	})
	slot := cmsData(c.ok("GET", cachePath(0), nil))
	withImage := put(cachePath(0), c.admin, slot, 0, map[string]any{"bodyHtml": fmt.Sprintf(`<p>图</p><img src="/media/%d">`, assetId)})
	cmsCode(t, withImage, 200)
	cmsCode(t, c.call("DELETE", assetPath, c.admin, map[string]any{"version": asset["version"]}), 500)
	refs := c.ok("GET", assetPath+"/references", nil)
	if refs["total"] != float64(1) || !strings.Contains(refs["rows"].([]any)[0].(map[string]any)["ownerName"].(string), "自动保存") {
		t.Fatal("recovery reference missing or mislabeled")
	}
	c.ok("DELETE", cachePath(0), tokenOf(cmsData(withImage)))
	c.ok("DELETE", assetPath, map[string]any{"version": asset["version"]})
	// Ownership comes from the authenticated user; read-only and revoked editors are denied.
	key := "rc" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var roleId, userId int64
	var menuIds []int64
	rs, e := c.db.Query("SELECT menu_id FROM sys_menu WHERE perms IN ('cms:page:list','cms:page:query','cms:page:edit','cms:page:add')")
	if e != nil {
		t.Fatal(e)
	}
	for rs.Next() {
		var x int64
		if e = rs.Scan(&x); e != nil {
			t.Fatal(e)
		}
		menuIds = append(menuIds, x)
	}
	rs.Close()
	role := map[string]any{"roleName": key, "roleKey": key, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": menuIds, "menuCheckStrictly": true, "deptCheckStrictly": true}
	admin.ok("POST", "/system/role", role)
	if e = c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", key).Scan(&roleId); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = roleId
	t.Cleanup(func() {
		if userId > 0 {
			admin.ok("PUT", "/system/user", map[string]any{"userId": userId, "userName": key, "nickName": "恢复测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		admin.ok("PUT", "/system/role", role)
	})
	admin.ok("POST", "/system/user", map[string]any{"userName": key, "nickName": "个人恢复测试", "password": "Recovery_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{roleId}, "postIds": []int64{}})
	if e = c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", key).Scan(&userId); e != nil {
		t.Fatal(e)
	}
	token := c.login(key, "Recovery_test_975!")
	defer c.call("POST", "/logout", token, nil)
	own := c.call("GET", path+"?userId="+strconv.FormatInt(actorID, 10), token, nil)
	cmsCode(t, own, 200)
	if cmsData(own)["found"] != false {
		t.Fatal("another user's cache exposed")
	}
	cur = cmsData(c.ok("GET", officialPath, nil))
	other := put(path, token, cmsData(own), cur["version"], draft)
	cmsCode(t, other, 200)
	forged := put(path, token, rv, cur["version"], draft)
	cmsCode(t, forged, 409)
	// No asset query permission can create a private-image reference.
	otherSlot := cmsData(other)
	cmsCode(t, put(path, token, otherSlot, cur["version"], map[string]any{"coverAssetId": 999999}), 403)
	cmsCode(t, c.call("GET", path, "", nil), 401)
	c.call("DELETE", path, token, tokenOf(otherSlot))
	role["menuIds"] = []int64{}
	admin.ok("PUT", "/system/role", role)
	cmsCode(t, c.call("GET", path, token, nil), 403)
	t.Log("PASS isolated recovery, independent slot/content CAS, partial validation, manual commit tombstone, stale normal-save/history rejection, new-form delayed-write protection, image references and revocation")
}
