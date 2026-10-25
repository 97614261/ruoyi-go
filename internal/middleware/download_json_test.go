package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/response"
	"testing"
)

func TestDownloadErrorJSONBeforePermissionAndSuccessfulOverride(t *testing.T) {
	r := gin.New()
	r.Use(DownloadErrorJSON("/export"))
	r.POST("/export", func(c *gin.Context) {
		if c.Query("ok") == "true" {
			if e := excelx.WriteResponse(c, "物流.xlsx", "物流", []struct {
				Text string `excel:"name:文本"`
			}{{"0001"}}); e != nil {
				t.Fatal(e)
			}
			return
		}
		response.FailCode(c, 403, "拒绝")
	})
	r.GET("/other", func(c *gin.Context) { response.FailCode(c, 403, "拒绝") })
	for _, tc := range []struct{ method, path, want string }{{"POST", "/export", "application/json"}, {"POST", "/export?ok=true", excelx.ContentType}, {"GET", "/other", "application/json; charset=utf-8"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Header().Get("Content-Type") != tc.want {
			t.Fatal(tc.path, w.Header().Get("Content-Type"))
		}
	}
}
