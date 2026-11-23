package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func TestVolcengineCatalogChat(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		if req.URL.String() != VolcengineOrigin+"/api/v3/chat/completions" || req.Header.Get("Authorization") != "Bearer fixture-api-key" || json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Fatal("wrong Ark Chat request")
		}
		if body["model"] != volcengineMiniModel || !reflect.DeepEqual(body["thinking"], map[string]any{"type": "disabled"}) || body["service_tier"] != "default" || body["stream"] != false || body["max_completion_tokens"] != float64(64) || body["max_tokens"] != nil || body["enable_thinking"] != nil {
			t.Fatal("wrong Ark Chat model, tier or bounds", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"ark-chat","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":20,"completion_tokens":7}}`))}, nil
	})}
	provider, err := NewCatalogProvider("openai-chat-v1", VolcengineOrigin, "fixture-api-key", client)
	if err != nil || !ValidateCatalogOrigin("openai-chat-v1", "cn-beijing", VolcengineOrigin) || ValidateCatalogOrigin("openai-chat-v1", "cn", VolcengineOrigin) {
		t.Fatal("invalid Ark origin binding", err)
	}
	r := QwenRequest{Model: volcengineMiniModel, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 16000, MaxOutputTokens: 64}}
	if got := provider.Submit(context.Background(), r); got.State != "succeeded" || got.Usage.InputTokens != 20 || got.Usage.OutputTokens != 7 {
		t.Fatal("wrong Ark Chat receipt", got)
	}
	r.Capabilities.MaxInputBytes++
	if got := provider.Submit(context.Background(), r); got.State != "failed" || calls != 1 {
		t.Fatal("tier limit bypassed", got)
	}
}

func TestVolcengineCatalogSeedreamSingleImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	inline := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	calls, billed := 0, "1"
	var expectedImage any
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		if req.URL.String() != VolcengineOrigin+"/api/v3/images/generations" || req.Header.Get("Content-Type") != "application/json" || json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Fatal("wrong Seedream endpoint")
		}
		if body["model"] != volcengineImageModel || body["size"] != "1776x2368" || body["response_format"] != "url" || body["layer_decomposition"] != false || !reflect.DeepEqual(body["image"], expectedImage) || body["n"] != nil || body["sequential_image_generation"] != nil {
			t.Fatal("wrong Seedream single-image request")
		}
		response := `{"model":"` + volcengineImageModel + `","data":[{"url":"https://images.example.com/art.jpg"}],"usage":{"generated_images":` + billed + `}}`
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": {"ark-image"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	provider, err := NewCatalogProvider("openai-chat-v1", VolcengineOrigin, "fixture-api-key", client)
	if err != nil {
		t.Fatal(err)
	}
	r := QwenRequest{Model: volcengineImageModel, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"ratio":"3:4","count":1}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, MaxReferenceImages: 3, Ratios: []string{"3:4"}}}
	for n, expected := range []any{nil, inline, []any{inline, inline}, []any{inline, inline, inline}} {
		expectedImage = expected
		if n > 0 {
			r.Images = append(r.Images, inline)
			r.Input.AssetIDs = append(r.Input.AssetIDs, "1")
		}
		if got := provider.Submit(context.Background(), r); got.State != "succeeded" || got.Usage.ImageCount != 1 || got.Usage.RequestID != "ark-image" || got.ImageSource != "catalog-v1" {
			t.Fatalf("%d refs: unexpected Seedream receipt: %+v", n, got)
		}
	}
	billed = "2"
	if got := provider.Submit(context.Background(), r); got.State != "awaitingReconciliation" || calls != 5 {
		t.Fatal("mismatched meter accepted or request replayed")
	}
	r.Node.Config = json.RawMessage(`{"ratio":"3:4","count":2}`)
	if got := provider.Submit(context.Background(), r); got.State != "failed" || calls != 5 {
		t.Fatal("unsupported multi-output request reached the provider")
	}
	r.Node.Config = json.RawMessage(`{"ratio":"3:4","count":1}`)
	r.Images[0] = "data:image/png;base64," + catalogTestImage(t)
	if got := provider.Submit(context.Background(), r); got.Code != "invalidImage" || calls != 5 {
		t.Fatal("too-small reference reached the provider")
	}
}
