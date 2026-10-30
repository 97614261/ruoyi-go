package portaltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"ruoyi-go/pkg/types"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCMSSchedules(t *testing.T) {
	c := cmsSetup(t)
	prefix := "schedule-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	ctx := context.Background()
	ids := []int64{}
	tasks := []int64{}
	t.Cleanup(func() {
		for _, id := range tasks {
			rows := c.ok("GET", "/cms/page/schedules?pageSize=50", nil)["rows"].([]any)
			for _, v := range rows {
				row := v.(map[string]any)
				if cmsID(row, "scheduleId") == id && row["status"] == "PENDING" {
					c.ok("POST", fmt.Sprintf("/cms/page/schedules/%d/cancel", id), map[string]any{"version": row["version"]})
				}
			}
		}
		for _, id := range ids {
			p := fmt.Sprintf("/cms/page/%d", id)
			out := c.call("GET", p, c.admin, nil)
			if out["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(out)["version"]})
			}
		}
	})
	create := func(suffix, body string) map[string]any {
		out := c.ok("POST", "/cms/page", map[string]any{"title": prefix + suffix, "slug": prefix + suffix, "bodyHtml": body})
		id := cmsID(cmsData(out), "contentId")
		ids = append(ids, id)
		return cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil))
	}
	success := create("-ok", "<p>定时发布测试</p>")
	changed := create("-changed", "<p>原稿</p>")
	canceled := create("-canceled", "<p>取消测试</p>")
	disabled := create("-disabled", "<p>停用账号测试</p>")
	revoked := create("-revoked", "<p>撤权测试</p>")
	expired := create("-expired", "<p>过期测试</p>")
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 12, 12)))
	upload := c.upload(prefix+".png", img.Bytes())
	cmsCode(t, upload, 200)
	asset := cmsID(cmsData(upload), "assetId")
	missing := create("-missing", fmt.Sprintf(`<p>文件丢失测试</p><img src="/media/%d">`, asset))
	// Create two independent limited publishers; only this test's users/roles are modified.
	publisher := func(suffix string) (string, func(), func()) {
		roleKey := "sched_" + strconv.FormatInt(time.Now().UnixNano(), 36)
		username := "sp" + strconv.FormatInt(time.Now().UnixNano(), 36)
		password := "Schedule_test_987!"
		menus := []int64{}
		rs, e := c.db.QueryContext(ctx, "SELECT menu_id FROM sys_menu WHERE perms IN ('cms:page:list','cms:page:query','cms:page:publish')")
		if e != nil {
			t.Fatal(e)
		}
		for rs.Next() {
			var id int64
			if e = rs.Scan(&id); e != nil {
				t.Fatal(e)
			}
			menus = append(menus, id)
		}
		rs.Close()
		if len(menus) != 3 {
			t.Fatal("publisher menus missing")
		}
		role := map[string]any{"roleName": roleKey, "roleKey": roleKey, "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": menus, "menuCheckStrictly": true, "deptCheckStrictly": true}
		c.ok("POST", "/system/role", role)
		var rid, uid int64
		if e = c.db.QueryRowContext(ctx, "SELECT role_id FROM sys_role WHERE role_key=?", roleKey).Scan(&rid); e != nil {
			t.Fatal(e)
		}
		role["roleId"] = rid
		t.Cleanup(func() {
			if uid > 0 {
				c.ok("PUT", "/system/user", map[string]any{"userId": uid, "userName": username, "nickName": "定时测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
			}
			role["status"] = "1"
			role["menuIds"] = []int64{}
			c.ok("PUT", "/system/role", role)
		})
		c.ok("POST", "/system/user", map[string]any{"userName": username, "nickName": "定时测试" + suffix, "password": password, "deptId": 103, "status": "0", "roleIds": []int64{rid}, "postIds": []int64{}})
		if e = c.db.QueryRowContext(ctx, "SELECT user_id FROM sys_user WHERE user_name=?", username).Scan(&uid); e != nil {
			t.Fatal(e)
		}
		token := c.login(username, password)
		t.Cleanup(func() { c.call("POST", "/logout", token, nil) })
		disable := func() { c.ok("PUT", "/system/user/changeStatus", map[string]any{"userId": uid, "status": "1"}) }
		revoke := func() { role["menuIds"] = []int64{}; c.ok("PUT", "/system/role", role) }
		return token, disable, revoke
	}
	disabledToken, disable, _ := publisher("停用")
	revokedToken, _, revoke := publisher("撤权")
	due := time.Now().In(types.Location).Add(15 * time.Second).Format(types.Layout)
	arrange := func(row map[string]any, token, action string) int64 {
		out := c.call("POST", "/cms/page/schedules", token, map[string]any{"contentId": row["contentId"], "contentVersion": row["version"], "action": action, "dueAt": due})
		cmsCode(t, out, 200)
		id := cmsID(cmsData(out), "scheduleId")
		tasks = append(tasks, id)
		return id
	}
	okID := arrange(success, c.admin, "publish")
	changeID := arrange(changed, c.admin, "publish")
	cancelID := arrange(canceled, c.admin, "publish")
	disableID := arrange(disabled, disabledToken, "publish")
	revokeID := arrange(revoked, revokedToken, "publish")
	missingID := arrange(missing, c.admin, "publish")
	expireID := arrange(expired, c.admin, "publish")
	input := map[string]any{"contentId": success["contentId"], "contentVersion": success["version"], "action": "publish", "dueAt": due}
	cmsCode(t, c.call("POST", "/cms/page/schedules", c.admin, input), 500)
	cmsCode(t, c.call("POST", "/cms/article/schedules", c.admin, input), 500)
	cmsCode(t, c.call("POST", "/cms/page/schedules", "", input), 401)
	input["dueAt"] = "2026-01-01 00:00:00"
	cmsCode(t, c.call("POST", "/cms/page/schedules", c.admin, input), 500)
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/page/schedules/%d/cancel", okID), revokedToken, map[string]any{"version": 1}), 403)
	c.ok("POST", fmt.Sprintf("/cms/page/schedules/%d/cancel", cancelID), map[string]any{"version": 1})
	cmsCode(t, c.call("POST", fmt.Sprintf("/cms/page/schedules/%d/cancel", cancelID), c.admin, map[string]any{"version": 1}), 500)
	draft := changed["draft"].(map[string]any)
	draft["contentId"] = changed["contentId"]
	draft["version"] = changed["version"]
	draft["slug"] = changed["slug"]
	draft["bodyHtml"] = "<p>修改后的新稿不能被旧任务发布</p>"
	c.ok("PUT", "/cms/page", draft)
	disable()
	revoke()
	cmsCode(t, c.call("POST", "/cms/page/schedules", revokedToken, input), 403)
	var key string
	if e := c.db.QueryRowContext(ctx, "SELECT storage_key FROM cms_asset WHERE asset_id=?", asset).Scan(&key); e != nil {
		t.Fatal(e)
	}
	if filepath.Base(key) != key {
		t.Fatal("unsafe key")
	}
	file := filepath.Join("../../uploadPath-portal-private", key)
	off := file + ".schedule-test"
	if e := os.Rename(file, off); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := os.Rename(off, file); e != nil {
			t.Error("restore own file failed")
		}
	})
	if _, e := c.db.ExecContext(ctx, "UPDATE cms_schedule SET due_at=? WHERE schedule_id=? AND create_by='admin'", time.Now().In(types.Location).Add(-25*time.Hour), expireID); e != nil {
		t.Fatal(e)
	}
	expected := map[int64]string{okID: "SUCCESS", changeID: "FAILED", cancelID: "CANCELED", disableID: "FAILED", revokeID: "FAILED", missingID: "FAILED", expireID: "FAILED"}
	for deadline := time.Now().Add(35 * time.Second); ; {
		complete := true
		for id, want := range expected {
			var status, message string
			var version int64
			if e := c.db.QueryRowContext(ctx, "SELECT status,result_message,version FROM cms_schedule WHERE schedule_id=?", id).Scan(&status, &message, &version); e != nil {
				t.Fatal(e)
			}
			if status == "PENDING" {
				complete = false
				continue
			}
			if status != want || version != 2 {
				t.Fatalf("task %d: %s version%d want%s (%s)", id, status, version, want, message)
			}
			if status == "FAILED" && message == "" {
				t.Fatal("missing failure reason")
			}
		}
		if complete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker failed to finish due tasks")
		}
		time.Sleep(250 * time.Millisecond)
	}
	for _, row := range []map[string]any{changed, canceled, disabled, revoked, missing, expired} {
		if cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", cmsID(row, "contentId")), nil))["isPublished"] != float64(0) {
			t.Fatal("failed or canceled task published content")
		}
	}
	current := cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", cmsID(success, "contentId")), nil))
	if current["isPublished"] != float64(1) || current["version"] != success["version"].(float64)+1 {
		t.Fatal("publication not committed exactly once")
	}
	due = time.Now().In(types.Location).Add(7 * time.Second).Format(types.Layout)
	unpublish := arrange(current, c.admin, "unpublish")
	for deadline := time.Now().Add(15 * time.Second); ; {
		var status string
		if e := c.db.QueryRowContext(ctx, "SELECT status FROM cms_schedule WHERE schedule_id=?", unpublish).Scan(&status); e != nil {
			t.Fatal(e)
		}
		if status == "SUCCESS" {
			break
		}
		if status != "PENDING" || time.Now().After(deadline) {
			t.Fatal("unpublish failed", status)
		}
		time.Sleep(200 * time.Millisecond)
	}
	current = cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", cmsID(success, "contentId")), nil))
	if current["isPublished"] != float64(0) || current["version"] != success["version"].(float64)+2 {
		t.Fatal("terminal job replayed or unpublish not committed")
	}
	t.Log("PASS scheduled publish/unpublish; canceled/stale/disabled/revoked/missing/overdue rejected; CAS, owner and kind boundaries; terminal tasks do not rerun")
}

