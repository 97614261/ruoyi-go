package apitest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyLimitIsWiredBeforeLogin(t *testing.T) {
	limit := appConfig.Server.MaxRequestBodyMB << 20
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(strings.Repeat("x", int(limit+1))))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("全局请求体限制应在登录处理前返回 413，实际 HTTP=%d body=%s",
			rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("413 响应应为统一 JSON：%v", err)
	}
	if result["code"] != float64(http.StatusRequestEntityTooLarge) {
		t.Errorf("413 响应业务 code 不符：%v", result)
	}
}
