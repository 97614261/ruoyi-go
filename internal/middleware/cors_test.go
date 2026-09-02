package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSWildcardNeverAllowsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitCORS([]string{"*"})
	t.Cleanup(func() { InitCORS(nil) })

	response := performCORSRequest("https://example.test")
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin=%q, want *", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("wildcard CORS must not allow credentials, got %q", got)
	}
}

func TestCORSExplicitOriginAllowsCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitCORS([]string{"https://allowed.test"})
	t.Cleanup(func() { InitCORS(nil) })

	response := performCORSRequest("https://allowed.test")
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.test" {
		t.Fatalf("allow origin=%q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("explicit origin should allow credentials, got %q", got)
	}
	if got := response.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary=%q, want Origin", got)
	}
}

func performCORSRequest(origin string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Origin", origin)
	CORS()(c)
	return recorder
}
