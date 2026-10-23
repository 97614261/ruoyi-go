package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSupplierHTTPLogDropsAllQueryValues(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var buffer bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	r := gin.New()
	r.Use(Logger())
	r.GET("/erp/supplier/list", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/erp/supplier/list?unexpected=private-phone-value&supplierName=private-name", nil))
	if strings.Contains(buffer.String(), "private-phone-value") || strings.Contains(buffer.String(), "private-name") {
		t.Fatal("supplier filter/unknown PII was logged")
	}
	if !strings.Contains(buffer.String(), "/erp/supplier/list") {
		t.Fatal("request metadata missing")
	}
}
