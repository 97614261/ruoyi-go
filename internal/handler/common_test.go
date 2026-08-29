package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/config"
)

func TestRollbackUploadsRemovesOnlyCurrentBatch(t *testing.T) {
	previous := uploadCfg
	base := t.TempDir()
	uploadCfg = config.UploadConfig{Path: base, URLPrefix: "/profile"}
	t.Cleanup(func() { uploadCfg = previous })

	batchFile := filepath.Join(base, "upload", "2026", "08", "28", "batch.txt")
	keepFile := filepath.Join(base, "upload", "2026", "08", "28", "keep.txt")
	for _, target := range []string{batchFile, keepFile} {
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("test"), 0o640); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/common/uploads", nil)
	rollbackUploads(c, []string{"/profile/upload/2026/08/28/batch.txt"})
	if _, err := os.Stat(batchFile); !os.IsNotExist(err) {
		t.Fatalf("batch file should be rolled back: %v", err)
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("unrelated file must remain: %v", err)
	}
}
