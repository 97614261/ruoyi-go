package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestPortalMigrationsAreAdditiveAndVersioned(t *testing.T) {
	if CurrentVersion != 16 {
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

func TestThemeCapabilityMigrationDoesNotMutateSchemaOrGrants(t *testing.T) {
	s := MigrationSQL(9)
	for _, bad := range []string{"CREATE TABLE", "ALTER TABLE", "INSERT INTO sys_role_menu", "UPDATE cms_site", "DROP TABLE"} {
		if strings.Contains(s, bad) {
			t.Fatal("template marker changed business data or schema", bad)
		}
	}
	if !strings.Contains(s, "DO 0;") {
		t.Fatal("explicit marker missing")
	}
}

func TestGrowthMigrationKeepsBaselineAndGrants(t *testing.T) {
	s := MigrationSQL(13)
	if len(regexp.MustCompile(`(?im)^CREATE TABLE `).FindAllString(s, -1)) != 4 {
		t.Fatal("growth table count")
	}
	for _, bad := range []string{"ALTER TABLE sys_", "UPDATE sys_user", "INSERT INTO sys_role_menu", "DROP TABLE", "ALTER TABLE cms_content"} {
		if strings.Contains(s, bad) {
			t.Fatal("growth crossed boundary", bad)
		}
	}
}
