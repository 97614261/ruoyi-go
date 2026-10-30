package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestPortalMigrationsAreAdditiveAndVersioned(t *testing.T) {
	if CurrentVersion != 6 {
		t.Fatal("update migration guard test for new explicit version")
	}
	for version := 1; version <= CurrentVersion; version++ {
		if len(SourceSHA256(version)) != 64 {
			t.Fatal("missing source fingerprint", version)
		}
	}
	sql := MigrationSQL(3)
	if len(regexp.MustCompile(`(?im)^CREATE TABLE `).FindAllString(sql, -1)) != 2 {
		t.Fatal("v3 table count")
	}
	for _, bad := range []string{"DROP TABLE", "TRUNCATE ", "ALTER TABLE sys_", "UPDATE sys_user", "INSERT INTO sys_role_menu"} {
		if strings.Contains(sql, bad) {
			t.Fatal("migration crossed frozen/permission boundary", bad)
		}
	}
	if !strings.Contains(sql, "fk_cms_content_published") || !strings.Contains(sql, "fk_cms_content_working") {
		t.Fatal("content ownership constraints missing")
	}
	if !strings.Contains(MigrationSQL(4), "ADD gallery_ids") {
		t.Fatal("v4 gallery migration missing")
	}
	if MigrationSQL(99) != "" || SourceSHA256(99) != "" {
		t.Fatal("unknown version allowed")
	}
}
