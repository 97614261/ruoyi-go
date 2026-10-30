package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestSiteSettingsRejectUnsafeConfiguration(t *testing.T) {
	base := model.SiteConfig{Theme: "business", SiteName: "Test", Palette: "earth", Density: "normal", Blocks: []model.SiteBlock{{Kind: "ARTICLE", Count: 6, Enabled: true}}}
	for _, mutate := range []func(*model.SiteConfig){func(c *model.SiteConfig) { c.Theme = "../outside" }, func(c *model.SiteConfig) { c.SiteName = "<script>" }, func(c *model.SiteConfig) { c.Palette = "red;position:fixed" }, func(c *model.SiteConfig) { c.IconAssetId = -1 }, func(c *model.SiteConfig) { c.FontScale = "arbitrary" }, func(c *model.SiteConfig) { c.Navigation = []model.SiteLink{{Label: "x", URL: "javascript:alert(1)"}} }, func(c *model.SiteConfig) { c.Navigation = []model.SiteLink{{Label: "x", URL: "//evil.example"}} }, func(c *model.SiteConfig) {
		c.Navigation = []model.SiteLink{{Label: "x", URL: "https://user:secret@example.com"}}
	}, func(c *model.SiteConfig) { c.Navigation = []model.SiteLink{{Label: "x", URL: `/\\evil.example`}} }, func(c *model.SiteConfig) { c.Blocks = []model.SiteBlock{{Kind: "ARTICLE", Count: 100}} }} {
		c := base
		mutate(&c)
		if _, err := ValidateSite(c); err == nil {
			t.Fatal("unsafe configuration accepted", c)
		}
	}
	for _, theme := range []string{"business", "editorial"} {
		c := base
		c.Theme = theme
		if _, err := ValidateSite(c); err != nil {
			t.Fatal(err)
		}
	}
}
