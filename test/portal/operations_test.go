package portaltest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/xuri/excelize/v2"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These fixtures belong only to this run. Cleanup previews always supply explicit
// IDs so a developer's unrelated old inquiry can never enter an erasure batch.
func TestPortalOperations(t *testing.T) {
	c := cmsSetup(t)
	key := "op" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	policy := cmsData(c.ok("GET", "/cms/data/policy", nil))
	workflow := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
	setWorkflow := func(isolation bool) {
		s := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
		required := map[string]bool{}
		for _, v := range workflow["policies"].([]any) {
			p := v.(map[string]any)
			required[p["kind"].(string)] = p["required"].(bool)
		}
		c.ok("PUT", "/cms/workflow/settings", map[string]any{"version": s["version"], "required": required, "inquiryIsolation": isolation, "managerIds": workflow["managerIds"], "reason": key + " isolation fixture"})
	}
	type actor struct {
		id          int64
		name, token string
		role        map[string]any
	}
	actors := []actor{}
	contents := []int64{}
	inquiries := []int64{}
	t.Cleanup(func() {
		p := cmsData(c.ok("GET", "/cms/data/policy", nil))
		c.ok("PUT", "/cms/data/policy", map[string]any{"version": p["version"], "retentionDays": policy["retentionDays"], "statuses": policy["statuses"], "reason": key + " restore original retention"})
		setWorkflow(workflow["inquiryIsolation"].(bool))
		for _, id := range inquiries {
			path := fmt.Sprintf("/cms/inquiry/%d", id)
			r := c.call("GET", path, c.admin, nil)
			if r["code"] == float64(200) {
				c.ok("DELETE", path, map[string]any{"version": cmsData(r)["version"]})
			}
		}
		for _, id := range contents {
			path := fmt.Sprintf("/cms/page/%d", id)
			r := c.call("GET", path, c.admin, nil)
			if r["code"] == float64(200) {
				c.ok("DELETE", path, map[string]any{"version": cmsData(r)["version"]})
			}
		}
		for _, a := range actors {
			c.call("POST", "/logout", a.token, nil)
			c.ok("PUT", "/system/user", map[string]any{"userId": a.id, "userName": a.name, "nickName": "运营测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
			a.role["status"] = "1"
			a.role["menuIds"] = []int64{}
			c.ok("PUT", "/system/role", a.role)
		}
	})
	makeActor := func(suffix string, perms ...string) actor {
		name := key + suffix
		marks := []string{}
		args := []any{}
		for _, p := range perms {
			marks = append(marks, "?")
			args = append(args, p)
		}
		rows, e := c.db.QueryContext(ctx, "SELECT menu_id FROM sys_menu WHERE perms IN ("+strings.Join(marks, ",")+")", args...)
		if e != nil {
			t.Fatal(e)
		}
		ids := []int64{}
		for rows.Next() {
			var id int64
			if e = rows.Scan(&id); e != nil {
				t.Fatal(e)
			}
			ids = append(ids, id)
		}
		rows.Close()
		role := map[string]any{"roleName": name, "roleKey": name, "roleSort": 1, "status": "0", "dataScope": "1", "menuCheckStrictly": true, "deptCheckStrictly": true, "menuIds": ids}
		c.ok("POST", "/system/role", role)
		var rid, id int64
		if e = c.db.QueryRowContext(ctx, "SELECT role_id FROM sys_role WHERE role_key=?", name).Scan(&rid); e != nil {
			t.Fatal(e)
		}
		role["roleId"] = rid
		c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "运营专用" + suffix, "password": "Operations_975!", "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
		if e = c.db.QueryRowContext(ctx, "SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&id); e != nil {
			t.Fatal(e)
		}
		a := actor{id, name, c.login(name, "Operations_975!"), role}
		actors = append(actors, a)
		return a
	}
	list := makeActor("l", "cms:inquiry:list", "cms:inquiry:export")
	private := makeActor("p", "cms:inquiry:list", "cms:inquiry:query", "cms:inquiry:edit", "cms:inquiry:export", "cms:inquiry:exportPrivate")
	restricted := makeActor("r", "cms:page:list")
	manager := makeActor("m", "cms:data:list", "cms:data:cleanup", "cms:data:audit", "cms:inquiry:list", "cms:inquiry:query", "cms:inquiry:all", "cms:inquiry:remove")
	setWorkflow(true)
	insertInquiry := func(status string, owner int64) int64 {
		r, e := c.db.ExecContext(ctx, `INSERT INTO cms_inquiry(name,contact,message,status,note,version,is_deleted,create_by,create_time,update_by,update_time,remark) VALUES(?,?,?,?,?,1,0,?,'2000-01-01 00:00:00','','2000-01-01 00:00:00','')`, key, "private-"+key+"@example.invalid", "=FORMULA_PRIVATE_"+key, status, "private note "+key, key)
		if e != nil {
			t.Fatal(e)
		}
		id, e := r.LastInsertId()
		if e != nil {
			t.Fatal(e)
		}
		inquiries = append(inquiries, id)
		_, e = c.db.ExecContext(ctx, `INSERT INTO cms_inquiry_team(inquiry_id,owner_user_id,version,create_by,create_time,update_by,remark) VALUES(?,?,1,?,NOW(),'','')`, id, owner, key)
		if e != nil {
			t.Fatal(e)
		}
		_, e = c.db.ExecContext(ctx, `INSERT INTO cms_inquiry_clock(inquiry_id,submitted_at,active_at,status,is_erased,source_version,create_by,create_time) VALUES(?,'2000-01-01 00:00:00','2000-01-01 00:00:00',?,0,1,?,NOW())`, id, status, key)
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	download := func(token string, body any) ([]byte, int) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", portalTestURL()+"/cms/inquiry/export", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		r, e := c.http.Do(req)
		if e != nil {
			t.Fatal("export request failed")
		}
		defer r.Body.Close()
		out, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(r.Header.Get("Content-Type"), "json") {
			var msg map[string]any
			if e = json.Unmarshal(out, &msg); e != nil {
				t.Fatal(e)
			}
			return out, int(msg["code"].(float64))
		}
		if !strings.Contains(r.Header.Get("Cache-Control"), "no-store") || !strings.Contains(r.Header.Get("Content-Disposition"), "attachment") {
			t.Fatal("export download headers")
		}
		return out, 200
	}
	sheet := func(b []byte) [][]string {
		f, e := excelize.OpenReader(bytes.NewReader(b))
		if e != nil {
			t.Fatal("invalid XLSX")
		}
		defer f.Close()
		r, e := f.GetRows(f.GetSheetName(0))
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	t.Run("ExportPermissionsScopeAndAudit", func(t *testing.T) {
		mine := insertInquiry("NEW", list.id)
		other := insertInquiry("NEW", private.id)
		b, code := download(list.token, map[string]any{"from": "2000-01-01", "to": "2000-01-01"})
		if code != 200 {
			t.Fatal("masked export", code)
		}
		rows := sheet(b)
		if len(rows) != 2 || rows[1][0] != strconv.FormatInt(mine, 10) {
			t.Fatal("export owner scope", len(rows))
		}
		if strings.Contains(strings.Join(rows[1], "|"), "PRIVATE_") || strings.Contains(strings.Join(rows[1], "|"), "@example.invalid") {
			t.Fatal("masked export leaked private fields")
		}
		var digest, details string
		if e := c.db.QueryRowContext(ctx, "SELECT file_sha256,details FROM cms_data_event WHERE actor_user_id=? AND action='EXPORT' ORDER BY event_id DESC LIMIT 1", list.id).Scan(&digest, &details); e != nil {
			t.Fatal(e)
		}
		if digest != fmt.Sprintf("%x", sha256.Sum256(b)) || strings.Contains(details, "private-") {
			t.Fatal("export audit digest/privacy")
		}
		_, code = download(list.token, map[string]any{"includePrivate": true})
		if code != 403 {
			t.Fatal("private permission not enforced", code)
		}
		b, code = download(private.token, map[string]any{"includePrivate": true, "ownerUserId": private.id})
		if code != 200 {
			t.Fatal("private export", code)
		}
		rows = sheet(b)
		if len(rows) != 2 || rows[1][0] != strconv.FormatInt(other, 10) || !strings.Contains(strings.Join(rows[1], "|"), "FORMULA_PRIVATE_") {
			t.Fatal("private fields missing")
		}
		f, e := excelize.OpenReader(bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		formula, e := f.GetCellFormula(f.GetSheetName(0), "H2")
		f.Close()
		if e != nil || formula != "" {
			t.Fatal("customer text became Excel formula")
		}
		d := cmsData(c.ok("GET", fmt.Sprintf("/cms/inquiry/%d", other), nil))
		c.ok("POST", fmt.Sprintf("/cms/inquiry/%d/assign", other), map[string]any{"version": d["version"], "ownerUserId": 1})
		b, code = download(private.token, map[string]any{"includePrivate": true})
		if code != 200 || len(sheet(b)) != 1 {
			t.Fatal("reassigned owner retained export data")
		}
		b, code = download(list.token, map[string]any{"from": "2099-01-01"})
		if code != 200 || len(sheet(b)) != 1 {
			t.Fatal("date filter lost")
		}
		for _, body := range []any{map[string]any{"from": "2000-99-01"}, map[string]any{"from": "2001-01-01", "to": "2000-01-01"}} {
			_, code = download(list.token, body)
			if code != 500 {
				t.Fatal("invalid date accepted", code)
			}
		}
		cmsCode(t, c.call("GET", "/cms/data/events", list.token, nil), 403)
		cmsCode(t, c.call("GET", "/cms/data/policy", restricted.token, nil), 403)
		list.role["menuIds"] = []int64{}
		c.ok("PUT", "/system/role", list.role)
		_, code = download(list.token, map[string]any{})
		if code != 403 {
			t.Fatal("revoked export permission remained valid", code)
		}
	})
	t.Run("CleanupSnapshotAtomicity", func(t *testing.T) {
		p := cmsData(c.ok("GET", "/cms/data/policy", nil))
		c.ok("PUT", "/cms/data/policy", map[string]any{"version": p["version"], "retentionDays": 1, "statuses": []string{"DONE"}, "reason": key + " test policy"})
		cmsCode(t, c.call("PUT", "/cms/data/policy", c.admin, map[string]any{"version": p["version"], "retentionDays": 1, "statuses": []string{"DONE"}, "reason": "stale"}), 500)
		a, b := insertInquiry("DONE", private.id), insertInquiry("DONE", private.id)
		if _, e := c.db.ExecContext(ctx, "UPDATE cms_inquiry SET contact='12' WHERE inquiry_id=?", b); e != nil {
			t.Fatal(e)
		}
		if _, e := c.db.ExecContext(ctx, `INSERT INTO cms_inquiry_followup(inquiry_id,actor_user_id,event_type,text,result,is_erased,create_by,create_time,update_by,remark) VALUES(?,?,'FOLLOWUP','private bulk text','private bulk result',0,?,NOW(),'','')`, b, private.id, key); e != nil {
			t.Fatal(e)
		}

		preview := func(ids []int64) map[string]any {
			return cmsData(c.ok("POST", "/cms/data/cleanup/preview", map[string]any{"reason": key + " only test IDs", "inquiryIds": ids}))
		}
		batch := preview([]int64{a, b})
		if cmsID(batch, "previewCount") != 2 || cmsID(batch, "totalEligible") != 2 {
			t.Fatal("selected preview did not match fixtures")
		}
		bid := cmsID(batch, "batchId")
		rows := c.ok("GET", fmt.Sprintf("/cms/data/cleanup/%d/items", bid), nil)["rows"].([]any)
		for _, v := range rows {
			row := v.(map[string]any)
			if cmsID(row, "inquiryId") == b && row["contact"] != "***" {
				t.Fatal("short contact revealed")
			}
		}

		path := fmt.Sprintf("/cms/data/cleanup/%d/execute", bid)
		var deleted int
		if e := c.db.QueryRowContext(ctx, "SELECT SUM(is_deleted) FROM cms_inquiry WHERE inquiry_id IN (?,?)", a, b).Scan(&deleted); e != nil || deleted != 0 {
			t.Fatal("preview mutated inquiry")
		}
		cmsCode(t, c.call("POST", path, manager.token, map[string]any{"version": batch["version"], "confirmation": "确认清除"}), 500)
		c.ok("PUT", fmt.Sprintf("/cms/inquiry/%d", a), map[string]any{"version": 1, "status": "DONE", "note": "new activity after preview"})
		cmsCode(t, c.call("POST", path, c.admin, map[string]any{"version": batch["version"], "confirmation": "确认清除"}), 500)
		if e := c.db.QueryRowContext(ctx, "SELECT is_deleted FROM cms_inquiry WHERE inquiry_id=?", b).Scan(&deleted); e != nil || deleted != 0 {
			t.Fatal("partial cleanup on stale batch")
		}
		batch = preview([]int64{b})
		bid = cmsID(batch, "batchId")
		path = fmt.Sprintf("/cms/data/cleanup/%d/execute", bid)
		p = cmsData(c.ok("GET", "/cms/data/policy", nil))
		c.ok("PUT", "/cms/data/policy", map[string]any{"version": p["version"], "retentionDays": 2, "statuses": []string{"DONE"}, "reason": key + " changed policy"})
		cmsCode(t, c.call("POST", path, c.admin, map[string]any{"version": batch["version"], "confirmation": "确认清除"}), 500)
		batch = preview([]int64{b})
		bid = cmsID(batch, "batchId")
		_, e := c.db.ExecContext(ctx, "UPDATE cms_data_cleanup_batch SET expires_at=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE batch_id=? AND actor_user_id=1", bid)
		if e != nil {
			t.Fatal(e)
		}
		cmsCode(t, c.call("POST", fmt.Sprintf("/cms/data/cleanup/%d/execute", bid), c.admin, map[string]any{"version": batch["version"], "confirmation": "确认清除"}), 500)
		batch = preview([]int64{b})
		bid = cmsID(batch, "batchId")
		path = fmt.Sprintf("/cms/data/cleanup/%d/execute", bid)
		cmsCode(t, c.call("POST", path, c.admin, map[string]any{"version": batch["version"], "confirmation": "yes"}), 500)
		var wg sync.WaitGroup
		codes := make(chan float64, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := c.call("POST", path, c.admin, map[string]any{"version": batch["version"], "confirmation": "确认清除"})
				codes <- r["code"].(float64)
			}()
		}
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code != 500 {
				t.Fatal("unexpected concurrent code", code)
			}
		}
		if success != 1 {
			t.Fatal("cleanup was not exactly once", success)
		}
		var text string
		if e = c.db.QueryRowContext(ctx, "SELECT CONCAT(name,contact,message,note) FROM cms_inquiry WHERE inquiry_id=? AND is_deleted=1", b).Scan(&text); e != nil || text != "" {
			t.Fatal("cleanup did not erase personal fields", e)
		}
		var followText, followResult string
		if e = c.db.QueryRowContext(ctx, "SELECT text,result FROM cms_inquiry_followup WHERE inquiry_id=? AND is_erased=1", b).Scan(&followText, &followResult); e != nil || followText != "" || followResult != "" {
			t.Fatal("bulk followup erasure", e)
		}
		audit := c.ok("GET", "/cms/data/events?pageSize=1000", nil)
		if len(audit["rows"].([]any)) > 100 {
			t.Fatal("pagination cap lost")
		}
		cmsCode(t, c.call("POST", "/cms/data/cleanup/preview", private.token, map[string]any{"reason": "no permission", "inquiryIds": []int64{a}}), 403)
	})
	t.Run("ComparisonAndPublishedChineseSearch", func(t *testing.T) {
		makePage := func(title, body string) int64 {
			id := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": key + "-" + strconv.Itoa(len(contents)), "title": title, "bodyHtml": body})), "contentId")
			contents = append(contents, id)
			return id
		}
		get := func(id int64) map[string]any { return cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil)) }
		action := func(id int64, a string) {
			c.ok("POST", fmt.Sprintf("/cms/page/%d/%s", id, a), map[string]any{"version": get(id)["version"]})
		}
		save := func(id int64, body string) {
			d := get(id)
			r := d["draft"].(map[string]any)
			r["contentId"] = id
			r["version"] = d["version"]
			r["slug"] = d["slug"]
			r["bodyHtml"] = body
			c.ok("PUT", "/cms/page", r)
		}
		phrase := "中文仓库检索" + key
		id := makePage("普通标题"+key, "<p>安全 &lt;img onerror=alert(1)&gt; "+phrase+"</p>")
		draft := get(id)
		old := draft["draft"].(map[string]any)["revisionId"]
		action(id, "publish")
		rank := makePage(phrase, "<p>标题优先</p>")
		action(rank, "publish")
		waitIndex := func(ids ...int64) {
			deadline := time.Now().Add(50 * time.Second)
			for time.Now().Before(deadline) {
				ready := true
				for _, id := range ids {
					var s string
					if e := c.db.QueryRowContext(ctx, "SELECT status FROM cms_search_task WHERE content_id=?", id).Scan(&s); e != nil || s != "READY" {
						ready = false
					}
				}
				if ready {
					return
				}
				time.Sleep(500 * time.Millisecond)
			}
			t.Fatal("managed search worker did not finish")
		}
		waitIndex(id, rank)
		search := func(q string) string {
			status, b, _ := c.public("/search?q="+url.QueryEscape(q), "")
			if status != 200 {
				t.Fatal("search status", status)
			}
			return b
		}
		body := search(phrase)
		if !strings.Contains(search("检"), "/p/"+key+"-0") {
			t.Fatal("single Chinese character search missing")
		}
		if !strings.Contains(search("中文仓库 "+key), "/p/"+key+"-0") {
			t.Fatal("AND terms search missing")
		}

		first := strings.Index(body, "/p/"+key+"-1")
		second := strings.Index(body, "/p/"+key+"-0")
		if first < 0 || second < 0 || first > second {
			t.Fatal("Chinese full text/ranking missing")
		}
		if !strings.Contains(body, "<mark>") || strings.Contains(body, "<img onerror=") {
			t.Fatal("highlight missing or interpreted markup")
		}
		save(id, "<p>草稿秘密"+key+"</p>")
		d := get(id)
		compare := map[string]any{"kind": "PAGE", "objectId": id, "left": map[string]any{"source": "REVISION", "id": old}, "right": map[string]any{"source": "DRAFT", "version": d["version"]}}
		diff := cmsData(c.ok("POST", "/cms/compare", compare))
		if len(diff["notices"].([]any)) == 0 {
			t.Fatal("historical address limitation must be visible")
		}
		if cmsID(diff, "changedCount") < 1 {
			t.Fatal("no version difference")
		}
		if get(id)["version"] != d["version"] {
			t.Fatal("comparison mutated draft")
		}
		cmsCode(t, c.call("POST", "/cms/compare", restricted.token, compare), 403)
		compare["right"] = map[string]any{"source": "DRAFT", "version": 1}
		cmsCode(t, c.call("POST", "/cms/compare", c.admin, compare), 500)
		compare["right"] = map[string]any{"source": "REVISION", "id": get(rank)["draft"].(map[string]any)["revisionId"]}
		cmsCode(t, c.call("POST", "/cms/compare", c.admin, compare), 500)
		if strings.Contains(search("草稿秘密"+key), "/p/"+key+"-0") {
			t.Fatal("draft body entered search")
		}
		action(id, "publish")
		if strings.Contains(search(phrase), "/p/"+key+"-0") {
			t.Fatal("old published index leaked immediately after republish")
		}
		waitIndex(id)
		if !strings.Contains(search("草稿秘密"+key), "/p/"+key+"-0") {
			t.Fatal("new public body not indexed")
		}
		action(id, "unpublish")
		if strings.Contains(search("草稿秘密"+key), "/p/"+key+"-0") {
			t.Fatal("unpublished content leaked")
		}
		cmsCode(t, c.call("POST", "/cms/search-index/rebuild", restricted.token, nil), 403)
		if _, e := c.db.ExecContext(ctx, "UPDATE cms_search_task SET status='PENDING',attempts=3,version=version+1 WHERE content_id=?", rank); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(10 * time.Second)
		failed := false
		for time.Now().Before(deadline) {
			var status string
			var tries int
			if e := c.db.QueryRowContext(ctx, "SELECT status,attempts FROM cms_search_task WHERE content_id=?", rank).Scan(&status, &tries); e != nil {
				t.Fatal(e)
			}
			if status == "FAILED" && tries == 3 {
				failed = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !failed {
			t.Fatal("search automatic retries not bounded")
		}
		c.ok("POST", "/cms/search-index/rebuild", nil)
		waitIndex(rank)
		c.ok("GET", "/cms/search-index/status", nil)
		c.ok("GET", "/cms/search-index/list?pageSize=1000", nil)
		reviewer := makeActor("v", "cms:page:query", "cms:page:review", "cms:page:publish")
		_ = reviewer
		settings := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
		required := map[string]bool{}
		for _, v := range settings["policies"].([]any) {
			p := v.(map[string]any)
			required[p["kind"].(string)] = p["required"].(bool)
		}
		required["PAGE"] = true
		c.ok("PUT", "/cms/workflow/settings", map[string]any{"version": settings["version"], "required": required, "inquiryIsolation": true, "managerIds": settings["managerIds"], "reason": key + " compare review fixture"})
		rd := get(id)
		reviewID := c.ok("POST", "/cms/review", map[string]any{"kind": "PAGE", "objectId": id, "contentVersion": rd["version"]})["data"]
		save(id, "<p>审核后的独立新稿</p>")
		req := map[string]any{"kind": "PAGE", "objectId": id, "left": map[string]any{"source": "REVIEW", "id": reviewID}, "right": map[string]any{"source": "DRAFT", "version": get(id)["version"]}}
		cmp := cmsData(c.ok("POST", "/cms/compare", req))
		if cmsID(cmp, "changedCount") < 1 {
			t.Fatal("immutable review snapshot comparison missing")
		}
		req["objectId"] = rank
		req["right"] = map[string]any{"source": "DRAFT", "version": get(rank)["version"]}
		cmsCode(t, c.call("POST", "/cms/compare", c.admin, req), 500)
		setWorkflow(true)
		s := cmsData(c.ok("GET", "/cms/site", nil))
		release := s["publishedReleaseId"]
		if release != nil {
			c.ok("POST", "/cms/compare", map[string]any{"kind": "SITE", "objectId": 1, "left": map[string]any{"source": "RELEASE", "id": release}, "right": map[string]any{"source": "DRAFT", "version": s["version"]}})
		}
	})
}
