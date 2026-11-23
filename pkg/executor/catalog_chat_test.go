package executor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCatalogChatProviderWireAndUncertainty(t *testing.T) {
	for _, tt := range []struct {
		origin, model, path string
		thinking            bool
	}{
		{"https://api.deepseek.com", "deepseek-flash", "/chat/completions", true},
		{"https://open.bigmodel.cn", "glm-4.7", "/api/paas/v4/chat/completions", true},
		{"https://api.siliconflow.cn", "Qwen/Qwen3-8B", "/v1/chat/completions", false},
		{"https://api.openai.com", "gpt-4.1-mini", "/v1/chat/completions", false},
	} {
		t.Run(tt.model, func(t *testing.T) {
			calls := 0
			uncertain := false
			client := &http.Client{Transport: catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != tt.origin+tt.path || r.Header.Get("Authorization") != "Bearer local-fake-key" {
					t.Fatal("origin, path or credential not bound")
				}
				var b map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&b) != nil {
					t.Fatal("invalid JSON")
				}
				if string(b["model"]) != `"`+tt.model+`"` || string(b["max_tokens"]) != "64" || string(b["stream"]) != "false" {
					t.Fatal("bounded request lost")
				}
				if tt.thinking && string(b["thinking"]) != `{"type":"disabled"}` {
					t.Fatal("unbounded thinking mode enabled")
				}
				if tt.origin == "https://api.openai.com" && len(b["enable_thinking"]) != 0 {
					t.Fatal("Qwen parameter leaked into OpenAI request")
				}
				if uncertain {
					return nil, errors.New("fixture timeout")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"local-receipt","choices":[{"finish_reason":"stop","message":{"content":"blue flower"}}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`))}, nil
			})}
			p, err := NewCatalogProvider("openai-chat-v1", tt.origin, "local-fake-key", client)
			if err != nil {
				t.Fatal(err)
			}
			req := QwenRequest{Model: tt.model, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64}}
			got := p.Submit(context.Background(), req)
			if got.State != "succeeded" || got.Text != "blue flower" || got.Usage.InputTokens != 10 || calls != 1 {
				t.Fatal("result or usage lost", got.State)
			}
			missing := req
			missing.Capabilities = nil
			missing.Model = QwenTextModel
			if got := p.Submit(context.Background(), missing); got.State != "failed" || calls != 1 {
				t.Fatal("compatible adapter accepted an unversioned legacy request")
			}
			wrongKind := req
			wrongKind.Node.Type = "imageGenerate"
			wrongKind.Model = QwenImageModel
			wrongKind.Capabilities = &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, Ratios: []string{"1:1"}}
			if got := p.Submit(context.Background(), wrongKind); got.State != "failed" || calls != 1 {
				t.Fatal("compatible text adapter submitted an image request")
			}
			uncertain = true
			got = p.Submit(context.Background(), req)
			if got.State != "awaitingReconciliation" || calls != 2 {
				t.Fatal("uncertain response retried or refunded")
			}
			req.Model = "qwen-flash"
			p.Submit(context.Background(), req)
			if calls != 2 {
				t.Fatal("cross-provider model submitted")
			}
		})
	}
}

func TestCatalogProviderRejectsUnsafeOriginAndWrongModality(t *testing.T) {
	for _, origin := range []string{"http://api.deepseek.com", "https://api.deepseek.com:443", "https://api.deepseek.com/v1", "https://api.deepseek.com.evil.test", "https://127.0.0.1", "https://key@api.deepseek.com", "https://api.deepseek.com?x=1"} {
		if _, err := NewCatalogProvider("openai-chat-v1", origin, "local-fake-key", nil); err == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
	if ValidateCatalogOrigin("openai-chat-v1", "cn-beijing", "https://api.deepseek.com") {
		t.Fatal("incorrect region accepted")
	}
	if ValidateCatalogModel("openai-chat-v1", "https://api.openai.com", "image", "qwen-image-2.1-turbo") {
		t.Fatal("image adapter mismatch accepted")
	}
	if ValidateCatalogModel("qwen-v1", "https://dashscope.aliyuncs.com", "text", "deepseek-flash") {
		t.Fatal("foreign model assigned to Qwen")
	}
}

func TestCatalogQwenImageVersionsKeepLegacyFixed(t *testing.T) {
	calls := 0
	p, err := NewQwen("https://dashscope.aliyuncs.com", "local-fake-key", &http.Client{Transport: catalogRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":"InvalidParameter"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"qwen-image-2.1-turbo", "qwen-image-2.1-pro", "qwen-image-2.0", "qwen-image-2.0-pro"} {
		req := QwenRequest{Model: id, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"count":1}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, MaxReferenceImages: 0, Ratios: []string{"1:1"}}}
		before := calls
		p.Submit(context.Background(), req)
		if calls != before+1 {
			t.Fatalf("catalog variant %s did not reach provider", id)
		}
		if id != QwenImageModel {
			req.Capabilities = nil
			p.Submit(context.Background(), req)
			if calls != before+1 {
				t.Fatal("legacy image model changed")
			}
		}
	}
}

func TestBailianCatalogTextVersionsUseBoundedTextProtocol(t *testing.T) {
	for _, id := range []string{"qwen3.8-flash", "qwen3.8-27b", "qwen3.8-omni-flash", "deepseek-v4.1-flash"} {
		t.Run(id, func(t *testing.T) {
			calls := 0
			uncertain := false
			p, err := NewQwen("https://dashscope.aliyuncs.com", "local-fake-key", &http.Client{Transport: catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				var b map[string]json.RawMessage
				if r.URL.Path != "/compatible-mode/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&b) != nil {
					t.Fatal("wrong Bailian text protocol")
				}
				if string(b["model"]) != `"`+id+`"` || string(b["max_tokens"]) != "64" || string(b["stream"]) != "false" {
					t.Fatal("model or output bound lost")
				}
				if id == "qwen3.8-omni-flash" {
					if string(b["reasoning_effort"]) != `"none"` || string(b["modalities"]) != `["text"]` || len(b["enable_thinking"]) != 0 {
						t.Fatal("Omni must disable reasoning and return only text")
					}
				} else if string(b["enable_thinking"]) != "false" {
					t.Fatal("thinking enabled")
				}
				if uncertain {
					return nil, errors.New("fixture timeout")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","choices":[{"finish_reason":"stop","message":{"content":"flower"}}],"usage":{"prompt_tokens":8,"completion_tokens":1}}`))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			req := QwenRequest{Model: id, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64}}
			if got := p.Submit(context.Background(), req); got.State != "succeeded" || got.Usage.OutputTokens != 1 || calls != 1 {
				t.Fatal("text result or metered usage lost", got.State)
			}
			req.Capabilities = nil
			if got := p.Submit(context.Background(), req); got.State != "failed" || calls != 1 {
				t.Fatal("catalog model accepted without a version snapshot")
			}
			req.Capabilities = &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64}
			uncertain = true
			if got := p.Submit(context.Background(), req); got.State != "awaitingReconciliation" || calls != 2 {
				t.Fatal("uncertain call retried or treated as a definite failure")
			}
		})
	}
	for _, id := range []string{"qwen3.8-unknown", "deepseek-flash", "deepseek-v4-pro", "qwen-mt-image-2.0"} {
		if ValidateCatalogModel("qwen-v1", "https://dashscope.aliyuncs.com", "text", id) {
			t.Fatalf("unsupported Bailian text model accepted: %s", id)
		}
	}
	if QwenCatalogImageModel("qwen-mt-image-2.0") {
		t.Fatal("image translation advertised as image generation")
	}
}
