package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase1CLIRefusesOriginalRemoteAndRestore(t *testing.T) {
	for _, s := range []string{"bad", "root@tcp(192.0.2.1:3306)/ruoyi_go_mt", "root@tcp(127.0.0.1:3306)/ry", "root@unix(/mysql.sock)/ruoyi_go_mt", "root@tcp(127.0.0.1:3306)/ruoyi_go_mt_restore_12345678"} {
		if validatePhase1DSN(s) == nil {
			t.Fatal("unsafe DSN accepted")
		}
	}
	if err := validatePhase1DSN("root@tcp(127.0.0.1:3306)/ruoyi_go_mt"); err != nil {
		t.Fatal(err)
	}
}
func TestPhase1BackupEvidenceGate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	now := time.Now().UTC()
	dump := []byte("reviewed local backup fixture")
	if err := os.WriteFile(filepath.Join(dir, "database.sql"), dump, 0600); err != nil {
		t.Fatal(err)
	}
	checks := map[string]bool{}
	for _, k := range []string{"nativeTransactionalDump", "sourceStableDuringBackup", "allTableSchemasAndRowHashes", "uploadCopyHashes", "registeredAttachmentsPresentAndExact"} {
		checks[k] = true
	}
	report := map[string]any{"sourceDatabase": "ruoyi_go_mt", "restoredDatabase": "ruoyi_go_mt_restore_test_12345678", "completed": now, "dumpSHA256": fmt.Sprintf("%x", sha256.Sum256(dump)), "dumpBytes": len(dump), "tablesVerified": 32, "checks": checks}
	write := func() {
		t.Helper()
		raw, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := validatePhase1Backup(path, now); err != nil {
		t.Fatal(err)
	}
	if validatePhase1Backup("", now) == nil {
		t.Fatal("no evidence accepted")
	}
	report["completed"] = now.Add(-25 * time.Hour)
	write()
	if validatePhase1Backup(path, now) == nil {
		t.Fatal("stale evidence accepted")
	}
	report["completed"] = now
	checks["allTableSchemasAndRowHashes"] = false
	write()
	if validatePhase1Backup(path, now) == nil {
		t.Fatal("failed restore accepted")
	}
	checks["allTableSchemasAndRowHashes"] = true
	report["sourceDatabase"] = "ry"
	write()
	if validatePhase1Backup(path, now) == nil {
		t.Fatal("original database proof accepted")
	}
	report["sourceDatabase"] = "ruoyi_go_mt"
	write()
	os.WriteFile(filepath.Join(dir, "database.sql"), []byte("tampered"), 0600)
	if validatePhase1Backup(path, now) == nil {
		t.Fatal("modified dump accepted")
	}
}
