package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPrivateLocalStorage(t *testing.T) {
	ctx := context.Background()
	store := Local{Path: t.TempDir()}
	key, err := store.Put(ctx, "png", []byte("private image"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(file)
	file.Close()
	if string(data) != "private image" {
		t.Fatal("wrong bytes")
	}
	for _, key := range []string{"../secret.png", `C:\secret.png`, "https://example.com/file.png", "original.png", "/etc/passwd"} {
		if file, err := store.Open(ctx, key); err == nil {
			file.Close()
			t.Fatal("unsafe path accepted")
		}
	}
	if err := store.Remove(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, key); err == nil {
		t.Fatal("removed file remains")
	}
}
func TestStorageRefusesEscapingLink(t *testing.T) {
	store := Local{Path: t.TempDir()}
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	key := "0123456789abcdef0123456789abcdef.png"
	if err := os.Symlink(outside, filepath.Join(store.Path, key)); err != nil {
		t.Skip("OS does not allow test symlinks")
	}
	if file, err := store.Open(context.Background(), key); err == nil {
		file.Close()
		t.Fatal("escaped storage root through a link")
	}
}

func TestPutVerifiedResumesWithoutReplacingDifferentBytes(t *testing.T) {
	store := Local{Path: filepath.Join(t.TempDir(), "copies")}
	key := "0123456789abcdef0123456789abcdef.png"
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := store.PutVerified(ctx, key, []byte("planned image")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PutVerified(ctx, key, []byte("other image")); err == nil {
		t.Fatal("different plan overwrote existing file")
	}
	f, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != "planned image" {
		t.Fatal("original bytes changed")
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.PutVerified(stopped, key, []byte("planned image")); err == nil {
		t.Fatal("cancel ignored")
	}
	blocked := "abcdef0123456789abcdef0123456789.png"
	if err := os.Mkdir(filepath.Join(store.Path, blocked), 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.PutVerified(ctx, blocked, []byte("image")); err == nil {
		t.Fatal("invalid destination overwritten")
	}
}

func TestPutVerifiedConcurrentConflictingPlansNeverOverwrite(t *testing.T) {
	store := Local{Path: t.TempDir()}
	ctx := context.Background()
	key := "0123456789abcdef0123456789abcdef.png"
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan []byte, 2)
	for _, data := range [][]byte{[]byte("first plan"), []byte("other plan")} {
		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			<-start
			if err := store.PutVerified(ctx, key, data); err == nil {
				results <- data
			}
		}(data)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := [][]byte{}
	for data := range results {
		winners = append(winners, data)
	}
	if len(winners) != 1 {
		t.Fatal("conflicting plans both succeeded")
	}
	f, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != string(winners[0]) {
		t.Fatal("successful plan was overwritten")
	}
}
