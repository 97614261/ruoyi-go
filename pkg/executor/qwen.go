package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ruoyi-go/internal/model"
)

const QwenTextModel = "qwen-flash"
const QwenImageModel = "qwen-image-2.1-turbo"
const QwenMaxOutputTokens = 2048
const QwenMaxInputBytes = 32000

var qwenWorkspaceHost = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}\.cn-beijing\.maas\.aliyuncs\.com$`)

func ValidateQwenOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && (u.Host == "dashscope.aliyuncs.com" || qwenWorkspaceHost.MatchString(u.Host))
}

type Qwen struct {
	origin      string
	key         string
	client      *http.Client
	chatPath    string
	adapterKind string
}

func NewQwen(origin, key string, client *http.Client) (*Qwen, error) {
	if !ValidateQwenOrigin(origin) || len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, "\r\n \t") {
		return nil, errors.New("千问域名或服务端密钥配置无效")
	}
	if client == nil {
		client = &http.Client{Timeout: 300 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("千问接口不允许跳转") }
	return &Qwen{origin: origin, key: key, client: &copyClient}, nil
}

// QwenReceipt is internal durable state. ImageURLs must never enter public DTOs.
type QwenReceipt struct {
	State       string                 `json:"state"`
	Text        string                 `json:"text,omitempty"`
	ImageURLs   []string               `json:"imageUrls,omitempty"`
	ImageSource string                 `json:"imageSource,omitempty"`
	Usage       model.PODProviderUsage `json:"usage"`
	Code        string                 `json:"code,omitempty"`
	Message     string                 `json:"message,omitempty"`
}

type QwenRequest struct {
	Capabilities *model.PODModelCapabilities
	Model        string
	Node         model.PODNode
	Input        model.PODRunOutput
	Images       []string
}

func qwenFailure(code, message string) QwenReceipt {
	return QwenReceipt{State: "failed", Code: code, Message: message}
}
func qwenUnknown(code, message string) QwenReceipt {
	return QwenReceipt{State: "awaitingReconciliation", Code: code, Message: message}
}

func QwenImageCount(node model.PODNode) int {
	var c struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(node.Config, &c)
	if c.Count == 0 {
		return 1
	}
	return c.Count
}

func QwenImageSize(node model.PODNode) string {
	var c struct {
		Ratio string `json:"ratio"`
	}
	_ = json.Unmarshal(node.Config, &c)
	return map[string]string{"": "1024*1024", "1:1": "1024*1024", "3:4": "960*1280", "4:3": "1280*960", "9:16": "720*1280", "16:9": "1280*720"}[c.Ratio]
}

// Reservations are deliberately above the configured text token bound.
func QwenReservation(node model.PODNode) int64 {
	if node.Type == "textGenerate" {
		return 25000
	}
	if node.Type == "imageGenerate" {
		return int64(QwenImageCount(node)) * 100000
	}
	return 0
}

func (q *Qwen) Submit(ctx context.Context, r QwenRequest) (receipt QwenReceipt) {
	if q.chatPath != "" {
		defer func() { receipt.Message = strings.ReplaceAll(receipt.Message, "千问", "模型供应商") }()
		if r.Node.Type == "imageGenerate" {
			return q.submitCatalogImage(ctx, r)
		}
		if r.Node.Type != "textGenerate" {
			return qwenFailure("unsupportedNode", "兼容接口当前仅支持文字生成，未调用模型")
		}
		if r.Capabilities == nil {
			return qwenFailure("invalidInput", "兼容接口必须使用已保存的模型能力版本，未调用模型")
		}
		if q.origin == VolcengineOrigin && ValidateCatalogCapabilities(q.adapterKind, q.origin, "text", r.Model, *r.Capabilities) != nil {
			return qwenFailure("unsupportedModel", "方舟文字模型或输入上限无效，未调用模型")
		}
		if r.Capabilities.TextAPI == "responses" {
			return q.submitCatalogResponses(ctx, r)
		}
	}
	var c struct {
		Instruction string `json:"instruction"`
		Prompt      string `json:"prompt"`
	}
	if json.Unmarshal(r.Node.Config, &c) != nil {
		return qwenFailure("invalidInput", "节点配置无效，未调用模型")
	}
	var payload any
	path := ""
	switch r.Node.Type {
	case "textGenerate":
		caps := r.Capabilities
		maxInput, maxOutput := QwenMaxInputBytes, QwenMaxOutputTokens
		validModel := r.Model == QwenTextModel
		if caps != nil {
			maxInput, maxOutput = caps.MaxInputBytes, caps.MaxOutputTokens
			validModel = QwenCatalogTextModel(r.Model)
			if q.chatPath != "" {
				validModel = ValidateCatalogModel(q.adapterKind, q.origin, "text", r.Model)
			}
			if maxInput < 1 || maxInput > QwenMaxInputBytes || maxOutput < 1 || maxOutput > QwenMaxOutputTokens {
				validModel = false
			}
		}
		if !validModel || len(r.Input.AssetIDs) > 0 || len(r.Images) > 0 {
			return qwenFailure("unsupportedInput", "当前文字模型仅支持文字输入，未调用模型")
		}
		instruction := strings.TrimSpace(c.Instruction)
		if instruction == "" {
			instruction = "按用户要求编写适合 POD 印花创作的文字或提示词。直接返回结果。"
		}
		if len(instruction)+len(r.Input.Text) > maxInput || strings.TrimSpace(r.Input.Text) == "" {
			return qwenFailure("invalidInput", "文字输入为空或过长，未调用模型")
		}
		payload = map[string]any{"model": r.Model, "messages": []map[string]string{{"role": "system", "content": instruction}, {"role": "user", "content": r.Input.Text}}, "max_tokens": maxOutput, "enable_thinking": false, "stream": false}
		path = "/compatible-mode/v1/chat/completions"
		if q.chatPath == "" && r.Model == "qwen3.8-omni-flash" {
			// Omni 3.8 uses reasoning_effort, unlike the older Qwen thinking switch.
			body := payload.(map[string]any)
			delete(body, "enable_thinking")
			body["reasoning_effort"] = "none"
			body["modalities"] = []string{"text"}
		}
		if q.chatPath != "" {
			path = q.chatPath
			body := payload.(map[string]any)
			delete(body, "enable_thinking")
			if caps.TextAPI == "chat-completion-tokens" {
				delete(body, "max_tokens")
				body["max_completion_tokens"] = maxOutput
			}
			if q.origin == "https://api.deepseek.com" || q.origin == "https://open.bigmodel.cn" || q.origin == VolcengineOrigin {
				body["thinking"] = map[string]string{"type": "disabled"}
			}
			if q.origin == VolcengineOrigin {
				delete(body, "max_tokens")
				body["max_completion_tokens"] = maxOutput
				body["service_tier"] = "default"
			}
			if q.origin == "https://api.siliconflow.cn" {
				body["enable_thinking"] = false
			}
			if q.origin == "https://api.moonshot.cn" {
				delete(body, "max_tokens")
				body["max_completion_tokens"] = maxOutput
				body["reasoning_effort"] = "low"
			}
		}
	case "imageGenerate":
		count := QwenImageCount(r.Node)
		prompt := strings.TrimSpace(r.Input.Text + "\n" + c.Prompt)
		maxImages, maxReferences, maxChars := 4, 3, 1200
		validRatio := true
		if r.Capabilities != nil {
			maxImages, maxReferences, maxChars = r.Capabilities.MaxImages, r.Capabilities.MaxReferenceImages, r.Capabilities.MaxPromptChars
			var conf struct {
				Ratio string `json:"ratio"`
			}
			_ = json.Unmarshal(r.Node.Config, &conf)
			if conf.Ratio == "" {
				conf.Ratio = "1:1"
			}
			validRatio = false
			for _, ratio := range r.Capabilities.Ratios {
				if ratio == conf.Ratio {
					validRatio = true
				}
			}
		}
		validModel := r.Model == QwenImageModel
		if r.Capabilities != nil {
			validModel = QwenCatalogImageModel(r.Model)
		}
		if q.chatPath != "" {
			validModel = false
		}
		if !validRatio || !validModel || count < 1 || count > maxImages || len(r.Images) > maxReferences || len(r.Images) != len(r.Input.AssetIDs) || QwenImageSize(r.Node) == "" || prompt == "" || utf8.RuneCountInString(prompt) > maxChars {
			return qwenFailure("invalidInput", "生图提示词须为 1—1200 字、参考图最多 3 张、输出 1—4 张；未调用模型")
		}
		content := []map[string]string{}
		for _, data := range r.Images {
			if !strings.HasPrefix(data, "data:image/") || !strings.Contains(data, ";base64,") {
				return qwenFailure("invalidImage", "参考图数据无效，未调用模型")
			}
			content = append(content, map[string]string{"image": data})
		}
		content = append(content, map[string]string{"text": prompt})
		payload = map[string]any{"model": r.Model, "input": map[string]any{"messages": []any{map[string]any{"role": "user", "content": content}}}, "parameters": map[string]any{"n": count, "size": QwenImageSize(r.Node), "prompt_extend": false, "watermark": false}}
		path = "/api/v1/services/aigc/multimodal-generation/generation"
	default:
		return qwenFailure("unsupportedNode", "该节点尚未接通千问执行")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return qwenFailure("invalidInput", "无法准备模型输入，未调用模型")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.origin+path, bytes.NewReader(data))
	if err != nil || ctx.Err() != nil {
		return qwenFailure("notSubmitted", "调用开始前已中断，未调用模型")
	}
	req.Header.Set("Authorization", "Bearer "+q.key)
	req.Header.Set("Content-Type", "application/json")
	// No retries: a transport error does not prove the provider rejected the request.
	resp, err := q.client.Do(req)
	if err != nil {
		return qwenUnknown("providerUncertain", "千问响应未确认，保留预算预占；不会自动重复调用，请核对供应商请求明细")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return qwenUnknown("providerResponseIncomplete", "千问响应不完整，需核对原调用；不会重新提交")
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 || resp.StatusCode == 413 || resp.StatusCode == 422 || resp.StatusCode == 429 {
			// Only map known codes to our own text; provider messages may contain private input.
			var failure struct {
				Code string `json:"code"`
			}
			if resp.StatusCode == http.StatusForbidden && json.Unmarshal(body, &failure) == nil && failure.Code == "AllocationQuota.FreeTierOnly" {
				return qwenFailure("providerFreeQuotaExhausted", "千问免费额度已用尽，供应商当前仅允许免费调用；请在百炼控制台核对是否开通付费。本站估算预算不会自动开通付费，未自动重试")
			}
			switch resp.StatusCode {
			case 401:
				return qwenFailure("providerCredentialInvalid", "模型供应商凭据无效，请管理员检查或轮换对应厂商的 API Key；未自动重试")
			case 403:
				return qwenFailure("providerAccessDenied", "模型供应商拒绝访问，请管理员检查模型权限、账号额度和地域；未自动重试")
			case 404:
				return qwenFailure("providerModelUnavailable", "供应商未找到该模型，请管理员核对官方模型 ID 和本账号开通范围；未自动切换模型")
			case 429:
				return qwenFailure("providerRateLimited", "供应商已限流，请稍后查看原任务，再决定是否恢复失败步骤；未自动重试")
			default:
				return qwenFailure("providerInputRejected", "供应商拒绝输入或参数，请修改提示词、图片或参数后重新报价；未自动重试")
			}
		}
		return qwenUnknown("providerUncertain", "千问服务返回异常，需核对原调用；不会自动重复提交")
	}
	if r.Node.Type == "textGenerate" {
		if r.Capabilities != nil {
			return decodeQwenTextBounded(body, r.Model, resp.Header.Get("x-dashscope-partialresponse") == "true", 1000000)
		}
		return decodeQwenText(body, r.Model, resp.Header.Get("x-dashscope-partialresponse") == "true")
	}
	if r.Capabilities != nil {
		return decodeQwenImagesBounded(body, r.Model, QwenImageCount(r.Node), 16)
	}
	return decodeQwenImages(body, r.Model, QwenImageCount(r.Node))
}

func decodeQwenText(body []byte, modelID string, partial bool) QwenReceipt {
	return decodeQwenTextBounded(body, modelID, partial, QwenMaxOutputTokens)
}
func decodeQwenTextBounded(body []byte, modelID string, partial bool, maxTokens int64) QwenReceipt {
	var v struct {
		ID      string `json:"id"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Input  *int64 `json:"prompt_tokens"`
			Output *int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &v) != nil || len(v.Choices) != 1 || v.ID == "" || len(v.ID) > 128 || v.Usage.Input == nil || v.Usage.Output == nil || *v.Usage.Input < 0 || *v.Usage.Input > 100000 || *v.Usage.Output < 0 || *v.Usage.Output > maxTokens || len(v.Choices[0].Message.Content) > 128000 || v.Choices[0].Message.Content == "" {
		return qwenUnknown("providerInvalidResponse", "无法确认千问文字响应和用量，请核对原调用")
	}
	u := model.PODProviderUsage{Model: modelID, RequestID: v.ID, InputTokens: *v.Usage.Input, OutputTokens: *v.Usage.Output}
	u.EstimatedCostMicros = (u.InputTokens*15 + u.OutputTokens*150 + 99) / 100
	r := QwenReceipt{State: "succeeded", Text: v.Choices[0].Message.Content, Usage: u}
	if partial || v.Choices[0].FinishReason != "stop" {
		r.State = "failed"
		r.Code = "partialOutput"
		r.Message = "千问返回了部分文字，已记录用量；请缩短要求后再创建运行"
	}
	return r
}

