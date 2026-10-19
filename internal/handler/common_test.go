package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/tenantctx"
)

func TestRollbackUploadsRemovesOnlyCurrentBatch(t *testing.T) {
	previous := uploadCfg
	oldForget := forgetUploadedFile
	forgetUploadedFile = func(context.Context, string) error { return nil }
	t.Cleanup(func() { forgetUploadedFile = oldForget })
	base := t.TempDir()
	uploadCfg = config.UploadConfig{Path: base, URLPrefix: "/profile"}
	t.Cleanup(func() { uploadCfg = previous })

	batchFile := filepath.Join(base, "tenants", "10000", "upload", "2026", "08", "28", "batch.txt")
	keepFile := filepath.Join(base, "tenants", "10000", "upload", "2026", "08", "28", "keep.txt")
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
	c.Request = c.Request.WithContext(tenantctx.With(c.Request.Context(), 10000, false))
	rollbackUploads(c, []string{"/profile/tenants/10000/upload/2026/08/28/batch.txt"})
	if _, err := os.Stat(batchFile); !os.IsNotExist(err) {
		t.Fatalf("batch file should be rolled back: %v", err)
	}
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("unrelated file must remain: %v", err)
	}
}
