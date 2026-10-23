package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPurchaseLogDropsAllQueryValues(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var b bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&b, nil)))
	r := gin.New()
	r.Use(Logger())
	r.GET("/erp/purchase/list", func(c *gin.Context) { c.Status(200) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/erp/purchase/list?unknown=PRIVATE_PURCHASE_AMOUNT&purchaseNo=PRIVATE_PURCHASE_NUMBER", nil))
	if strings.Contains(b.String(), "PRIVATE_PURCHASE") {
		t.Fatal("private query logged")
	}
	if !strings.Contains(b.String(), "/erp/purchase/list") {
		t.Fatal("metadata absent")
	}
}
