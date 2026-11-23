package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

type qwenRoundTrip func(*http.Request) (*http.Response, error)

func (f qwenRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func qwenResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func textQwenRequest() QwenRequest {
	return QwenRequest{Model: QwenTextModel, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{"instruction":"Write a short print prompt"}`)}, Input: model.PODRunOutput{Text: "a small cat"}}
}

func TestQwenFreeQuotaFailureDoesNotLeakOrRetry(t *testing.T) {
	for _, sample := range []struct {
		status            int
		body, state, code string
	}{
		{403, `{"code":"AllocationQuota.FreeTierOnly","message":"private-prompt https://private.invalid/?token=secret"}`, "failed", "providerFreeQuotaExhausted"},
		{403, `{"code":"unknown","message":"private-prompt token=secret"}`, "failed", "providerAccessDenied"},
		{401, `{"error":{"message":"private-prompt token=secret"}}`, "failed", "providerCredentialInvalid"},
		{404, `{"message":"private-prompt token=secret"}`, "failed", "providerModelUnavailable"},
		{429, `{"message":"private-prompt token=secret"}`, "failed", "providerRateLimited"},
		{422, `{"message":"private-prompt token=secret"}`, "failed", "providerInputRejected"},
		{500, `{"code":"AllocationQuota.FreeTierOnly"}`, "awaitingReconciliation", "providerUncertain"},
	} {
		calls := 0
		q, err := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return qwenResponse(sample.status, sample.body), nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		got := q.Submit(context.Background(), textQwenRequest())
		if calls != 1 || got.State != sample.state || got.Code != sample.code || strings.Contains(got.Message, "private") || strings.Contains(got.Message, "secret") || got.Usage.EstimatedCostMicros != 0 {
			t.Fatalf("unsafe failure classification: %+v calls=%d", got, calls)
		}
	}
}

func TestQwenTextContractAndUsage(t *testing.T) {
	calls := 0
	provider, err := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/compatible-mode/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only-not-a-real-key" {
			t.Fatal("incorrect authenticated request")
		}
		var b struct {
			Model    string              `json:"model"`
			Max      int                 `json:"max_tokens"`
			Thinking bool                `json:"enable_thinking"`
			Messages []map[string]string `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&b) != nil || b.Model != QwenTextModel || b.Max != 2048 || b.Thinking || len(b.Messages) != 2 || b.Messages[1]["content"] != "a small cat" {
			t.Fatal("incorrect text mapping")
		}
		return qwenResponse(200, `{"id":"request-text-1","choices":[{"finish_reason":"stop","message":{"content":"cat print"}}],"usage":{"prompt_tokens":1000,"completion_tokens":1000}}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result := provider.Submit(context.Background(), textQwenRequest())
	if calls != 1 || result.State != "succeeded" || result.Text != "cat print" || result.Usage.EstimatedCostMicros != 1650 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestQwenImageContractAndPartialBilling(t *testing.T) {
	var body map[string]json.RawMessage
	provider, _ := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/services/aigc/multimodal-generation/generation" {
			t.Fatal("incorrect native image API")
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid body")
		}
		return qwenResponse(200, `{"request_id":"request-image-1","output":{"choices":[{"message":{"content":[{"image":"https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/generated.png?Expires=1"}]}}]},"usage":{"image_count":1}}`), nil
	})})
	r := QwenRequest{Model: QwenImageModel, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"count":2,"ratio":"3:4","prompt":"flat colors"}`)}, Input: model.PODRunOutput{Text: "cat", AssetIDs: []string{"10"}}, Images: []string{"data:image/png;base64,YQ=="}}
	result := provider.Submit(context.Background(), r)
	var params struct {
		N         int    `json:"n"`
		Size      string `json:"size"`
		Extend    bool   `json:"prompt_extend"`
		Watermark bool   `json:"watermark"`
	}
	_ = json.Unmarshal(body["parameters"], &params)
	if params.N != 2 || params.Size != "960*1280" || params.Extend || params.Watermark {
		t.Fatal("incorrect bounded image parameters")
	}
	if result.State != "failed" || result.Code != "partialImages" || result.Usage.ImageCount != 1 || result.Usage.EstimatedCostMicros != 100000 || len(result.ImageURLs) != 1 {
		t.Fatal("partial images must retain usage and results")
	}
	if !strings.Contains(string(body["input"]), "data:image/png;base64,YQ==") || !strings.Contains(string(body["input"]), "flat colors") {
		t.Fatal("missing private reference or prompt")
	}
}

