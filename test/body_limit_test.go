package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyLimitsRunAfterAdmissionChecks(t *testing.T) {
	if err := purgeRateLimit(); err != nil {
		t.Fatalf("清理限流测试状态失败：%v", err)
	}
	tracker := &trackingReader{}
	req := httptest.NewRequest(http.MethodPost, "/system/user", tracker)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	var unauthorized map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &unauthorized); err != nil {
		t.Fatalf("未认证响应应为统一 JSON：%v", err)
	}
	if unauthorized["code"] != float64(http.StatusUnauthorized) {
		t.Fatalf("受保护接口应先鉴权，实际响应=%v", unauthorized)
	}
	if tracker.reads != 0 {
		t.Fatalf("鉴权失败前不得读取请求体，实际读取 %d 次", tracker.reads)
	}

	tracker = &trackingReader{}
	req = httptest.NewRequest(http.MethodPost, "/not-found", tracker)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在路径应直接返回 404，实际 HTTP=%d body=%s", rec.Code, rec.Body.String())
	}
	if tracker.reads != 0 {
		t.Fatalf("404 路径不得解析 multipart，实际读取 %d 次", tracker.reads)
	}

	// 匿名登录仍受大小限制，但顺序在 IP 限流之后。
	limit := appConfig.Server.MaxRequestBodyMB << 20
	req = httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(strings.Repeat("x", int(limit+1))))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
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

type trackingReader struct {
	reads int
}

func (r *trackingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}
