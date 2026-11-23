package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleManifestDetectsCorruptionAndUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	data := []byte("fixture")
	entries := []bundleEntry{}
	for _, name := range []string{"database.sql", "config.snapshot.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, bundleEntry{Path: name, Bytes: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	write := func(files []bundleEntry) {
		t.Helper()
		raw, _ := json.Marshal(bundleManifest{Format: 1, Files: files})
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(entries)
	if _, err := verifyBundle(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	changed := append([]bundleEntry(nil), entries...)
	changed[0].Path = "../outside.sql"
	write(changed)
	if _, err := verifyBundle(context.Background(), dir); err == nil {
		t.Fatal("traversal accepted")
	}
	write(append(entries, entries[0]))
	if _, err := verifyBundle(context.Background(), dir); err == nil {
		t.Fatal("duplicate accepted")
	}
	write(entries)
	if err := os.WriteFile(filepath.Join(dir, "database.sql"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(context.Background(), dir); err == nil {
		t.Fatal("same-length corruption not detected")
	}
}

func TestBundlePathsOnlyAcceptGeneratedFiles(t *testing.T) {
	for _, name := range []string{"/database.sql", "storage/../config.snapshot.yml", "storage/a.png", "storage/012345678901234567890123456789ab.png/other", "storage\\012345678901234567890123456789ab.png"} {
		if allowedBundlePath(name) {
			t.Fatal("unsafe path", name)
		}
	}
	if !allowedBundlePath("storage/012345678901234567890123456789ab.png") {
		t.Fatal("valid asset rejected")
	}
	if !allowedBundlePath("storage/catalog/012345678901234567890123456789ab.png") {
		t.Fatal("valid platform asset rejected")
	}
	for _, name := range []string{"storage/catalog/../012345678901234567890123456789ab.png", "storage/catalog/catalog/012345678901234567890123456789ab.png", "storage/catalog\\012345678901234567890123456789ab.png"} {
		if allowedBundlePath(name) {
			t.Fatal("unsafe platform path", name)
		}
	}
}

func TestBundleCatalogFilesAndCorruption(t *testing.T) {
	dir := t.TempDir()
	key := "012345678901234567890123456789ab.png"
	receipt := "012345678901234567890123456789ab.podreceipt"
	if err := os.MkdirAll(filepath.Join(dir, "storage", "catalog"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "storage", "receipts"), 0700); err != nil {
		t.Fatal(err)
	}
	entries := []bundleEntry{}
	for _, name := range []string{"database.sql", "config.snapshot.yml", "storage/" + key, "storage/catalog/" + key, "storage/receipts/" + receipt} {
		data := []byte(name)
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), data, 0600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, bundleEntry{Path: name, Bytes: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	raw, _ := json.Marshal(bundleManifest{Format: 1, Files: entries})
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	files, err := bundleStorageFiles(filepath.Join(dir, "storage"))
	if err != nil || len(files) != 3 {
		t.Fatal("private storage namespaces were not all included", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "storage", "catalog", key), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyBundle(context.Background(), dir); err == nil {
		t.Fatal("platform corruption accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "storage", "unexpected"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := bundleStorageFiles(filepath.Join(dir, "storage")); err == nil {
		t.Fatal("unexpected directory accepted")
	}
}
