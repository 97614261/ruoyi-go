package main

import (
	mysql "github.com/go-sql-driver/mysql"
	"ruoyi-go/internal/config"
	"strings"
	"testing"
)

func TestRecoveryRequiresExactLocalTarget(t *testing.T) {
	cfg := &config.Config{}
	cfg.Redis.DB = 14
	cfg.Server.Port = 8085
	cfg.Portal.ListenHost = "127.0.0.1"
	d := &mysql.Config{Net: "tcp", Addr: "127.0.0.1:3306", DBName: "ruoyi_go_portal"}
	if !recoveryTargetAllowed(cfg, d) {
		t.Fatal("local target rejected")
	}
	for _, mutate := range []func(*config.Config, *mysql.Config){func(c *config.Config, d *mysql.Config) { c.Server.Port = 80 }, func(c *config.Config, d *mysql.Config) { c.Portal.ListenHost = "0.0.0.0" }, func(c *config.Config, d *mysql.Config) { d.Addr = "example.com:3306" }, func(c *config.Config, d *mysql.Config) { d.DBName = "ruoyi_go_mt" }, func(c *config.Config, d *mysql.Config) { d.Net = "unix" }} {
		c := *cfg
		copy := *d
		mutate(&c, &copy)
		if recoveryTargetAllowed(&c, &copy) {
			t.Fatal("nonlocal target accepted")
		}
	}
}
func TestClientOptionsAreDerivedAndQuoted(t *testing.T) {
	s := string(recoveryClientOptions(&mysql.Config{User: "test", Passwd: "a\"b\nc"}))
	if !strings.Contains(s, "host=127.0.0.1\nport=3306\nprotocol=tcp") || !strings.Contains(s, `password="a\"b\nc"`) {
		t.Fatal("client options not safely quoted")
	}
}
