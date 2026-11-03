package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInquiryQueryPrivacy(t *testing.T) {
	var b bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&b, nil)))
	defer slog.SetDefault(old)
	r := gin.New()
	r.Use(Logger())
	r.GET("/inquiry", func(c *gin.Context) { c.Status(200) })
	r.GET("/cms/inquiry/list", func(c *gin.Context) { c.Status(200) })
	for _, path := range []string{"/inquiry", "/cms/inquiry/list", "/cms/review/list", "/cms/workflow/settings"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path+"?contact=private-inquiry-canary&message=secret-body-canary", nil))
	}
	if strings.Contains(b.String(), "private-inquiry-canary") || strings.Contains(b.String(), "secret-body-canary") {
		t.Fatal("query privacy leaked")
	}
	if !strings.Contains(b.String(), "请求完成") {
		t.Fatal("request metadata was not logged")
	}
}
