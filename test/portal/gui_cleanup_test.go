package portaltest

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestCMSCleanupGUIFixture(t *testing.T) {
	parts := strings.Split(os.Getenv("PORTAL_GUI_CLEANUP_IDS"), ",")
	if len(parts) != 2 {
		t.Skip("explicit owned GUI fixture IDs required")
	}
	channel, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || channel < 1 {
		t.Fatal("invalid channel ID")
	}
	asset, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || asset < 1 {
		t.Fatal("invalid asset ID")
	}
	c := cmsSetup(t)
	channelPath := "/cms/channel/" + parts[0]
	assetPath := "/cms/asset/" + parts[1]
	out := c.ok("GET", channelPath, nil)
	row := cmsData(out)
	if row["name"] != "页面验收栏目" || row["slug"] != "ui-proof-channel" || row["createBy"] != "admin" || row["coverAssetId"] != float64(asset) || row["version"] != float64(1) {
		t.Fatal("GUI channel ownership/version changed; refuse cleanup")
	}
	image := cmsData(c.ok("GET", assetPath, nil))
	if image["name"] != "ui-proof.png" || image["altText"] != "页面验收图片" || image["version"] != float64(2) {
		t.Fatal("GUI image ownership/version changed; refuse cleanup")
	}
	c.ok("DELETE", channelPath, map[string]any{"version": 1})
	c.ok("DELETE", assetPath, map[string]any{"version": 2})
	cmsCode(t, c.call("GET", channelPath, c.admin, nil), 500)
	cmsCode(t, c.call("GET", assetPath, c.admin, nil), 500)
	t.Logf("removed own GUI channel %d and private test asset %d; no existing content touched", channel, asset)
}
