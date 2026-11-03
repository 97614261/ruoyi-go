package portaltest

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortalOperationsHistory(t *testing.T) {
	c := cmsSetup(t)
	slug := "ops-history-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	id := cmsID(cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": slug, "title": "运营历史测试", "bodyHtml": "<p>历史正文</p>"})), "contentId")
	p := fmt.Sprintf("/cms/page/%d", id)
	t.Cleanup(func() {
		slot := cmsData(c.ok("GET", p+"/autosave", nil))
		c.ok("DELETE", p+"/autosave", map[string]any{"slotId": slot["slotId"], "version": slot["version"]})
		d := cmsData(c.ok("GET", p, nil))
		c.ok("DELETE", p, map[string]any{"version": d["version"]})
	})
	set := func(title string) {
		v := cmsData(c.ok("GET", p+"/operations", nil))
		c.ok("PUT", p+"/operations", map[string]any{"version": v["version"], "settings": map[string]any{"shareTitle": title}})
	}
	publish := func() {
		v := cmsData(c.ok("GET", p, nil))
		c.ok("POST", p+"/publish", map[string]any{"version": v["version"]})
	}
	set("历史分享A")
	old := cmsData(c.ok("GET", p, nil))
	set("历史分享B")
	publish()
	d := cmsData(c.ok("GET", p, nil))
	c.ok("POST", p+"/restore", map[string]any{"version": d["version"], "revisionId": old["workingRevisionId"]})
	ops := cmsData(c.ok("GET", p+"/operations", nil))
	if ops["settings"].(map[string]any)["shareTitle"] != "历史分享A" {
		t.Fatal("history restored wrong operations")
	}
	_, html, _ := c.public("/p/"+slug, "")
	if !strings.Contains(html, "历史分享B") {
		t.Fatal("history restore altered published snapshot")
	}
	publish()
	_, html, _ = c.public("/p/"+slug, "")
	if !strings.Contains(html, "历史分享A") {
		t.Fatal("restored operations failed publication")
	}
	d = cmsData(c.ok("GET", p, nil))
	slot := cmsData(c.ok("GET", p+"/autosave", nil))
	draft := map[string]any{"title": "恢复后继续编辑", "slug": slug, "bodyHtml": "<p>新正文</p>"}
	saved := cmsData(c.ok("PUT", p+"/autosave", map[string]any{"slotId": slot["slotId"], "version": slot["version"], "baseVersion": d["version"], "draft": draft}))
	c.ok("POST", p+"/autosave/commit", map[string]any{"slotId": saved["slotId"], "version": saved["version"], "baseVersion": d["version"], "draft": draft})
	ops = cmsData(c.ok("GET", p+"/operations", nil))
	if ops["settings"].(map[string]any)["shareTitle"] != "历史分享A" {
		t.Fatal("recovery commit lost inherited operations")
	}
	t.Log("PASS selected history operations snapshot, public isolation and recovery-commit inheritance")
}
