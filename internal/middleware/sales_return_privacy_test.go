package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSalesReturnLoggerDropsUnknownQuery(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var b bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&b, nil)))
	r := gin.New()
	r.Use(Logger())
	r.GET("/erp/salesReturnOrder/list", func(c *gin.Context) { c.Status(200) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/erp/salesReturnOrder/list?unknown=PRIVATE_SALES_RETURN_REASON", nil))
	if strings.Contains(b.String(), "PRIVATE_SALES_RETURN_REASON") || !strings.Contains(b.String(), "/erp/salesReturnOrder/list") {
		t.Fatal("private query or missing metadata")
	}
}
