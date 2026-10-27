package main

import (
	"bytes"
	"strings"
	"testing"

	"ruoyi-go/internal/config"
)

func TestPreviewDoesNotReadCredentialsOrConfig(t *testing.T) {
	var out bytes.Buffer
	if e := run([]string{"-config", "missing.yml", "-token-file", "secret-never-opened.json"}, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), `"execute":false`) || strings.Contains(out.String(), "secret-never-opened") {
		t.Fatal("preview read or exposed credentials")
	}
}
func TestRejectsRemotePathsAndUnboundedLoad(t *testing.T) {
	for _, args := range [][]string{{"-url", "http://example.com"}, {"-url", "http://127.0.0.1:8080"}, {"-path", "http://example.com/erp/order/list"}, {"-path", "/tool/gen/list"}, {"-path", "/erp/../system/user"}, {"-c", "257"}, {"-duration", "5m"}, {"-n", "0"}, {"-timeout", "40s"}} {
		if _, e := parse(args); e == nil {
			t.Fatal("unsafe plan accepted", args)
		}
	}
}
func TestRequiresDedicatedDatabaseAndRedis(t *testing.T) {
	cfg := &config.Config{}
	cfg.MySQL.DSN = "test:test@tcp(127.0.0.1:3306)/ruoyi_go_mt"
	cfg.Redis.Addr = "127.0.0.1:6379"
	cfg.Redis.DB = 13
	if !localConfig(cfg) {
		t.Fatal("dedicated local environment rejected")
	}
	for _, dsn := range []string{"test:test@tcp(127.0.0.1:3306)/ry", "test:test@tcp(example.com:3306)/ruoyi_go_mt"} {
		cfg.MySQL.DSN = dsn
		if localConfig(cfg) {
			t.Fatal("original/remote database accepted")
		}
	}
	cfg.MySQL.DSN = "test:test@tcp(127.0.0.1:3306)/ruoyi_go_mt"
	cfg.Redis.DB = 0
	if localConfig(cfg) {
		t.Fatal("shared Redis accepted")
	}
}
