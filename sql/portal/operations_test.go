package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestOperationsMigrationPreservesBaselineAndDefaults(t *testing.T) {
	s := MigrationSQL(15)
	if len(regexp.MustCompile(`(?im)^CREATE TABLE `).FindAllString(s, -1)) != 8 {
		t.Fatal("operations tables")
	}
	for _, bad := range []string{"ALTER TABLE ", "DROP TABLE", "TRUNCATE ", "UPDATE sys_", "INSERT INTO sys_role_menu"} {
		if strings.Contains(s, bad) {
			t.Fatal("operations crossed frozen/grant boundary", bad)
		}
	}
	for _, required := range []string{"retention_days INT NOT NULL DEFAULT 0", "PRIMARY KEY(token_hash,content_id)", "idx_cms_clock_retention", "source_version BIGINT", "file_sha256 CHAR(64)", "'cms:inquiry:exportPrivate'", "'cms:data:cleanup'"} {
		if !strings.Contains(s, required) {
			t.Fatal("missing guard", required)
		}
	}
}
