package portalsql

import (
	"regexp"
	"strings"
	"testing"
)

func TestInstallSQLHasTwentyTablesAndNoDestructiveOrCrossDatabaseStatements(t *testing.T) {
	sql := InstallSQL()
	if got := len(regexp.MustCompile(`(?im)^create table `).FindAllString(sql, -1)); got != 20 {
		t.Fatalf("baseline table count %d", got)
	}
	if regexp.MustCompile(`(?im)^\s*(drop|use|truncate|alter|delete|update)\s`).MatchString(sql) {
		t.Fatal("unsafe install SQL")
	}
	for _, table := range []string{"sys_user", "sys_notice_read", "gen_table_column"} {
		if !strings.Contains(sql, "create table "+table) {
			t.Fatal("missing required baseline table")
		}
	}
	if len(BaselineSHA256()) != 64 {
		t.Fatal("baseline fingerprint missing")
	}
}
