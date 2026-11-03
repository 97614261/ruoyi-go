package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestCollaborationMigrationOptIn(t *testing.T) {
	s := MigrationSQL(14)
	if len(regexp.MustCompile(`(?im)^CREATE TABLE `).FindAllString(s, -1)) != 9 {
		t.Fatal("workflow table count")
	}
	for _, bad := range []string{"ALTER TABLE sys_", "UPDATE sys_user", "INSERT INTO sys_role_menu", "DROP TABLE", "ALTER TABLE cms_content"} {
		if strings.Contains(s, bad) {
			t.Fatal("migration crossed frozen/grant boundary", bad)
		}
	}
	if !strings.Contains(s, "required TINYINT NOT NULL DEFAULT 0") || !strings.Contains(s, "inquiry_isolation TINYINT NOT NULL DEFAULT 0") || !strings.Contains(s, "uk_cms_outbox_event") {
		t.Fatal("defaults/deduplication missing")
	}
}
