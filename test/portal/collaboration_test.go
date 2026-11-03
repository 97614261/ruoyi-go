package portaltest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPortalCollaboration(t *testing.T) {
	c := cmsSetup(t)
	key := "cw" + strconv.FormatInt(time.Now().UnixNano(), 36)
	initial := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
	siteBefore := cmsData(c.ok("GET", "/cms/site", nil))
	type person struct {
		ID          int64
		Name, Token string
		Role        map[string]any
	}
	people := []person{}
	contents := []int64{}
	var inquiry int64
	set := func(required map[string]bool, isolation bool, managers any) {
		s := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
		c.ok("PUT", "/cms/workflow/settings", map[string]any{"version": s["version"], "required": required, "inquiryIsolation": isolation, "managerIds": managers, "reason": key + " fixture policy"})
	}
	policy := func(on bool) map[string]bool {
		return map[string]bool{"ARTICLE": on, "PRODUCT": on, "PAGE": on, "SITE": on}
	}
	t.Cleanup(func() {
		original := map[string]bool{}
		for _, v := range initial["policies"].([]any) {
			p := v.(map[string]any)
			original[p["kind"].(string)] = p["required"].(bool)
		}
		set(original, initial["inquiryIsolation"].(bool), initial["managerIds"])
		s := cmsData(c.ok("GET", "/cms/site", nil))
		if s["publishedReleaseId"] != siteBefore["publishedReleaseId"] {
			c.ok("POST", "/cms/site/publish", map[string]any{"version": s["version"], "releaseId": siteBefore["publishedReleaseId"]})
		}
		s = cmsData(c.ok("GET", "/cms/site", nil))
		c.ok("PUT", "/cms/site", map[string]any{"version": s["version"], "config": siteBefore["config"]})
		for _, id := range contents {
			p := fmt.Sprintf("/cms/page/%d", id)
			r := c.call("GET", p, c.admin, nil)
			if r["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(r)["version"]})
			}
		}
		if inquiry > 0 {
			p := fmt.Sprintf("/cms/inquiry/%d", inquiry)
			r := c.call("GET", p, c.admin, nil)
			if r["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(r)["version"]})
			}
		}
		for _, p := range people {
			c.call("POST", "/logout", p.Token, nil)
			c.ok("PUT", "/system/user", map[string]any{"userId": p.ID, "userName": p.Name, "nickName": "协作测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
			p.Role["status"] = "1"
			p.Role["menuIds"] = []int64{}
			c.ok("PUT", "/system/role", p.Role)
		}
	})
	makePerson := func(suffix string, perms []string) person {
		name := key + suffix
		args := []any{}
		marks := []string{}
		for _, p := range perms {
			args = append(args, p)
			marks = append(marks, "?")
		}
		rows, e := c.db.Query("SELECT menu_id FROM sys_menu WHERE perms IN ("+strings.Join(marks, ",")+")", args...)
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
		var roleID int64
		if e = c.db.QueryRow("SELECT role_id FROM sys_role WHERE role_key=?", name).Scan(&roleID); e != nil {
			t.Fatal(e)
		}
		role["roleId"] = roleID
		t.Cleanup(func() {
			retired := map[string]any{}
			for k, v := range role {
				retired[k] = v
			}
			retired["status"] = "1"
			retired["menuIds"] = []int64{}
			c.ok("PUT", "/system/role", retired)
		})
		c.ok("POST", "/system/user", map[string]any{"userName": name, "nickName": "本地协作验收" + suffix, "password": "Workflow_975!", "deptId": 103, "status": "0", "roleIds": []int64{roleID}, "postIds": []int64{}})
		var id int64
		if e = c.db.QueryRow("SELECT user_id FROM sys_user WHERE user_name=?", name).Scan(&id); e != nil {
			t.Fatal(e)
		}
		p := person{id, name, c.login(name, "Workflow_975!"), role}
		people = append(people, p)
		return p
	}
	editorPerms := []string{"cms:page:list", "cms:page:query", "cms:page:add", "cms:page:edit", "cms:page:publish", "cms:page:remove", "cms:page:review", "cms:asset:query", "cms:review:list", "cms:notification:list"}
	reviewPerms := []string{"cms:review:list", "cms:notification:list", "cms:asset:query"}
	for _, k := range []string{"article", "product", "page", "site"} {
		for _, a := range []string{"query", "review", "publish"} {
			reviewPerms = append(reviewPerms, "cms:"+k+":"+a)
		}
	}
	editor := makePerson("e", editorPerms)
	reviewer := makePerson("r", reviewPerms)
	inquiryPerms := []string{"cms:inquiry:list", "cms:inquiry:query", "cms:inquiry:edit", "cms:notification:list"}
	worker := makePerson("i", inquiryPerms)
	other := makePerson("j", inquiryPerms)
	call := func(method, path, token string, body any) map[string]any {
		r := c.call(method, path, token, body)
		cmsCode(t, r, 200)
		return r
	}
	makePage := func(token string) int64 {
		id := cmsID(cmsData(call("POST", "/cms/page", token, map[string]any{"slug": key + "-" + strconv.Itoa(len(contents)), "title": "协作验收固定快照", "bodyHtml": "<p>first immutable snapshot</p>"})), "contentId")
		contents = append(contents, id)
		return id
	}
	page := func(id int64) map[string]any { return cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil)) }
	submit := func(id int64, token string) int64 {
		d := page(id)
		r := call("POST", "/cms/review", token, map[string]any{"kind": "PAGE", "objectId": id, "contentVersion": d["version"]})
		return int64(r["data"].(float64))
	}
	review := func(id int64) map[string]any { return cmsData(c.ok("GET", fmt.Sprintf("/cms/review/%d", id), nil)) }
	decide := func(id int64, token, action string) {
		r := review(id)
		call("POST", fmt.Sprintf("/cms/review/%d/decision", id), token, map[string]any{"version": r["version"], "action": action, "opinion": "本地验收"})
	}
	publish := func(id int64, token string) map[string]any {
		return c.call("POST", fmt.Sprintf("/cms/page/%d/publish", id), token, map[string]any{"version": page(id)["version"]})
	}
	save := func(id int64, body string) {
		d := page(id)
		r := d["draft"].(map[string]any)
		r["contentId"] = id
		r["version"] = d["version"]
		r["slug"] = d["slug"]
		r["bodyHtml"] = body
		call("PUT", "/cms/page", editor.Token, r)
	}
	// Default mode remains direct, with ordinary publication authorization still required.
	set(policy(false), false, []int64{1})
	solo := makePage(editor.Token)
	cmsCode(t, publish(solo, editor.Token), 200)
	cmsCode(t, c.call("PUT", "/cms/workflow/settings", editor.Token, map[string]any{}), 403)
	// With no other ARTICLE/PRODUCT/SITE reviewer, the single administrator cannot enable review.
	savedMenus := reviewer.Role["menuIds"]
	reviewer.Role["menuIds"] = []int64{}
	c.ok("PUT", "/system/role", reviewer.Role)
	settings := cmsData(c.ok("GET", "/cms/workflow/settings", nil))
	cmsCode(t, c.call("PUT", "/cms/workflow/settings", c.admin, map[string]any{"version": settings["version"], "required": policy(true), "inquiryIsolation": false, "managerIds": []int64{1}, "reason": "single available reviewer must fail"}), 500)
	reviewer.Role["menuIds"] = savedMenus
	c.ok("PUT", "/system/role", reviewer.Role)
	set(policy(true), false, []int64{1})
	cmsCode(t, c.call("PUT", "/cms/workflow/settings", c.admin, map[string]any{"version": settings["version"], "required": policy(true), "inquiryIsolation": false, "managerIds": []int64{1}, "reason": "stale policy version"}), 500)
	id := makePage(editor.Token)
	path := fmt.Sprintf("/cms/page/%d", id)
	cmsCode(t, publish(id, editor.Token), 500)
	rid := submit(id, editor.Token)
	r := review(rid)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/review/%d/decision", rid), editor.Token, map[string]any{"version": r["version"], "action": "approve"}), 500)
	// Current authorship is a stable userId and the root wildcard also cannot self-review.
	own := makePage(c.admin)
	self := submit(own, c.admin)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/review/%d/decision", self), c.admin, map[string]any{"version": review(self)["version"], "action": "approve"}), 500)
	decide(self, reviewer.Token, "approve")
	cmsCode(t, publish(own, c.admin), 200)
	// A separate submitter cannot review either, even when the author is someone else.
	distinct := makePage(editor.Token)
	dr := submit(distinct, c.admin)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/review/%d/decision", dr), c.admin, map[string]any{"version": review(dr)["version"], "action": "approve"}), 500)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/review/%d/decision", dr), editor.Token, map[string]any{"version": review(dr)["version"], "action": "approve"}), 500)
	decide(dr, reviewer.Token, "approve")
	cmsCode(t, publish(distinct, c.admin), 200)
	decide(rid, reviewer.Token, "approve")
	cmsCode(t, publish(id, editor.Token), 200)
	if review(rid)["status"] != "PUBLISHED" {
		t.Fatal("approval not consumed")
	}
	c.ok("POST", path+"/unpublish", map[string]any{"version": page(id)["version"]})
	cmsCode(t, publish(id, editor.Token), 500)
	// Body and operations saves invalidate pending/approved requests, while the sent snapshot stays immutable.
	rid = submit(id, editor.Token)
	decide(rid, reviewer.Token, "approve")
	save(id, "<p>second immutable snapshot</p>")
	old := review(rid)
	if old["status"] != "STALE" || old["isCurrent"] != false {
		t.Fatal("body edit retained approval")
	}
	snap := old["snapshot"].(map[string]any)["content"].(map[string]any)["draft"].(map[string]any)
	if !strings.Contains(snap["bodyHtml"].(string), "first immutable") {
		t.Fatal("review used current body")
	}
	rid = submit(id, editor.Token)
	decide(rid, reviewer.Token, "approve")
	d := page(id)
	call("PUT", path+"/operations", editor.Token, map[string]any{"version": d["version"], "settings": map[string]any{"shareTitle": "workflow share title"}})
	if review(rid)["status"] != "STALE" {
		t.Fatal("operations retained approval")
	}
	// An autosave does not alter the formal version; its explicit commit does.
	rid = submit(id, editor.Token)
	decide(rid, reviewer.Token, "approve")
	d = page(id)
	slot := cmsData(call("GET", path+"/autosave", editor.Token, nil))
	draft := d["draft"].(map[string]any)
	draft["slug"] = d["slug"]
	draft["bodyHtml"] = "<p>recovery formal commit</p>"
	draft["contentId"] = id
	draft["version"] = d["version"]
	slot = cmsData(call("PUT", path+"/autosave", editor.Token, map[string]any{"slotId": slot["slotId"], "version": slot["version"], "baseVersion": d["version"], "draft": draft}))
	if review(rid)["status"] != "APPROVED" {
		t.Fatal("personal cache invalidated formal approval")
	}
	call("POST", path+"/autosave/commit", editor.Token, map[string]any{"slotId": slot["slotId"], "version": slot["version"], "baseVersion": d["version"], "draft": draft})
	if review(rid)["status"] != "STALE" {
		t.Fatal("recovery commit retained approval")
	}
	rid = submit(id, editor.Token)
	decide(rid, reviewer.Token, "approve")
	history := c.ok("GET", path+"/history", nil)["rows"].([]any)
	first := history[len(history)-1].(map[string]any)
	call("POST", path+"/restore", editor.Token, map[string]any{"version": page(id)["version"], "revisionId": first["revisionId"]})
	if review(rid)["status"] != "STALE" {
		t.Fatal("history restore retained approval")
	}
	// Publication batch uses exactly the same gate.
	d = page(id)
	batch := cmsData(c.ok("POST", "/cms/page/batch", map[string]any{"action": "publish", "items": []any{map[string]any{"contentId": id, "version": d["version"]}}}))
	if batch["failureCount"] != float64(1) {
		t.Fatal("batch bypassed review")
	}
	rid = submit(id, editor.Token)
	decide(rid, reviewer.Token, "approve")
	batch = cmsData(c.ok("POST", "/cms/page/batch", map[string]any{"action": "publish", "items": []any{map[string]any{"contentId": id, "version": page(id)["version"]}}}))
	if batch["successCount"] != float64(1) {
		t.Fatal("approved batch failed")
	}
	// Scheduling checks now and later, and policy changes cancel only pending publication.
	schedulePage := makePage(editor.Token)
	scheduleBody := func(action string) map[string]any {
		return map[string]any{"contentId": schedulePage, "contentVersion": page(schedulePage)["version"], "action": action, "dueAt": time.Now().In(time.FixedZone("CST", 8*3600)).Add(time.Hour).Format("2006-01-02 15:04:05")}
	}
	cmsCode(t, c.call("POST", "/cms/page/schedules", editor.Token, scheduleBody("publish")), 500)
	sr := submit(schedulePage, editor.Token)
	decide(sr, reviewer.Token, "approve")
	task := cmsID(cmsData(call("POST", "/cms/page/schedules", editor.Token, scheduleBody("publish"))), "scheduleId")
	// Revocation is live, even though the old reviewer token still exists.
	oldMenus := reviewer.Role["menuIds"]
	reviewer.Role["menuIds"] = []int64{}
	c.ok("PUT", "/system/role", reviewer.Role)
	cmsCode(t, publish(schedulePage, editor.Token), 500)
	reviewer.Role["menuIds"] = oldMenus
	c.ok("PUT", "/system/role", reviewer.Role)
	// Restoring the same grants does not resurrect the old approval generation.
	cmsCode(t, publish(schedulePage, editor.Token), 500)
	set(policy(false), false, []int64{1})
	var taskStatus string
	if e := c.db.QueryRow("SELECT status FROM cms_schedule WHERE schedule_id=?", task).Scan(&taskStatus); e != nil || taskStatus != "CANCELED" {
		t.Fatal("policy left publish task pending", e)
	}
	set(policy(true), false, []int64{1})
	sr = submit(schedulePage, editor.Token)
	decide(sr, reviewer.Token, "approve")
	task = cmsID(cmsData(call("POST", "/cms/page/schedules", editor.Token, scheduleBody("publish"))), "scheduleId")
	if _, e := c.db.Exec("UPDATE cms_schedule SET due_at=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE schedule_id=? AND owner_user_id=?", task, editor.ID); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if e := c.db.QueryRow("SELECT status FROM cms_schedule WHERE schedule_id=?", task).Scan(&taskStatus); e != nil {
			t.Fatal(e)
		}
		if taskStatus != "PENDING" {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if taskStatus != "SUCCESS" || review(sr)["status"] != "PUBLISHED" {
		t.Fatal("approved scheduled publish failed", taskStatus)
	}
	// Withdrawing and replacing an approval for the SAME draft cannot silently retarget a scheduled task.
	schedulePage = makePage(editor.Token)
	oldApproval := submit(schedulePage, editor.Token)
	decide(oldApproval, reviewer.Token, "approve")
	bindingTask := cmsID(cmsData(call("POST", "/cms/page/schedules", editor.Token, scheduleBody("publish"))), "scheduleId")
	decide(oldApproval, editor.Token, "withdraw")
	replacement := submit(schedulePage, editor.Token)
	decide(replacement, reviewer.Token, "approve")
	if _, e := c.db.Exec("UPDATE cms_schedule SET due_at=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE schedule_id=? AND owner_user_id=?", bindingTask, editor.ID); e != nil {
		t.Fatal(e)
	}
	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if e := c.db.QueryRow("SELECT status FROM cms_schedule WHERE schedule_id=?", bindingTask).Scan(&taskStatus); e != nil {
			t.Fatal(e)
		}
		if taskStatus != "PENDING" {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if taskStatus != "FAILED" || review(replacement)["status"] != "APPROVED" {
		t.Fatal("schedule accepted a replacement approval or consumed it", taskStatus)
	}
	var noticeID, noticeVersion int64
	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if e := c.db.QueryRow("SELECT notification_id,version FROM cms_notification WHERE resource_type='SCHEDULE' AND resource_id=? AND recipient_user_id=?", bindingTask, editor.ID).Scan(&noticeID, &noticeVersion); e == nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if noticeID == 0 {
		t.Fatal("failed schedule notification missing")
	}
	target := cmsData(call("POST", fmt.Sprintf("/cms/notifications/%d/read", noticeID), editor.Token, map[string]any{"version": noticeVersion}))
	if target["targetContentId"] != float64(schedulePage) {
		t.Fatal("notification cannot open the exact schedule content", target["targetContentId"])
	}
	// Two eligible reviewers cannot both decide one pending request.
	racePage := makePage(editor.Token)
	raceReview := submit(racePage, editor.Token)
	v := review(raceReview)["version"]
	var wg sync.WaitGroup
	answers := make(chan map[string]any, 2)
	for _, token := range []string{reviewer.Token, c.admin} {
		wg.Add(1)
		go func(tok string) {
			defer wg.Done()
			answers <- c.call("POST", fmt.Sprintf("/cms/review/%d/decision", raceReview), tok, map[string]any{"version": v, "action": "approve"})
		}(token)
	}
	wg.Wait()
	close(answers)
	success := 0
	for a := range answers {
		if a["code"] == float64(200) {
			success++
		}
	}
	if success != 1 {
		t.Fatal("decision CAS failed", success)
	}
	// ARTICLE and PRODUCT use the same immutable gate, with their required channels.
	for _, kind := range []string{"ARTICLE", "PRODUCT"} {
		base := strings.ToLower(kind)
		chKey := key + "-" + base
		channel := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": chKey, "slug": chKey, "kind": kind, "status": "0"})), "channelId")
		content := cmsID(cmsData(c.ok("POST", "/cms/"+base, map[string]any{"title": chKey, "slug": chKey, "channelId": channel, "bodyHtml": "<p>kind review gate</p>"})), "contentId")
		p := fmt.Sprintf("/cms/%s/%d", base, content)
		t.Cleanup(func() {
			d := cmsData(c.ok("GET", p, nil))
			c.ok("DELETE", p, map[string]any{"version": d["version"]})
			ch := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", channel), nil))
			ch["status"] = "1"
			c.ok("PUT", "/cms/channel", ch)
		})
		d := cmsData(c.ok("GET", p, nil))
		cmsCode(t, c.call("POST", p+"/publish", c.admin, map[string]any{"version": d["version"]}), 500)
		r := int64(c.ok("POST", "/cms/review", map[string]any{"kind": kind, "objectId": content, "contentVersion": d["version"]})["data"].(float64))
		decide(r, reviewer.Token, "approve")
		c.ok("POST", p+"/publish", map[string]any{"version": d["version"]})
	}
	// Removing an object explicitly invalidates its pending review and hides its notification.
	removed := makePage(editor.Token)
	removedReview := submit(removed, editor.Token)
	rp := fmt.Sprintf("/cms/page/%d", removed)
	c.ok("DELETE", rp, map[string]any{"version": page(removed)["version"]})
	if review(removedReview)["status"] != "STALE" {
		t.Fatal("removed object left a pending approval")
	}
	noticeList := call("GET", "/cms/notifications/list?pageSize=100", reviewer.Token, nil)
	for _, v := range noticeList["rows"].([]any) {
		n := v.(map[string]any)
		if n["resourceType"] == "REVIEW" && n["resourceId"] == float64(removedReview) {
			t.Fatal("removed review source remained visible in inbox")
		}
	}
	// Site history cannot be restored straight into publication while review is on.
	s := cmsData(c.ok("GET", "/cms/site", nil))
	c.ok("PUT", "/cms/site", map[string]any{"version": s["version"], "config": siteBefore["config"]})
	s = cmsData(c.ok("GET", "/cms/site", nil))
	cmsCode(t, c.call("POST", "/cms/site/publish", c.admin, map[string]any{"version": s["version"]}), 500)
	siteReview := int64(c.ok("POST", "/cms/review", map[string]any{"kind": "SITE", "objectId": 1, "contentVersion": s["version"]})["data"].(float64))
	preview := call("GET", fmt.Sprintf("/cms/review/%d/preview", siteReview), reviewer.Token, nil)
	if len(cmsData(preview)["html"].(string)) < 100 {
		t.Fatal("snapshot site preview missing")
	}
	decide(siteReview, reviewer.Token, "approve")
	c.ok("POST", "/cms/site/publish", map[string]any{"version": s["version"]})
	s = cmsData(c.ok("GET", "/cms/site", nil))
	cmsCode(t, c.call("POST", "/cms/site/publish", c.admin, map[string]any{"version": s["version"], "releaseId": siteBefore["publishedReleaseId"]}), 500)
	c.ok("POST", "/cms/site/restore-draft", map[string]any{"version": s["version"], "releaseId": siteBefore["publishedReleaseId"]})
	s = cmsData(c.ok("GET", "/cms/site", nil))
	cmsCode(t, c.call("POST", "/cms/site/publish", c.admin, map[string]any{"version": s["version"]}), 500)
	// Inquiry tests create only this run's private fixture; old and new endpoints must agree on row scope.
	result, e := c.db.Exec("INSERT INTO cms_inquiry(name,contact,message,status,note,version,is_deleted,create_by,create_time,update_by,remark) VALUES(?,?,?,'NEW','',1,0,?,NOW(),'','')", key, "workflow-private@example.invalid", "private workflow message", key)
	if e != nil {
		t.Fatal(e)
	}
	inquiry, e = result.LastInsertId()
	if e != nil {
		t.Fatal(e)
	}
	ip := fmt.Sprintf("/cms/inquiry/%d", inquiry)
	// Isolation off: no forced assignment, preserving solo processing.
	set(policy(true), false, []int64{1})
	call("GET", ip, worker.Token, nil)
	set(policy(true), true, []int64{1})
	cmsCode(t, c.call("GET", ip, worker.Token, nil), 500)
	d = cmsData(c.ok("GET", ip, nil))
	c.ok("POST", ip+"/assign", map[string]any{"version": d["version"], "ownerUserId": worker.ID})
	d = cmsData(call("GET", ip, worker.Token, nil))
	cmsCode(t, c.call("GET", ip+"/followups", other.Token, nil), 500)
	call("POST", ip+"/followups", worker.Token, map[string]any{"version": d["version"], "text": "private followup canary", "result": "private result", "nextFollowAt": time.Now().In(time.FixedZone("CST", 8*3600)).Add(time.Hour).Format("2006-01-02 15:04:05")})
	// Force only the fixture's due time; the managed worker produces one unique reminder per version.
	if _, e = c.db.Exec("UPDATE cms_inquiry_team SET next_follow_at=DATE_SUB(NOW(),INTERVAL 1 SECOND) WHERE inquiry_id=?", inquiry); e != nil {
		t.Fatal(e)
	}
	var reminders int
	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		c.db.QueryRow("SELECT COUNT(*) FROM cms_notification_outbox WHERE resource_type='INQUIRY' AND resource_id=? AND event_type='INQUIRY_DUE'", inquiry).Scan(&reminders)
		if reminders > 0 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if reminders != 1 {
		t.Fatal("due reminder not queued exactly once", reminders)
	}
	notices := call("GET", "/cms/notifications/list?pageSize=100", worker.Token, nil)
	serialized, _ := json.Marshal(notices)
	if strings.Contains(string(serialized), "private") {
		t.Fatal("notification contains customer text")
	}
	var note map[string]any
	for _, n := range notices["rows"].([]any) {
		r := n.(map[string]any)
		if r["resourceType"] == "INQUIRY" && cmsID(r, "resourceId") == inquiry {
			note = r
		}
	}
	if note == nil {
		time.Sleep(1200 * time.Millisecond)
		notices = call("GET", "/cms/notifications/list?pageSize=100", worker.Token, nil)
		for _, n := range notices["rows"].([]any) {
			r := n.(map[string]any)
			if r["resourceType"] == "INQUIRY" && cmsID(r, "resourceId") == inquiry {
				note = r
			}
		}
	}
	if note == nil {
		t.Fatal("assignment/reminder notification missing")
	}
	d = cmsData(c.ok("GET", ip, nil))
	c.ok("POST", ip+"/assign", map[string]any{"version": d["version"], "ownerUserId": other.ID})
	for _, suffix := range []string{"", "/team", "/followups"} {
		cmsCode(t, c.call("GET", ip+suffix, worker.Token, nil), 500)
	}
	cmsCode(t, c.call("PUT", ip, worker.Token, map[string]any{"version": d["version"], "status": "DONE"}), 500)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/notifications/%d/read", cmsID(note, "notificationId")), worker.Token, map[string]any{"version": note["version"]}), 500)
	workbench := cmsData(call("GET", "/cms/workbench", worker.Token, nil))
	if workbench["inquiryNew"] != float64(0) {
		t.Fatal("old owner aggregate leaked reassigned inquiry")
	}
	// Direct call to the original list cannot reveal another owner's row.
	list := call("GET", "/cms/inquiry/list?pageSize=100", worker.Token, nil)
	for _, v := range list["rows"].([]any) {
		if cmsID(v.(map[string]any), "inquiryId") == inquiry {
			t.Fatal("old list bypassed scope")
		}
	}
	d = cmsData(c.ok("GET", ip, nil))
	c.ok("DELETE", ip, map[string]any{"version": d["version"]})
	var text, resultText string
	var active int
	if e = c.db.QueryRow("SELECT text,result FROM cms_inquiry_followup WHERE inquiry_id=? AND event_type='FOLLOWUP' LIMIT 1", inquiry).Scan(&text, &resultText); e != nil || text != "" || resultText != "" {
		t.Fatal("followup erasure failed", e)
	}
	c.db.QueryRow("SELECT COUNT(*) FROM cms_notification WHERE resource_type='INQUIRY' AND resource_id=? AND expires_at>NOW()", inquiry).Scan(&active)
	if active != 0 {
		t.Fatal("erased inquiry notifications active")
	}
	impact := c.ok("GET", "/cms/workflow/impact", nil)["data"].([]any)
	if len(impact) != 4 {
		t.Fatal("policy impact missing kinds")
	}
	cmsCode(t, c.call("GET", "/cms/workflow/impact", editor.Token, nil), 403)
	call("GET", "/cms/workbench", editor.Token, nil)
	call("GET", "/cms/workbench/drafts?pageSize=1000", editor.Token, nil)
	cmsCode(t, c.call("GET", "/cms/workflow/people?purpose=inquiry", worker.Token, nil), 403)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var tables int
	if e = c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='ruoyi_go_portal'").Scan(&tables); e != nil || tables != 58 {
		t.Fatal("v16 structure mismatch")
	}
	t.Log("PASS default direct publishing; immutable two-person review; author/submitter self-review; body/ops/recovery/history invalidation; batch/schedule/site gates; live revocation; CAS; inquiry scopes/erasure/reminders; privacy and workbench")
}
