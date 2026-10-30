package config

import "testing"

func TestPortalIsolationIsOptInAndValidated(t *testing.T) {
	base, err := Load("../../configs/application.yml")
	if err != nil {
		t.Fatal(err)
	}
	if base.Portal.Enabled {
		t.Fatal("legacy config unexpectedly enabled portal")
	}
	valid := *base
	valid.Portal = PortalConfig{Enabled: true, Theme: "business", SiteName: "Portal", PublicURL: "http://127.0.0.1:8085"}
	valid.MySQL.DSN = "test:test@tcp(127.0.0.1:3306)/ruoyi_go_portal"
	valid.Redis.DB = 14
	valid.Upload.Path = "./uploadPath-portal-public"
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Portal.PublicURL = "https://user:password@example.com/path" },
		func(c *Config) { c.Portal.PublicURL = "javascript:alert(1)" },
		func(c *Config) { c.MySQL.DSN = "test:test@tcp(127.0.0.1:3306)/ruoyi_go_mt" },
		func(c *Config) { c.Redis.DB = 13 },
		func(c *Config) { c.Upload.Path = "./uploadPath" },
		func(c *Config) { c.Portal.Theme = "../business" },
	} {
		c := valid
		mutate(&c)
		if err := c.validate(); err == nil {
			t.Fatal("nonisolated portal config accepted")
		}
	}
}
