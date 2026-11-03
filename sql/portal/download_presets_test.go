package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestDownloadPresetMigrationBoundary(t *testing.T) {
	s := MigrationSQL(16)
	if len(regexp.MustCompile(`(?im)^CREATE TABLE `).FindAllString(s, -1)) != 5 {
		t.Fatal("expected five extension tables")
	}
	for _, bad := range []string{"ALTER TABLE ", "DROP TABLE", "TRUNCATE ", "UPDATE sys_", "INSERT INTO sys_role_menu"} {
		if strings.Contains(s, bad) {
			t.Fatal("crossed schema/grant boundary", bad)
		}
	}
	for _, required := range []string{"working_revision_id", "published_revision_id", "idx_cms_dlrev_file", "idx_cms_dlrev_category", "uk_cms_preset_request(actor_user_id,request_id)", "before_config_json", "cms:download:publish", "cms:preset:apply"} {
		if !strings.Contains(s, required) {
			t.Fatal("missing boundary", required)
		}
	}
}