func TestCMSScheduleRestart(t *testing.T) {
	mode := os.Getenv("PORTAL_SCHEDULE_RESTART")
	if mode != "prepare" && mode != "verify" {
		t.Skip("explicit restart orchestration required")
	}
	c := cmsSetup(t)
	file := "../../logs/portal-schedule/restart-fixture.local.json"
	state := map[string]any{}
	if mode == "prepare" {
		if _, e := os.Stat(file); !os.IsNotExist(e) {
			t.Fatal("fresh restart fixture path required")
		}
		slug := "restart-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		id := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"title": slug, "slug": slug, "bodyHtml": "<p>重启持久化测试</p>"})), "contentId")
		row := cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil))
		due := time.Now().In(types.Location).Add(20 * time.Second)
		job := cmsID(cmsData(c.ok("POST", "/cms/page/schedules", map[string]any{"contentId": id, "contentVersion": row["version"], "action": "publish", "dueAt": due.Format(types.Layout)})), "scheduleId")
		state = map[string]any{"contentId": id, "scheduleId": job, "version": row["version"], "dueAt": due.Format(types.Layout), "slug": slug}
		b, e := json.Marshal(state)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(file, b, 0600); e != nil {
			t.Fatal(e)
		}
		t.Log("PASS prepared durable task for external stop/restart")
		return
	}
	b, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &state); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(state["slug"].(string), "restart-") {
		t.Fatal("fixture mismatch")
	}
	id := cmsID(state, "contentId")
	t.Cleanup(func() {
		p := fmt.Sprintf("/cms/page/%d", id)
		row := cmsData(c.ok("GET", p, nil))
		c.ok("DELETE", p, map[string]any{"version": row["version"]})
	})
	for deadline := time.Now().Add(20 * time.Second); ; {
		var status string
		var version int64
		if e = c.db.QueryRow("SELECT status,version FROM cms_schedule WHERE schedule_id=? AND content_id=?", state["scheduleId"], id).Scan(&status, &version); e != nil {
			t.Fatal(e)
		}
		if status == "SUCCESS" && version == 2 {
			break
		}
		if status != "PENDING" || time.Now().After(deadline) {
			t.Fatal("restart task not recovered", status)
		}
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond)
	row := cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil))
	if row["isPublished"] != float64(1) || row["version"] != state["version"].(float64)+1 {
		t.Fatal("restarted task lost or duplicated")
	}
	t.Log("PASS actual restart replayed missed task once; cleanup soft-deleted owned content")
}

