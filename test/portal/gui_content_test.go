package portaltest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestCMSContentGUIFixture(t *testing.T) {
	mode := os.Getenv("PORTAL_CONTENT_GUI")
	if mode != "seed" && mode != "cleanup" {
		t.Skip("explicit owned GUI fixture mode")
	}
	c := cmsSetup(t)
	file := "../../logs/portal-step3/gui-fixture.local.json"
	if mode == "seed" {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("fixture record exists; do not overwrite")
		}
		prefix := "gui-content-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		var b bytes.Buffer
		png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 120, 70)))
		asset := cmsData(c.upload(prefix+".png", b.Bytes()))
		assetID := cmsID(asset, "assetId")
		channelID := cmsID(cmsData(c.ok("POST", "/cms/channel", map[string]any{"name": "第3步页面验收栏目", "slug": prefix, "kind": "ARTICLE", "status": "0", "orderNum": 0})), "channelId")
		id := cmsID(cmsData(c.ok("POST", "/cms/article", map[string]any{"title": "第3步页面验收文章", "slug": prefix, "channelId": channelID, "bodyHtml": fmt.Sprintf(`<h2>页面验收正文</h2><p>此内容仅用于本地测试。</p><img src="/media/%d">`, assetID)})), "contentId")
		payload, _ := json.Marshal(map[string]any{"contentId": id, "channelId": channelID, "assetId": assetID, "slug": prefix})
		if err := os.WriteFile(file, payload, 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("GUI fixture ready; identifiers saved locally")
	} else {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		v := map[string]any{}
		if err = json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		id := cmsID(v, "contentId")
		p := fmt.Sprintf("/cms/article/%d", id)
		d := cmsData(c.ok("GET", p, nil))
		if d["slug"] != v["slug"] {
			t.Fatal("fixture identity mismatch")
		}
		c.ok("DELETE", p, map[string]any{"version": d["version"]})
		pages := c.ok("GET", "/cms/page/list?pageSize=100", nil)["rows"].([]any)
		for _, item := range pages {
			row := item.(map[string]any)
			if row["slug"] == v["slug"].(string)+"-page" {
				if row["title"] != "第3步单页页面验收" {
					t.Fatal("page fixture identity mismatch")
				}
				c.ok("DELETE", fmt.Sprintf("/cms/page/%d", cmsID(row, "contentId")), map[string]any{"version": row["version"]})
			}
		}

		cp := fmt.Sprintf("/cms/channel/%d", cmsID(v, "channelId"))
		ch := cmsData(c.ok("GET", cp, nil))
		ch["status"] = "1"
		c.ok("PUT", "/cms/channel", ch)
		t.Log("GUI fixture soft-deleted, channel disabled, immutable private history retained")
	}
}
