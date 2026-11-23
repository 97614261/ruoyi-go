package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func catalogTestImage(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestCatalogEndpointAndDNSGuards(t *testing.T) {
	for _, s := range []string{"https://relay.example.com", "https://relay.example.com/v1", "https://relay.example.com/api/openai/v1"} {
		if !ValidCatalogEndpoint(s) {
			t.Fatal("valid base rejected", s)
		}
	}
	for _, s := range []string{"http://relay.example.com/v1", "https://127.0.0.1/v1", "https://[::1]/v1", "https://localhost/v1", "https://a.local/v1", "https://relay.example.com:8443/v1", "https://key@relay.example.com/v1", "https://relay.example.com/v1/", "https://relay.example.com/a/../v1", "https://relay.example.com/a/%2e%2e/v1", "https://relay.example.com/v1?key=secret", "https://relay.example.com/v1#x", "https://relay.example.com//v1"} {
		if ValidCatalogEndpoint(s) {
			t.Fatal("unsafe base accepted", s)
		}
	}
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.100.100.200", "192.168.1.1", "192.0.2.1", "240.0.0.1", "::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1"} {
		if catalogPublicIPs([]netip.Addr{netip.MustParseAddr(s)}) {
			t.Fatal("unsafe IP accepted", s)
		}
	}
	if catalogPublicIPs([]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}) {
		t.Fatal("mixed DNS answer accepted")
	}
	if !catalogPublicIPs([]netip.Addr{netip.MustParseAddr("8.8.8.8")}) {
		t.Fatal("public DNS rejected")
	}
	if ValidQwenImageURL("https://cdn.example.com/a.png") {
		t.Fatal("legacy Qwen policy widened")
	}
}

