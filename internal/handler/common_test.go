package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestServeDownloadRequiresCompleteGETForCleanup(t *testing.T) {
	file := filepath.Join(t.TempDir(), "report.txt")
	const content = "temporary report"
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, method, header, value string
		cancel, broken              bool
		status                      int
		complete                    bool
	}{
		{"full", http.MethodGet, "", "", false, false, 200, true},
		{"range", http.MethodGet, "Range", "bytes=0-2", false, false, 206, false},
		{"unsatisfiable_range", http.MethodGet, "Range", "bytes=999-1000", false, false, 416, false},
		{"head", http.MethodHead, "", "", false, false, 200, false},
		{"not_modified", http.MethodGet, "If-Modified-Since", info.ModTime().UTC().Add(time.Second).Format(http.TimeFormat), false, false, 304, false},
		{"canceled", http.MethodGet, "", "", true, false, 200, false},
		{"failed_write", http.MethodGet, "", "", false, true, 200, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			var writer http.ResponseWriter = recorder
			if tc.broken {
				writer = failedDownloadWriter{recorder}
			}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(tc.method, "/download", nil)
			if tc.header != "" {
				c.Request.Header.Set(tc.header, tc.value)
			}
			if tc.cancel {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			if got := serveDownload(c, file, "report.txt"); got != tc.complete {
				t.Fatalf("complete=%v, want %v", got, tc.complete)
			}
			if c.Writer.Status() != tc.status {
				t.Fatalf("HTTP %d, want %d", c.Writer.Status(), tc.status)
			}
			if tc.complete && recorder.Body.String() != content {
				t.Fatal("incomplete bytes")
			}
			if _, err := os.Stat(file); err != nil {
				t.Fatal("streaming helper must not delete files", err)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodGet, "/download", nil)
		if serveDownload(c, file+".missing", "report.txt") {
			t.Fatal("missing file completed")
		}
		if r.Header().Get("Content-Type") != "application/json" {
			t.Fatal("download error must be exact application/json")
		}
	})
}

type failedDownloadWriter struct{ *httptest.ResponseRecorder }

func (failedDownloadWriter) Write([]byte) (int, error) {
	return 0, errors.New("test connection closed")
}
