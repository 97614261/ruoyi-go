package portalsql

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"strings"
)

const CurrentVersion = 6

//go:embed 001_cms_foundation.sql 002_content_revisions.sql 003_product.sql 004_site.sql 005_schedule.sql
var migrations embed.FS

func Step2SQL() string {
	body, _ := migrations.ReadFile("001_cms_foundation.sql")
	return strings.ReplaceAll(string(body), "\r\n", "\n")
}
func MigrationSQL(version int) string {
	if version == 6 {
		b, _ := migrations.ReadFile("005_schedule.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 5 {
		b, _ := migrations.ReadFile("004_site.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 4 {
		body, _ := migrations.ReadFile("003_product.sql")
		return strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	if version == 2 {
		return Step2SQL()
	}
	if version == 3 {
		body, _ := migrations.ReadFile("002_content_revisions.sql")
		return strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	return ""
}
func SourceSHA256(version int) string {
	if version == 6 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(5)+"\n"+MigrationSQL(6))))
	}
	if version == 5 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(4)+"\n"+MigrationSQL(5))))
	}
	if version == 4 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(3)+"\n"+MigrationSQL(4))))
	}
	if version == 3 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(2)+"\n"+MigrationSQL(3))))
	}
	if version == 1 {
		return BaselineSHA256()
	}
	if version == 2 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(BaselineSHA256()+"\n"+Step2SQL())))
	}
	return ""
}