func TestQwenImageRatiosCountsAndReferences(t *testing.T) {
	for _, ratio := range []struct{ name, size string }{{"1:1", "1024*1024"}, {"3:4", "960*1280"}, {"4:3", "1280*960"}, {"9:16", "720*1280"}, {"16:9", "1280*720"}} {
		for count := 1; count <= 4; count++ {
			for _, references := range []int{0, 1, 3} {
				t.Run(fmt.Sprintf("%s_count%d_refs%d", ratio.name, count, references), func(t *testing.T) {
					calls := 0
					q, err := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(r *http.Request) (*http.Response, error) {
						calls++
						var body struct {
							Model string `json:"model"`
							Input struct {
								Messages []struct {
									Role    string              `json:"role"`
									Content []map[string]string `json:"content"`
								} `json:"messages"`
							} `json:"input"`
							Parameters struct {
								N    int    `json:"n"`
								Size string `json:"size"`
							} `json:"parameters"`
						}
						if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != QwenImageModel || body.Parameters.N != count || body.Parameters.Size != ratio.size || len(body.Input.Messages) != 1 {
							t.Fatal("incorrect native image wire contract")
						}
						message := body.Input.Messages[0]
						if message.Role != "user" || len(message.Content) != references+1 || message.Content[references]["text"] != "blue flower\nflat colors" {
							t.Fatal("reference/text content lost or reordered")
						}
						for i := 0; i < references; i++ {
							if message.Content[i]["image"] != "data:image/png;base64,YQ==" {
								t.Fatal("private reference payload missing")
							}
						}
						images := make([]map[string]string, count)
						for i := range images {
							images[i] = map[string]string{"image": fmt.Sprintf("https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/result-%d.png", i)}
						}
						reply, _ := json.Marshal(map[string]any{"request_id": "image-matrix", "output": map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": images}}}}, "usage": map[string]int{"image_count": count}})
						return qwenResponse(200, string(reply)), nil
					})})
					if err != nil {
						t.Fatal(err)
					}
					config, _ := json.Marshal(map[string]any{"ratio": ratio.name, "count": count, "prompt": "flat colors"})
					r := QwenRequest{Model: QwenImageModel, Node: model.PODNode{Type: "imageGenerate", Config: config}, Input: model.PODRunOutput{Text: "blue flower"}}
					for i := 0; i < references; i++ {
						r.Input.AssetIDs = append(r.Input.AssetIDs, strconv.Itoa(i+1))
						r.Images = append(r.Images, "data:image/png;base64,YQ==")
					}
					result := q.Submit(context.Background(), r)
					if calls != 1 || result.State != "succeeded" || len(result.ImageURLs) != count || result.Usage.ImageCount != int64(count) || result.Usage.EstimatedCostMicros != int64(count)*100000 {
						t.Fatal("multi-image response or accounting lost")
					}
				})
			}
		}
	}
}

func TestQwenUnknownNeverRetriesOrLeaks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		network bool
		state   string
	}{
		{"transport", 0, "", true, "awaitingReconciliation"},
		{"timeout", 408, "secret-signed-url-and-key", false, "awaitingReconciliation"},
		{"upstream500", 500, "secret-signed-url-and-key", false, "awaitingReconciliation"},
		{"badJSON", 200, "secret-signed-url-and-key", false, "awaitingReconciliation"},
		{"missingUsage", 200, `{"id":"req","choices":[{"finish_reason":"stop","message":{"content":"a"}}]}`, false, "awaitingReconciliation"},
		{"rejected", 401, "secret-signed-url-and-key", false, "failed"},
		{"limited", 429, "secret-signed-url-and-key", false, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			q, _ := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.network {
					return nil, errors.New("secret-signed-url-and-key")
				}
				return qwenResponse(tc.status, tc.body), nil
			})})
			r := q.Submit(context.Background(), textQwenRequest())
			encoded, _ := json.Marshal(r)
			if calls != 1 || r.State != tc.state || strings.Contains(string(encoded), "secret-signed") || strings.Contains(string(encoded), "test-only-not-a-real-key") {
				t.Fatalf("unsafe result: %s", encoded)
			}
		})
	}
}

