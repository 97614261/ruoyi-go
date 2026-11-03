package portaltest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalInquiryFlow(t *testing.T) {
	c := cmsSetup(t)
	prefix := "inquiry-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var id int64
	t.Cleanup(func() {
		if id > 0 {
			out := c.call("GET", fmt.Sprintf("/cms/inquiry/%d", id), c.admin, nil)
			if out["code"] == float64(200) {
				c.ok("DELETE", fmt.Sprintf("/cms/inquiry/%d", id), map[string]any{"version": cmsData(out)["version"]})
			}
		}
	})
	stamp := time.Now().UnixNano()
	testIP := fmt.Sprintf("127.77.%d.%d", (stamp>>8)&255, stamp&255)
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(testIP)}}).DialContext}
	defer transport.CloseIdleConnections()
	anonymous := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	ipHash := fmt.Sprintf("%x", sha256.Sum256([]byte(testIP)))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if e := c.cache.Del(ctx, "portal_inquiry:form:"+ipHash, "portal_inquiry:submit:"+ipHash).Err(); e != nil {
			t.Error("test IP counter cleanup failed")
		}
	})
	res, e := anonymous.Get(portalTestURL() + "/inquiry")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(b), `form-action 'self'`) && !strings.Contains(res.Header.Get("Content-Security-Policy"), "form-action 'self'") {
		t.Fatal("form/CSP unavailable")
	}
	m := regexp.MustCompile(`name="token" value="([a-f0-9]{64})"`).FindSubmatch(b)
	if len(m) != 2 {
		t.Fatal("proof missing")
	}
	var cookie *http.Cookie
	for _, v := range res.Cookies() {
		if v.Name == "portal_inquiry" {
			cookie = v
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("proof cookie unsafe")
	}
	values := url.Values{"name": {prefix}, "contact": {"private-test@example.invalid"}, "message": {"<script>alert(1)</script>测试留言"}, "consent": {"yes"}, "token": {string(m[1])}}
	client := anonymous
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	submit := func(v url.Values, origin string, withCookie bool) int {
		req, _ := http.NewRequest("POST", portalTestURL()+"/inquiry", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if withCookie {
			req.AddCookie(cookie)
		}
		r, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		return r.StatusCode
	}
	if submit(values, "https://evil.invalid", true) != 403 {
		t.Fatal("cross-origin accepted")
	}
	if submit(values, "", false) != 400 {
		t.Fatal("missing cookie accepted")
	}
	values.Set("website", "spam")
	if submit(values, "", true) != 400 {
		t.Fatal("honeypot accepted")
	}
	values.Set("website", "")
	time.Sleep(2100 * time.Millisecond)
	if submit(values, portalTestURL()+"", true) != 303 {
		t.Fatal("valid submit failed")
	}
	if submit(values, "", true) != 400 {
		t.Fatal("replayed proof accepted")
	}
	if e = c.db.QueryRow("SELECT inquiry_id FROM cms_inquiry WHERE name=? AND is_deleted=0", prefix).Scan(&id); e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/cms/inquiry/%d", id)
	cmsCode(t, c.call("GET", path, "", nil), 401)
	out := c.ok("GET", "/cms/inquiry/list?pageSize=100", nil)
	found := false
	for _, item := range out["rows"].([]any) {
		row := item.(map[string]any)
		if cmsID(row, "inquiryId") == id {
			found = true
			if row["contact"] == values.Get("contact") || row["message"] != nil || row["note"] != nil {
				t.Fatal("list leaked private detail")
			}
		}
	}
	if !found {
		t.Fatal("inquiry missing")
	}
	detail := cmsData(c.ok("GET", path, nil))
	if detail["message"] != values.Get("message") {
		t.Fatal("plain text lost")
	}
	c.ok("PUT", path, map[string]any{"version": detail["version"], "status": "DONE", "note": "已联系"})
	cmsCode(t, c.call("PUT", path, c.admin, map[string]any{"version": detail["version"], "status": "SPAM"}), 500)
	// A list-only role must not fetch details, edit, or erase. No pre-existing role is modified.
	roleKey := "inq_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var menu, rid, uid int64
	if e = c.db.QueryRow("SELECT menu_id FROM sys_menu WHERE perms='cms:inquiry:list'").Scan(&menu); e != nil {
		t.Fatal(e)
	}
	role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{menu}, "menuCheckStrictly": true, "deptCheckStrictly": true}
	c.ok("POST", "/system/role", role)
	if e = c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	role["roleId"] = rid
	name := "iq" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		if uid > 0 {
			c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": name, "nickName": "询盘测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		}
		role["status"] = "1"
		role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", role)
	})
	c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "询盘列表测试", "password": "Inquiry_test_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
	if e = c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&uid); e != nil {
		t.Fatal(e)
	}
	token := c.login(name, "Inquiry_test_975!")
	defer c.call("POST", "/logout", token, nil)
	cmsCode(t, c.call("GET", "/cms/inquiry/list", token, nil), 200)
	cmsCode(t, c.call("GET", path, token, nil), 403)
	cmsCode(t, c.call("PUT", path, token, map[string]any{"version": 2, "status": "SPAM"}), 403)
	cmsCode(t, c.call("DELETE", path, token, map[string]any{"version": 2}), 403)
	c.ok("DELETE", path, map[string]any{"version": 2})
	cmsCode(t, c.call("GET", path, c.admin, nil), 500)
	var n int
	if e = c.db.QueryRow("SELECT COUNT(*) FROM cms_inquiry WHERE inquiry_id=? AND is_deleted=1 AND name='' AND contact='' AND message='' AND note=''", id).Scan(&n); e != nil || n != 1 {
		t.Fatal("erase retained personal data", e)
	}
	var audit string
	for deadline := time.Now().Add(3 * time.Second); ; {
		e = c.db.QueryRow("SELECT CONCAT(IFNULL(oper_param,''),IFNULL(json_result,'')) FROM sys_oper_log WHERE oper_url=? ORDER BY oper_id DESC LIMIT 1", path).Scan(&audit)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("audit missing")
		}
		time.Sleep(30 * time.Millisecond)
	}
	if strings.Contains(audit, values.Get("message")) || strings.Contains(audit, values.Get("contact")) {
		t.Fatal("inquiry leaked to audit")
	}
	proof, _ := json.Marshal(map[string]any{"id": id, "maskedList": true, "cas": true, "permissions": true, "erased": true})
	t.Logf("PASS %s", proof)
}

func TestInquiryGUICleanup(t *testing.T) {
	if os.Getenv("PORTAL_INQUIRY_GUI_ID") != "2" {
		t.Skip("explicit owned GUI fixture only")
	}
	c := cmsSetup(t)
	row := cmsData(c.ok("GET", "/cms/inquiry/2", nil))
	if row["name"] != "inquiry-ui-1793581858923" {
		t.Fatal("GUI fixture ownership mismatch")
	}
	if row["status"] != "DONE" || row["note"] != "本轮UI验收：已联系测试客户" {
		t.Fatal("GUI processing not persisted")
	}
	c.ok("DELETE", "/cms/inquiry/2", map[string]any{"version": row["version"]})
	t.Log("PASS GUI processing persisted; only owned fixture erased")
}
