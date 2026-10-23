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

func TestPurchasingCLIRejectsUnsafeDSN(t *testing.T) {
	for _, raw := range []string{"bad", "u@tcp(192.0.2.1:3306)/ruoyi_go_mt", "u@tcp(127.0.0.1:3306)/ry", "u@unix(/mysql.sock)/ruoyi_go_mt", "u@tcp(127.0.0.1:3306)/ruoyi_go_mt_restore_test_12345678"} {
		if validateDSN(raw) == nil {
			t.Fatal("unsafe DSN accepted")
		}
	}
	for _, raw := range []string{"u@tcp(127.0.0.1:3306)/ruoyi_go_mt", "u@tcp([::1]:3306)/ruoyi_go_mt"} {
		if err := validateDSN(raw); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPurchasingBackupRequiresBothRootsAndExactDump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	dumpPath := filepath.Join(dir, "database.sql")
	dump := []byte("synthetic backup test; not customer data")
	now := time.Now().UTC()
	checks := map[string]bool{}
	for _, key := range []string{"nativeTransactionalDump", "sourceStableDuringBackup", "allTableSchemasAndRowHashes", "uploadCopyHashes", "registeredAttachmentsPresentAndExact", "privateUploadCopyHashes", "registeredPrivateAttachmentsPresentAndExact"} {
		checks[key] = true
	}
	r := map[string]any{"sourceDatabase": "ruoyi_go_mt", "restoredDatabase": "ruoyi_go_mt_restore_p4_test_12345678", "completed": now, "dumpSHA256": fmt.Sprintf("%x", sha256.Sum256(dump)), "dumpBytes": len(dump), "tablesVerified": 45, "checks": checks}
	write := func() {
		t.Helper()
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dumpPath, dump, 0600); err != nil {
		t.Fatal(err)
	}
	write()
	if err := validateBackup(path, now); err != nil {
		t.Fatal(err)
	}
	if validateBackup("", now) == nil {
		t.Fatal("missing evidence accepted")
	}
	for key := range checks {
		checks[key] = false
		write()
		if validateBackup(path, now) == nil {
			t.Fatal("missing check accepted", key)
		}
		checks[key] = true
	}
	for _, value := range []time.Time{now.Add(-25 * time.Hour), now.Add(2 * time.Minute), {}} {
		r["completed"] = value
		write()
		if validateBackup(path, now) == nil {
			t.Fatal("invalid timestamp accepted")
		}
	}
	r["completed"] = now
	for _, pair := range [][2]any{{"sourceDatabase", "ry"}, {"restoredDatabase", "ruoyi_go_mt"}, {"failure", "restore failed"}, {"tablesVerified", 0}, {"dumpBytes", 1}, {"dumpSHA256", "wrong"}} {
		key := pair[0].(string)
		old, exists := r[key]
		r[key] = pair[1]
		write()
		if validateBackup(path, now) == nil {
			t.Fatal("invalid evidence accepted", key)
		}
		if exists {
			r[key] = old
		} else {
			delete(r, key)
		}
	}
	write()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(" {}")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if validateBackup(path, now) == nil {
		t.Fatal("trailing JSON accepted")
	}
	write()
	if err := os.WriteFile(dumpPath, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if validateBackup(path, now) == nil {
		t.Fatal("tampered SQL accepted")
	}
}