func TestCatalogOfficialImagesUseTheirOwnWire(t *testing.T) {
	for _, tt := range []struct{ origin, id, path, sizeKey, size string }{
		{"https://open.bigmodel.cn", "cogview-3-flash", "/api/paas/v4/images/generations", "size", "1024x1024"},
		{"https://open.bigmodel.cn", "glm-image", "/api/paas/v4/images/generations", "size", "1280x1280"},
		{"https://api.siliconflow.cn", "Kwai-Kolors/Kolors", "/v1/images/generations", "image_size", "1024x1024"},
		{"https://api.openai.com", "dall-e-3", "/v1/images/generations", "size", "1024x1024"},
	} {
		t.Run(tt.id, func(t *testing.T) {
			calls := 0
			status := 200
			p, e := NewCatalogProvider("openai-chat-v1", tt.origin, "fixture-api-key", &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				var b map[string]any
				if req.URL.Path != tt.path || req.Header.Get("Authorization") != "Bearer fixture-api-key" || json.NewDecoder(req.Body).Decode(&b) != nil || b[tt.sizeKey] != tt.size {
					t.Fatal("incorrect provider wire")
				}
				if tt.origin != "https://api.openai.com" && (b["n"] != nil || b["batch_size"] != nil) {
					t.Fatal("unsupported batch parameter sent")
				}
				body := `{"data":[{"url":"https://cdn.example.com/a.png"}]}`
				if strings.Contains(tt.origin, "siliconflow") {
					body = `{"images":[{"url":"https://cdn.example.com/a.png"}]}`
				}
				if status != 200 {
					body = `{"error":{"message":"private prompt fixture"}}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			r := QwenRequest{Model: tt.id, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"count":1,"ratio":"1:1"}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, Ratios: []string{"1:1"}}}
			got := p.Submit(context.Background(), r)
			if got.State != "succeeded" || got.ImageSource != "catalog-v1" || got.Usage.ImageCount != 1 || got.Usage.RequestID == "" || calls != 1 {
				t.Fatal("receipt missing", got)
			}
			r.Node.Config = json.RawMessage(`{"count":2}`)
			if got = p.Submit(context.Background(), r); got.State != "failed" || calls != 1 {
				t.Fatal("count limit reached network")
			}
			r.Node.Config = json.RawMessage(`{"count":1}`)
			status = 503
			got = p.Submit(context.Background(), r)
			if got.State != "awaitingReconciliation" || calls != 2 || strings.Contains(got.Message, "private prompt") {
				t.Fatal("uncertain response retried or leaked")
			}
			status = 401
			got = p.Submit(context.Background(), r)
			if got.State != "failed" || calls != 3 {
				t.Fatal("credential rejection not terminal")
			}
		})
	}
}

func TestCatalogRelayImageEditsAndInlineArchive(t *testing.T) {
	b64 := catalogTestImage(t)
	inline := "data:image/png;base64," + b64
	calls := 0
	p, e := NewCatalogProvider("openai-compatible-v1", "https://relay.example.com/v1", "fixture-api-key", &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/images/edits" || req.ParseMultipartForm(1<<20) != nil || req.FormValue("model") != "vendor/image-v1" || req.FormValue("size") != "1024x1536" || req.FormValue("quality") != "low" {
			t.Fatal("incorrect edit protocol")
		}
		defer req.MultipartForm.RemoveAll()
		file, part, err := req.FormFile("image[]")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if part.Header.Get("Content-Type") != "image/png" {
			t.Fatal("reference MIME missing")
		}
		data, _ := io.ReadAll(file)
		if base64.StdEncoding.EncodeToString(data) != b64 {
			t.Fatal("reference bytes lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"` + b64 + `"}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	r := QwenRequest{Model: "vendor/image-v1", Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"ratio":"3:4"}`)}, Input: model.PODRunOutput{Text: "flower", AssetIDs: []string{"1"}}, Images: []string{inline}, Capabilities: &model.PODModelCapabilities{MaxPromptChars: 1200, MaxImages: 1, MaxReferenceImages: 1, Ratios: []string{"3:4"}, ImageSizes: map[string]string{"3:4": "1024x1536"}, ImageQuality: "low"}}
	got := p.Submit(context.Background(), r)
	if got.State != "succeeded" || calls != 1 {
		t.Fatal("relay edit failed", got.State)
	}
	data, e := DownloadCatalogImage(context.Background(), nil, got.ImageURLs[0], 1<<20)
	if e != nil || base64.StdEncoding.EncodeToString(data) != b64 {
		t.Fatal("inline result not recoverable")
	}
	r.Capabilities.ImageResponse = "url"
	if got = p.Submit(context.Background(), r); got.State != "awaitingReconciliation" || calls != 2 {
		t.Fatal("URL-only channel accepted unexpected Base64")
	}
	r.Capabilities.ImageResponse = "b64_json"
	if got = p.Submit(context.Background(), r); got.State != "succeeded" || calls != 3 {
		t.Fatal("explicit Base64 channel failed")
	}
	r.Images = append(r.Images, inline)
	r.Input.AssetIDs = append(r.Input.AssetIDs, "2")
	if got = p.Submit(context.Background(), r); got.State != "failed" || calls != 3 {
		t.Fatal("too many references submitted")
	}
	for _, body := range []string{`{"data":[{"url":"http://127.0.0.1/private"}]}`, `{"data":[{"b64_json":"bm90LWFuLWltYWdl"}]}`, `{"data":[]}`, `{"id":"async-task","status":"pending"}`} {
		if got := decodeCatalogImages([]byte(body), "image", "trace", 1); got.State != "awaitingReconciliation" {
			t.Fatal("invalid result confirmed", got.State)
		}
	}
	for _, address := range []string{"http://127.0.0.1/a.png", "https://169.254.169.254/a", "data:image/png;base64,bm90LWFuLWltYWdl"} {
		if _, e := DownloadCatalogImage(context.Background(), nil, address, 1024); e == nil {
			t.Fatal("unsafe archive accepted")
		}
	}
}

func TestCatalogKimiAndRelayResponses(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		var b map[string]any
		if json.NewDecoder(req.Body).Decode(&b) != nil {
			t.Fatal("invalid JSON")
		}
		body := `{"id":"fixture","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":20,"completion_tokens":7}}`
		if req.URL.Host == "api.moonshot.cn" {
			if b["max_completion_tokens"] != float64(64) || b["max_tokens"] != nil || b["reasoning_effort"] != "low" {
				t.Fatal("unbounded Kimi request")
			}
		} else {
			if req.URL.Path != "/v1/responses" || b["max_output_tokens"] != float64(64) || b["store"] != false {
				t.Fatal("incorrect Responses request")
			}
			body = `{"id":"response-fixture","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":20,"output_tokens":7}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for _, tt := range []struct{ adapter, origin, id, api string }{{"openai-chat-v1", "https://api.moonshot.cn", "kimi-k3.1-pro", ""}, {"openai-compatible-v1", "https://relay.example.com/v1", "arbitrary/text-model", "responses"}} {
		p, e := NewCatalogProvider(tt.adapter, tt.origin, "fixture-api-key", client)
		if e != nil {
			t.Fatal(e)
		}
		got := p.Submit(context.Background(), QwenRequest{Model: tt.id, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{Text: "flower"}, Capabilities: &model.PODModelCapabilities{MaxInputBytes: 512, MaxOutputTokens: 64, TextAPI: tt.api}})
		if got.State != "succeeded" || got.Text != "ok" || got.Usage.OutputTokens != 7 {
			t.Fatal("usage or output lost", got)
		}
	}
	if calls != 2 {
		t.Fatal("request replayed")
	}
}

func TestCatalogDownloaderNeverSendsCredentials(t *testing.T) {
	calls := 0
	b64 := catalogTestImage(t)
	data, _ := base64.StdEncoding.DecodeString(b64)
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Header.Get("Authorization") != "" || req.Method != "GET" {
			t.Fatal("credential forwarded to CDN")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	got, e := DownloadCatalogImage(context.Background(), client, "https://cdn.example.com/a.png?signature=fixture", 1024)
	if e != nil || !bytes.Equal(got, data) || calls != 1 {
		t.Fatal("download failed")
	}
	client.Transport = catalogRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("fixture transport failure")
	})
	if _, e = DownloadCatalogImage(context.Background(), client, "https://cdn.example.com/a.png", 1024); e == nil || calls != 2 {
		t.Fatal("failed archive retried")
	}
}

func TestBBRootPrivateImageDownload(t *testing.T) {
	const result = "https://bian.bbroot.com/images/2026/11/19/result.png"
	const key = "fixture-bbroot-api-key"
	data, _ := base64.StdEncoding.DecodeString(catalogTestImage(t))
	calls := 0
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodGet || req.URL.String() != result || req.Header.Get("Authorization") != "Bearer "+key || req.Header.Get("Cookie") != "" {
			t.Fatal("incorrect private result GET")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	got, err := DownloadBBRootPrivateImage(context.Background(), client, result, 1024, key)
	if err != nil || !bytes.Equal(got, data) || calls != 1 {
		t.Fatal("private image download failed", err)
	}
	for _, addr := range []string{result + "?signature=fixture", result + "?", "https://cdn.example.com/images/result.png", "https://cdn.bian.bbroot.com/images/result.png", "https://bian.bbroot.com.attacker.example/images/result.png", "https://bian.bbroot.com:443/images/result.png", "http://bian.bbroot.com/images/result.png", "https://bian.bbroot.com/images/../v1/models", "https://bian.bbroot.com/images/%2e%2e/v1/models", "https://bian.bbroot.com/v1/models"} {
		if _, err := DownloadBBRootPrivateImage(context.Background(), client, addr, 1024, key); err == nil || calls != 1 {
			t.Fatalf("private auth not blocked for %s", addr)
		}
	}
	if _, err := DownloadBBRootPrivateImage(context.Background(), client, result, 1024, "sess-fixture-session"); err == nil || calls != 1 {
		t.Fatal("browser session credential accepted")
	}
	for _, destination := range []string{"https://cdn.example.com/image.png", "/images/redirect.png"} {
		calls := 0
		client.Transport = catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.URL.String() != result {
				t.Fatal("followed authenticated redirect")
			}
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{destination}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		if _, err := DownloadBBRootPrivateImage(context.Background(), client, result, 1024, key); err == nil || calls != 1 {
			t.Fatal("redirect was not blocked")
		}
	}
}
