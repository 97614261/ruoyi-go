package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestContentOpsBoundary(t *testing.T) {
	for _, o := range []model.CMSContentOps{{PinOrder: -1}, {ShareAssetId: -1}, {TagIds: []int64{1, 1}}, {RelatedIds: []int64{0}}, {ShareTitle: "<script>"}, {ShareDescription: strings.Repeat("中", 301)}} {
		if ValidateContentOps(o) == nil {
			t.Fatal("invalid operations accepted", o)
		}
	}
	if e := ValidateContentOps(model.CMSContentOps{TagIds: []int64{1, 2}, RelatedIds: []int64{3}, ShareTitle: "正常分享"}); e != nil {
		t.Fatal(e)
	}
}
func TestVisitPrivacyAndOptIn(t *testing.T) {
	cfg := model.SiteConfig{}
	if len(VisitDimensions("/news/one", "https://user:pass@bad.example/private?token=secret", "personal", cfg)) != 0 {
		t.Fatal("recorded while disabled")
	}
	cfg.AnalyticsEnabled = true
	rows := VisitDimensions("/news/one", "https://example.org/private?token=secret", "personal", cfg)
	if len(rows) != 3 || rows[2].Value != "example.org" {
		t.Fatal("source not reduced to hostname")
	}
	for _, r := range rows {
		if strings.Contains(r.Value, "secret") || r.Dimension == "keyword" {
			t.Fatal("private detail persisted")
		}
	}
	cfg.AnalyticsKeywords = true
	if len(VisitDimensions("/search", "", "词语", cfg)) != 4 {
		t.Fatal("opted-in search missing")
	}
	if len(VisitDimensions("/search", "", "bad\nquery", cfg)) != 3 {
		t.Fatal("control character stored")
	}
	if _, e := NormalizeVisitQuery(model.CMSVisitQuery{From: "2026-01-01", To: "2026-12-31"}); e == nil {
		t.Fatal("unbounded query")
	}
}
func TestTypedHomeWidgets(t *testing.T) {
	cfg := model.SiteConfig{Theme: "business", SiteName: "测试站点", Palette: "earth", Density: "normal", Blocks: []model.SiteBlock{{Kind: "FAQ", Enabled: true, Items: []model.SiteWidgetItem{{Title: "如何联系", Text: "填写留言"}}}}}
	if _, e := ValidateSite(cfg); e != nil {
		t.Fatal(e)
	}
	cfg.Blocks[0].Items[0].Text = "<script>"
	if _, e := ValidateSite(cfg); e == nil {
		t.Fatal("widget script accepted")
	}
	cfg.Blocks[0].Items[0].Text = "正常"
	cfg.Blocks[0].ChannelId = 1
	if _, e := ValidateSite(cfg); e == nil {
		t.Fatal("invalid widget channel accepted")
	}
}
