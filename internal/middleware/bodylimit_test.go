package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestBodyLimitBoundaryAndReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const limit = int64(64)
	payload := strings.Repeat("a", int(limit))
	var fingerprint, logged, handled string

	r := gin.New()
	r.Use(RequestBodyLimit(limit))
	r.POST("/body", func(c *gin.Context) {
		fingerprint, _ = requestFingerprint(c)
		logged = captureRequestBody(c)
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Errorf("handler 读取请求体失败：%v", err)
		}
		handled = string(raw)
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("边界大小请求应放行，HTTP=%d body=%s", rec.Code, rec.Body.String())
	}
	if logged != payload || handled != payload {
		t.Fatalf("缓存内容必须原样提供给日志和 handler，logged=%d handled=%d want=%d",
			len(logged), len(handled), len(payload))
	}
	if fingerprint != hashRequest("", []byte(payload)) {
		t.Errorf("防重复提交应使用缓存的完整请求体，fingerprint=%s", fingerprint)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(payload+"b"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	assertPayloadTooLarge(t, rec)
}

func TestRequestBodyLimitRejectsChunkedAndConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		limit    = int64(128)
		requests = 16
	)
	var handlerCalls atomic.Int32
	r := gin.New()
	r.Use(RequestBodyLimit(limit))
	r.POST("/body", func(c *gin.Context) {
		handlerCalls.Add(1)
		c.Status(http.StatusNoContent)
	})

	statuses := make([]int, requests)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader(strings.Repeat("x", int(limit+1))))
			req.Header.Set("Content-Type", "application/json")
			// Content-Length 缺失时也必须按实际读取字节数限制，不能只信请求头。
			req.ContentLength = -1
			req.TransferEncoding = []string{"chunked"}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			statuses[index] = rec.Code
		}(i)
	}
	wg.Wait()

	for i, status := range statuses {
		if status != http.StatusRequestEntityTooLarge {
			t.Errorf("第 %d 个并发超限请求 HTTP=%d，期望 413", i+1, status)
		}
	}
	if calls := handlerCalls.Load(); calls != 0 {
		t.Fatalf("超限请求不应进入 handler，实际调用 %d 次", calls)
	}
}

func TestRequestBodyLimitLeavesMultipartToUploadLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var raw bytes.Buffer
	writer := multipart.NewWriter(&raw)
	part, err := writer.CreateFormField("content")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, strings.Repeat("x", 256)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	var handled int
	r := gin.New()
	r.Use(RequestBodyLimit(32))
	r.POST("/upload", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Errorf("读取 multipart 失败：%v", err)
		}
		handled = len(body)
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(raw.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || handled != raw.Len() {
		t.Fatalf("multipart 应交给上传限制处理，HTTP=%d handled=%d want=%d",
			rec.Code, handled, raw.Len())
	}
}

func TestBodyConsumersNeverReadWithoutEntryLimit(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/body", nil)
	tracker := &readTrackingBody{}
	c.Request.Body = tracker
	c.Request.Header.Set("Content-Type", "application/json")

	if _, ok := requestFingerprint(c); ok {
		t.Fatal("没有入口缓存时不应计算请求指纹")
	}
	if got := captureRequestBody(c); got != uncachedLogValue {
		t.Fatalf("没有入口缓存时操作日志应使用固定省略提示，实际=%q", got)
	}
	if tracker.reads != 0 {
		t.Fatalf("没有入口限制时不得读取请求体，实际读取 %d 次", tracker.reads)
	}
}

type readTrackingBody struct {
	reads int
}

func (b *readTrackingBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*readTrackingBody) Close() error { return nil }

func assertPayloadTooLarge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限请求 HTTP=%d，期望 413，body=%s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("413 响应应保持统一 JSON 结构：%v，body=%s", err, rec.Body.String())
	}
	if result["code"] != float64(http.StatusRequestEntityTooLarge) {
		t.Errorf("413 响应业务 code 不符：%v", result)
	}
}
