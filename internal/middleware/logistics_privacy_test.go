package middleware

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogisticsLoggerDropsTrackingAndUnknownQuery(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)
	var b bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&b, nil)))
	r := gin.New()
	r.Use(Logger())
	r.GET("/erp/logistics/shipments", func(c *gin.Context) { c.Status(200) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/erp/logistics/shipments?trackingNo=PRIVATE_TRACKING&unknown=PRIVATE_LOGISTICS", nil))
	if strings.Contains(b.String(), "PRIVATE_") || !strings.Contains(b.String(), "/erp/logistics/shipments") {
		t.Fatal("sensitive query or missing metadata")
	}
}
