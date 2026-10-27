package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickingLoggerDropsBarcodeAndUnknownQuery(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var b bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&b, nil)))
	r := gin.New()
	r.Use(Logger())
	r.GET("/erp/picking/list", func(c *gin.Context) { c.Status(200) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/erp/picking/list?scanCode=PRIVATE_BARCODE&unknown=PRIVATE_PAYLOAD", nil))
	if strings.Contains(b.String(), "PRIVATE_") || !strings.Contains(b.String(), "/erp/picking/list") {
		t.Fatal("query/body leaked or request metadata missing")
	}
}
