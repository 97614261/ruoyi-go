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
	"ruoyi-go/internal/config"
	"strconv"
	"testing"
	"time"
)

func TestCMSOperations(t *testing.T) {
	c := cmsSetup(t)
	slug := "ops-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 20)))
	upload := c.upload(slug+".png", b.Bytes())
	cmsCode(t, upload, 200)
	asset := cmsID(cmsData(upload), "assetId")
	ids := []int64{}
	t.Cleanup(func() {
		for _, id := range ids {
			p := fmt.Sprintf("/cms/page/%d", id)
			out := c.call("GET", p, c.admin, nil)
			if out["code"] == float64(200) {
				c.ok("DELETE", p, map[string]any{"version": cmsData(out)["version"]})
			}
		}
	})
	cfg, e := config.Load("../../configs/portal.local.yml")
	if e != nil {
		t.Fatal(e)
	}
	origin := cfg.Portal.PublicURL
	create := func(suffix string) map[string]any {
		t.Helper()
		out := c.ok("POST", "/cms/page", map[string]any{"title": slug + suffix, "slug": slug + suffix, "bodyHtml": fmt.Sprintf(`<p>运营检查</p><img src="/media/%d"><a href="/p/%s-missing">内部失效路径</a><a href="%s/p/%s-absolute">绝对内链</a><a href="https://example.com/">外链不探测</a>`, asset, slug, origin, slug)})
		id := cmsID(cmsData(out), "contentId")
		ids = append(ids, id)
		return cmsData(c.ok("GET", fmt.Sprintf("/cms/page/%d", id), nil))
	}
	first, second, deleted := create("-one"), create("-two"), create("-deleted")
	firstID, secondID, deletedID := cmsID(first, "contentId"), cmsID(second, "contentId"), cmsID(deleted, "contentId")
	p := fmt.Sprintf("/cms/page/%d", firstID)
	check := cmsData(c.ok("GET", p+"/check", nil))
	if check["ready"] != true {
		t.Fatal("valid draft failed precheck")
	}
	cmsCode(t, c.call("GET", p+"/check", "", nil), 401)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var key string
	if e := c.db.QueryRowContext(ctx, "SELECT storage_key FROM cms_asset WHERE asset_id=?", asset).Scan(&key); e != nil {
		t.Fatal(e)
	}
	if filepath.Base(key) != key {
		t.Fatal("unsafe fixture key")
	}
	file := filepath.Join("..", "..", "uploadPath-portal-private", key)
	off := file + ".test-offline"
	if e := os.Rename(file, off); e != nil {
		t.Fatal("cannot hide owned fixture image")
	}
	hidden := true
	t.Cleanup(func() {
		if hidden {
			if e := os.Rename(off, file); e != nil {
				t.Error("owned image restore failed")
			}
		}
	})
	check = cmsData(c.ok("GET", p+"/check", nil))
	if check["ready"] != false {
		t.Fatal("missing file accepted by precheck")
	}
	cmsCode(t, c.call("POST", p+"/publish", c.admin, map[string]any{"version": first["version"]}), 500)
	if e := os.Rename(off, file); e != nil {
		t.Fatal(e)
	}
	hidden = false
	c.ok("DELETE", fmt.Sprintf("/cms/page/%d", deletedID), map[string]any{"version": deleted["version"]})
	// One valid item, one stale version, one deleted item: report partial success truthfully.
	var auditBefore int64
	if e := c.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(oper_id),0) FROM sys_oper_log").Scan(&auditBefore); e != nil {
		t.Fatal(e)
	}
	out := cmsData(c.ok("POST", "/cms/page/batch", map[string]any{"action": "publish", "items": []any{map[string]any{"contentId": firstID, "version": first["version"]}, map[string]any{"contentId": secondID, "version": 1}, map[string]any{"contentId": deletedID, "version": deleted["version"]}}}))
	if out["successCount"] != float64(1) || out["failureCount"] != float64(2) {
		t.Fatal("batch partial result wrong")
	}
	auditFound := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		var n int
		if e := c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys_oper_log WHERE oper_id>? AND oper_url='/cms/page/batch' AND status=1 AND error_msg=?", auditBefore, "批量操作存在失败：成功1，失败2").Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n > 0 {
			auditFound = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !auditFound {
		t.Fatal("partial batch was not audited as failure")
	}
	current := cmsData(c.ok("GET", p, nil))
	duplicate := map[string]any{"contentId": firstID, "version": current["version"]}
	cmsCode(t, c.call("POST", "/cms/page/batch", c.admin, map[string]any{"action": "unpublish", "items": []any{duplicate, duplicate}}), 500)
	if cmsData(c.ok("GET", p, nil))["isPublished"] != float64(1) {
		t.Fatal("duplicate batch mutated content")
	}
	scan := cmsData(c.ok("GET", "/cms/site/inspect?pageNum=1&pageSize=20", nil))
	found := false
	foundAbsolute := false
	for _, item := range scan["rows"].([]any) {
		row := item.(map[string]any)
		if row["contentId"] == float64(firstID) {
			for _, issue := range row["issues"].([]any) {
				if issue.(map[string]any)["code"] == "INTERNAL_LINK_BROKEN" {
					found = true
					if issue.(map[string]any)["url"] == "/p/"+slug+"-absolute" {
						foundAbsolute = true
					}
				}
			}
		}
	}
	if !found || !foundAbsolute {
		data, _ := json.Marshal(scan)
		t.Fatalf("internal broken link not reported: %.500s", data)
	}
	if scan["externalLinksSkipped"].(float64) < 1 {
		t.Fatal("external link skip not reported")
	}
	c.ok("POST", "/cms/page/batch", map[string]any{"action": "unpublish", "items": []any{map[string]any{"contentId": firstID, "version": current["version"]}}})
	if cmsData(c.ok("GET", p, nil))["isPublished"] != float64(0) {
		t.Fatal("batch unpublish failed")
	}
	t.Log("PASS file-missing gate, precheck auth, per-item CAS partial batch, duplicate rejection, local broken-link inspection and external skip")
}
