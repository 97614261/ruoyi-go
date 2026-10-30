// Package portalsql embeds the reviewed single-tenant baseline. It executes no SQL at import or startup.
package portalsql

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed 000_ruoyi_base.sql
var source embed.FS

func Baseline() []byte {
	body, _ := source.ReadFile("000_ruoyi_base.sql")
	return []byte(strings.ReplaceAll(string(body), "\r\n", "\n"))
}
func BaselineSHA256() string { return fmt.Sprintf("%x", sha256.Sum256(Baseline())) }

// InstallSQL is only for a database just created by the installer. Never replay it over existing tables.
func InstallSQL() string {
	sql := string(Baseline())
	sql = regexp.MustCompile(`(?im)^drop table if exists [a-z_]+;\s*$`).ReplaceAllString(sql, "")
	return strings.TrimSpace(sql)
}
