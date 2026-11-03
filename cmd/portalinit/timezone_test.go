package main

import (
	mysql "github.com/go-sql-driver/mysql"
	"testing"
)

func TestStandaloneDSNTimeZone(t *testing.T) {
	t.Setenv("ZONEINFO", t.TempDir())
	t.Setenv("GOROOT", t.TempDir())
	cfg, err := mysql.ParseDSN("test:unused@tcp(127.0.0.1:3306)/ruoyi_go_portal?parseTime=true&loc=Asia%2FShanghai")
	if err != nil {
		t.Fatalf("standalone DSN time zone unavailable: %v", err)
	}
	if cfg.Loc.String() != "Asia/Shanghai" {
		t.Fatal("DSN time zone changed")
	}
}
