package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestThemeLayoutValidation(t *testing.T) {
	base := model.SiteConfig{Theme: "industrial", SiteName: "Test", Palette: "earth", Density: "normal"}
	if v, e := ValidateSite(base); e != nil || v != "v1" {
		t.Fatal(v, e)
	}
	for _, mutate := range []func(*model.SiteConfig){func(c *model.SiteConfig) { c.ListingLayout = "grid;bad" }, func(c *model.SiteConfig) { c.ChannelLayouts = []model.ChannelLayout{{ChannelId: 1, Layout: "rawcss"}} }, func(c *model.SiteConfig) {
		c.ChannelLayouts = []model.ChannelLayout{{ChannelId: 1, Layout: "list"}, {ChannelId: 1, Layout: "grid"}}
	}, func(c *model.SiteConfig) { c.ChannelLayouts = make([]model.ChannelLayout, 101) }} {
		c := base
		mutate(&c)
		if _, e := ValidateSite(c); e == nil {
			t.Fatal("invalid layout allowed")
		}
	}
}
