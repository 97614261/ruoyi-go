package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"strings"
	"testing"
)

func TestRepeatedHomeSectionsKeepValidation(t *testing.T) {
	cfg := model.SiteConfig{Theme: "business", SiteName: "test", Palette: "earth", Density: "normal", Blocks: []model.SiteBlock{{Kind: "ARTICLE", ChannelId: 1, Count: 2}, {Kind: "ARTICLE", ChannelId: 2, Count: 5}, {Kind: "PRODUCT", Count: 3}, {Kind: "PRODUCT", Count: 4}, {Kind: "PAGE", Count: 2}, {Kind: "PAGE", Count: 1}}}
	if _, e := ValidateSite(cfg); e != nil {
		t.Fatal("independent repeated sections rejected", e)
	}
	cfg.Blocks = append(cfg.Blocks, model.SiteBlock{Kind: "ARTICLE", Count: 13})
	if _, e := ValidateSite(cfg); e == nil {
		t.Fatal("per-section limit lost")
	}
	cfg.Blocks = make([]model.SiteBlock, 16)
	if _, e := ValidateSite(cfg); e == nil {
		t.Fatal("total section cap lost")
	}
}
func TestSiteReferenceRejectsInvalidIDsAndScope(t *testing.T) {
	for _, ids := range []string{"0", "-1", "1,1", "1 OR 1=1", "9223372036854775808", strings.Repeat("1,", 100) + "2"} {
		if _, _, e := CMSSiteReferences(context.Background(), model.CMSSiteReferenceQuery{Type: "content", IDs: ids}, []string{"ARTICLE"}, page.Query{}); e == nil {
			t.Fatal("bad IDs accepted", ids)
		}
	}
	for _, q := range []model.CMSSiteReferenceQuery{{Type: "content", Kind: "PRODUCT"}, {Type: "channel", Kind: "PAGE"}, {Type: "sql"}} {
		if _, _, e := CMSSiteReferences(context.Background(), q, []string{"ARTICLE"}, page.Query{}); e == nil {
			t.Fatal("bad kind/scope accepted")
		}
	}
}