func decodeQwenImages(body []byte, modelID string, count int) QwenReceipt {
	return decodeQwenImagesBounded(body, modelID, count, count)
}
func decodeQwenImagesBounded(body []byte, modelID string, count, maxCount int) QwenReceipt {
	var v struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Output    struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
					} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			Count int64 `json:"image_count"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &v) != nil {
		return qwenUnknown("providerInvalidResponse", "无法确认千问图片响应，请核对原调用")
	}
	if v.Code != "" {
		return qwenUnknown("providerInvalidResponse", "千问未返回可确认的图片结果，请核对原调用明细")
	}
	urls := []string{}
	for _, choice := range v.Output.Choices {
		for _, item := range choice.Message.Content {
			if item.Image != "" {
				urls = append(urls, item.Image)
			}
		}
	}
	if v.RequestID == "" || len(v.RequestID) > 128 || len(urls) < 1 || len(urls) > maxCount || v.Usage.Count != int64(len(urls)) {
		return qwenUnknown("providerInvalidResponse", "图片数量或用量未确认，请核对千问原调用")
	}
	u := model.PODProviderUsage{Model: modelID, RequestID: v.RequestID, ImageCount: v.Usage.Count, EstimatedCostMicros: v.Usage.Count * 100000}
	r := QwenReceipt{State: "succeeded", ImageURLs: urls, Usage: u}
	if len(urls) != count {
		r.State = "failed"
		r.Code = "partialImages"
		r.Message = "千问仅返回部分图片，已记录实际用量和图片；请检查后再决定是否生成新图"
	}
	return r
}