func TestCMSScheduleKindsAndChannelGate(t *testing.T) {
	c := cmsSetup(t)
	prefix := "schedule-kind-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	channels := []int64{}
	ids := map[int64]string{}
	jobs := map[int64]string{}
	t.Cleanup(func() {
		for id, kind := range ids {
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
	due := time.Now().In(types.Location).Add(9 * time.Second).Format(types.Layout)
	for _, kind := range []string{"article", "product"} {
		cid := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": prefix + kind, "slug": prefix + kind, "parentId": 0, "kind": strings.ToUpper(kind), "status": "0"})), "channelId")
		channels = append(channels, cid)
		for _, suffix := range []string{"ok", "blocked"} {
			slug := prefix + kind + suffix
			id := cmsID(cmsData(c.ok("POST", "/cms/"+kind, map[string]any{"title": slug, "slug": slug, "channelId": cid, "bodyHtml": "<p>类型与栏目门禁测试</p>"})), "contentId")
			ids[id] = kind
			d := cmsData(c.ok("GET", fmt.Sprintf("/cms/%s/%d", kind, id), nil))
			in := map[string]any{"contentId": id, "contentVersion": d["version"], "action": "unpublish", "dueAt": due}
			cmsCode(t, c.call("POST", "/cms/"+kind+"/schedules", c.admin, in), 500)
			in["action"] = "publish"
			job := cmsID(cmsData(c.ok("POST", "/cms/"+kind+"/schedules", in)), "scheduleId")
			jobs[job] = "SUCCESS"
			if suffix == "blocked" { // A separate channel for this draft is disabled after scheduling.
				other := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": slug, "slug": slug, "parentId": 0, "kind": strings.ToUpper(kind), "status": "0"})), "channelId")
				channels = append(channels, other)
				c.ok("POST", fmt.Sprintf("/cms/%s/schedules/%d/cancel", kind, job), map[string]any{"version": 1})
				delete(jobs, job)
				draft := d["draft"].(map[string]any)
				draft["contentId"] = id
				draft["version"] = d["version"]
				draft["slug"] = slug
				draft["channelId"] = other
				c.ok("PUT", "/cms/"+kind, draft)
				d = cmsData(c.ok("GET", fmt.Sprintf("/cms/%s/%d", kind, id), nil))
				in["contentVersion"] = d["version"]
				job = cmsID(cmsData(c.ok("POST", "/cms/"+kind+"/schedules", in)), "scheduleId")
				jobs[job] = "FAILED"
				channel := cmsData(c.ok("GET", fmt.Sprintf("/cms/channel/%d", other), nil))
				channel["status"] = "1"
				c.ok("PUT", "/cms/channel", channel)
			}
		}
	}
	for deadline := time.Now().Add(20 * time.Second); ; {
		done := true
		for id, want := range jobs {
			var got string
			if e := c.db.QueryRow("SELECT status FROM cms_schedule WHERE schedule_id=?", id).Scan(&got); e != nil {
				t.Fatal(e)
			}
			if got == "PENDING" {
				done = false
			} else if got != want {
				t.Fatalf("task%d want%s got%s", id, want, got)
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("kind jobs did not finish")
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Log("PASS article/product scheduled publication and disabled-channel execution gates; unpublished content cannot schedule unpublish")
}
