package portaltest

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestPortalGrowthPermissions(t *testing.T) {
	c := cmsSetup(t)
	key := "gperm" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var userID, roleID, channel, article, pageID int64
	var menuIDs []int64
	role := map[string]any{"roleName": key, "roleKey": key, "roleSort": 1, "status": "0", "dataScope": "1", "menuCheckStrictly": true, "deptCheckStrictly": true}
	t.Cleanup(func() {
		if userID > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": userID, "userName": key, "nickName": "门户权限测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		if roleID > 0 {
			role["roleId"] = roleID
			role["status"] = "1"
			role["menuIds"] = []int64{}
			c.ok("PUT", "/system/role", role)
		}
		for _, v := range []struct {
			k  string
			id int64
		}{{"article", article}, {"page", pageID}} {
			if v.id > 0 {
				p := fmt.Sprintf("/cms/%s/%d", v.k, v.id)
				d := cmsData(c.ok("GET", p, nil))
				c.ok("DELETE", p, map[string]any{"version": d["version"]})
			}
		}
		if channel > 0 {
			d := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", channel), nil))
			d["status"] = "1"
			c.ok("PUT", "/cms/channel", d)
		}
	})
	rs, e := c.db.Query("SELECT menu_id FROM sys_menu WHERE perms IN ('cms:article:list','cms:article:query','cms:article:edit','cms:tag:list','cms:tag:edit')")
	if e != nil {
		t.Fatal(e)
	}
	for rs.Next() {
		var id int64
		if e = rs.Scan(&id); e != nil {
			t.Fatal(e)
		}
		menuIDs = append(menuIDs, id)
	}
	rs.Close()
	role["menuIds"] = menuIDs
	c.ok("POST", "/system/role", role)
	if e = c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", key).Scan(&roleID); e != nil {
		t.Fatal(e)
	}
	c.ok("POST", "/system/user", map[string]any{"userName": key, "nickName": "门户有限编辑", "password": "Growth_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{roleID}, "postIds": []int64{}})
	if e = c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", key).Scan(&userID); e != nil {
		t.Fatal(e)
	}
	token := c.login(key, "Growth_test_975!")
	defer c.call("POST", "/logout", token, nil)
	channel = cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": key, "slug": key, "kind": "ARTICLE", "status": "0"})), "channelId")
	article = cmsID(cmsData(c.ok("POST", "/cms/article", map[string]any{"title": key, "slug": key, "channelId": channel, "bodyHtml": "<p>权限测试</p>"})), "contentId")
	pageID = cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"title": key, "slug": key, "bodyHtml": "<p>不同模块</p>"})), "contentId")
	path := fmt.Sprintf("/cms/article/%d/operations", article)
	v := c.call("GET", path, token, nil)
	cmsCode(t, v, 200)
	cmsCode(t, c.call("PUT", path, token, map[string]any{"version": cmsData(v)["version"], "settings": map[string]any{"featured": true, "pinOrder": 1}}), 200)
	v = c.call("GET", path, token, nil)
	cmsCode(t, c.call("PUT", path, token, map[string]any{"version": cmsData(v)["version"], "settings": map[string]any{"shareAssetId": 999999}}), 403)
	cmsCode(t, c.call("GET", "/cms/tag/list", token, nil), 200)
	cmsCode(t, c.call("POST", "/cms/tag", token, map[string]any{"name": key, "status": "0"}), 403)
	cmsCode(t, c.call("PUT", "/cms/tag", token, map[string]any{"name": key, "status": "0", "tagId": 0}), 500)
	cmsCode(t, c.call("GET", "/cms/statistics/list", token, nil), 403)
	cmsCode(t, c.call("GET", fmt.Sprintf("/cms/page/%d/operations", pageID), token, nil), 403)
	cmsCode(t, c.call("GET", "/cms/article/trash", token, nil), 403)
	role["roleId"] = roleID
	role["menuIds"] = []int64{}
	c.ok("PUT", "/system/role", role)
	cmsCode(t, c.call("GET", path, token, nil), 403)
	t.Log("PASS source-module permissions, independent tag/statistics grants, asset denial and live revocation")
}
