package portaltest

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestOpsGUIFixture(t *testing.T) {
	mode := os.Getenv("PORTAL_OPS_GUI")
	if mode != "seed" && mode != "cleanup" {
		t.Skip("explicit owned ops GUI fixture")
	}
	c := cmsSetup(t)
	file := "../../logs/portal-phase2/gui-fixture.local.json"
	if mode == "seed" {
		if _, e := os.Stat(file); !os.IsNotExist(e) {
			t.Fatal("fixture record exists")
		}
		prefix := "ops-gui-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		ids := []int64{}
		for i := 1; i <= 2; i++ {
			d := cmsData(c.ok("POST", "/cms/page", map[string]any{"slug": prefix + "-" + fmt.Sprint(i), "title": fmt.Sprintf("运营页面验收%d", i), "bodyHtml": `<p>运营页面验收</p><a href="/p/` + prefix + `-missing">失效链接测试</a>`}))
			ids = append(ids, cmsID(d, "contentId"))
		}
		b, _ := json.Marshal(map[string]any{"prefix": prefix, "ids": ids})
		if e := os.WriteFile(file, b, 0600); e != nil {
			t.Fatal(e)
		}
	} else {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		v := map[string]any{}
		if json.Unmarshal(b, &v) != nil {
			t.Fatal("invalid fixture")
		}
		prefix := v["prefix"].(string)
		for i, id := range v["ids"].([]any) {
			p := fmt.Sprintf("/cms/page/%d", int64(id.(float64)))
			d := cmsData(c.ok("GET", p, nil))
			if d["slug"] != prefix+"-"+fmt.Sprint(i+1) {
				t.Fatal("fixture identity mismatch")
			}
			c.ok("DELETE", p, map[string]any{"version": d["version"]})
		}
	}
}