func TestQwenPartialTextRetainsBilledUsage(t *testing.T) {
	r := decodeQwenText([]byte(`{"id":"req","choices":[{"finish_reason":"length","message":{"content":"partial"}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`), QwenTextModel, false)
	if r.State != "failed" || r.Text != "partial" || r.Usage.EstimatedCostMicros == 0 {
		t.Fatal("truncated output must not claim complete or lose billing")
	}
}

func TestQwenInputRejectsBeforeHTTP(t *testing.T) {
	calls := 0
	q, _ := NewQwen("https://dashscope.aliyuncs.com", "test-only-not-a-real-key", &http.Client{Transport: qwenRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not call") })})
	for _, r := range []QwenRequest{
		{Model: "unpriced-model", Node: textQwenRequest().Node, Input: model.PODRunOutput{Text: "a"}},
		{Model: QwenTextModel, Node: textQwenRequest().Node, Input: model.PODRunOutput{Text: strings.Repeat("a", QwenMaxInputBytes+1)}},
		{Model: QwenTextModel, Node: textQwenRequest().Node, Input: model.PODRunOutput{Text: "a", AssetIDs: []string{"1"}}},
		{Model: QwenImageModel, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{"count":5}`)}, Input: model.PODRunOutput{Text: "a"}},
		{Model: QwenImageModel, Node: model.PODNode{Type: "imageGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{Text: strings.Repeat("a", 1201)}},
	} {
		if got := q.Submit(context.Background(), r); got.State != "failed" {
			t.Fatal("invalid input accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid input incurred calls")
	}
}

func TestQwenOriginAndDownloadBoundaries(t *testing.T) {
	for _, origin := range []string{"http://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com.attacker.test", "https://key@dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com:8443", "https://dashscope.aliyuncs.com/x"} {
		if ValidateQwenOrigin(origin) {
			t.Fatal("unsafe API origin", origin)
		}
	}
	for _, address := range []string{"http://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/a", "https://127.0.0.1/a", "https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com.attacker.test/a", "https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com:80/a"} {
		if ValidQwenImageURL(address) {
			t.Fatal("unsafe result URL", address)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.1.1.1", "169.254.169.254", "100.64.1.1", "::1", "::ffff:192.168.1.1", "2001:db8::1"} {
		if publicQwenAddress(netip.MustParseAddr(ip)) {
			t.Fatal("unsafe resolved address", ip)
		}
	}
	url := "https://dashscope-result-bj.oss-cn-beijing.aliyuncs.com/a.png?Expires=1"
	if !ValidQwenImageURL(url) || !publicQwenAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public result rejected")
	}
	client := &http.Client{Transport: qwenRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credentials forwarded to download")
		}
		resp := qwenResponse(200, "123456")
		resp.Header.Set("Content-Type", "image/png")
		return resp, nil
	})}
	if _, err := DownloadQwenImage(context.Background(), client, url, 5); err == nil {
		t.Fatal("oversized download accepted")
	}
}

func TestQwenAcceleratedImageDownload(t *testing.T) {
	address := "https://dashscope-a717.oss-accelerate.aliyuncs.com/result.png?Expires=1"
	calls := 0
	client := &http.Client{Transport: qwenRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "dashscope-a717.oss-accelerate.aliyuncs.com" || r.Header.Get("Authorization") != "" {
			t.Fatal("incorrect accelerated download request")
		}
		resp := qwenResponse(200, "image-bytes")
		resp.Header.Set("Content-Type", "image/png")
		return resp, nil
	})}
	data, err := DownloadQwenImage(context.Background(), client, address, 100)
	if err != nil || string(data) != "image-bytes" || calls != 1 {
		t.Fatal("authenticated provider accelerated result cannot be downloaded")
	}
	for _, unsafe := range []string{
		"http://dashscope-a717.oss-accelerate.aliyuncs.com/a",
		"https://dashscope-a717.oss-accelerate.aliyuncs.com.attacker.test/a",
		"https://dashscope-a717.oss-accelerate.aliyuncs.com:8443/a",
		"https://key@dashscope-a717.oss-accelerate.aliyuncs.com/a",
		"https://other-bucket.oss-accelerate.aliyuncs.com/a",
		"https://dashscope-other.oss-accelerate.aliyuncs.com/a",
	} {
		if _, err := DownloadQwenImage(context.Background(), client, unsafe, 100); err == nil {
			t.Fatal("unsafe accelerated URL accepted", unsafe)
		}
	}
	if calls != 1 {
		t.Fatal("unsafe accelerated URL reached the HTTP client")
	}
}
