package portalsql

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"strings"
)

const CurrentVersion = 16

//go:embed 001_cms_foundation.sql 002_content_revisions.sql 003_product.sql 004_site.sql 005_schedule.sql 006_inquiry.sql 007_transfer.sql 008_themes.sql 009_asset_groups.sql 010_redirect.sql 011_recovery.sql 012_growth.sql 013_collaboration.sql 014_operations.sql 015_download_presets.sql
var migrations embed.FS

func Step2SQL() string {
	body, _ := migrations.ReadFile("001_cms_foundation.sql")
	return strings.ReplaceAll(string(body), "\r\n", "\n")
}
func MigrationSQL(version int) string {
	if version == 16 {
		b, _ := migrations.ReadFile("015_download_presets.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 15 {
		b, _ := migrations.ReadFile("014_operations.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 14 {
		b, _ := migrations.ReadFile("013_collaboration.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 13 {
		b, _ := migrations.ReadFile("012_growth.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 12 {
		b, _ := migrations.ReadFile("011_recovery.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 11 {
		b, _ := migrations.ReadFile("010_redirect.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 10 {
		b, _ := migrations.ReadFile("009_asset_groups.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 9 {
		b, _ := migrations.ReadFile("008_themes.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 8 {
		b, _ := migrations.ReadFile("007_transfer.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if version == 7 {
		b, _ := migrations.ReadFile("006_inquiry.sql")
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
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
	if version == 16 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(15)+"\n"+MigrationSQL(16))))
	}
	if version == 15 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(14)+"\n"+MigrationSQL(15))))
	}
	if version == 14 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(13)+"\n"+MigrationSQL(14))))
	}
	if version == 13 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(12)+"\n"+MigrationSQL(13))))
	}
	if version == 12 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(11)+"\n"+MigrationSQL(12))))
	}
	if version == 11 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(10)+"\n"+MigrationSQL(11))))
	}
	if version == 10 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(9)+"\n"+MigrationSQL(10))))
	}
	if version == 9 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(8)+"\n"+MigrationSQL(9))))
	}
	if version == 8 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(7)+"\n"+MigrationSQL(8))))
	}
	if version == 7 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(SourceSHA256(6)+"\n"+MigrationSQL(7))))
	}
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
