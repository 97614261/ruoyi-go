package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ruoyi-go/internal/model"
)

func (q *Qwen) submitCatalogResponses(ctx context.Context, r QwenRequest) QwenReceipt {
	c := r.Capabilities
	var conf struct {
		Instruction string `json:"instruction"`
	}
	if c == nil || c.MaxInputBytes < 1 || c.MaxInputBytes > 32000 || c.MaxOutputTokens < 1 || c.MaxOutputTokens > 2048 || ValidateCatalogCapabilities(q.adapterKind, q.origin, "text", r.Model, *c) != nil || len(r.Images) > 0 || len(r.Input.AssetIDs) > 0 || json.Unmarshal(r.Node.Config, &conf) != nil {
		return qwenFailure("invalidInput", "Responses 模型能力或输入无效，未调用模型")
	}
	instruction := strings.TrimSpace(conf.Instruction)
	if instruction == "" {
		instruction = "按用户要求编写适合 POD 印花创作的文字或提示词。直接返回结果。"
	}
	if strings.TrimSpace(r.Input.Text) == "" || len(instruction)+len(r.Input.Text) > c.MaxInputBytes {
		return qwenFailure("invalidInput", "文字输入为空或过长，未调用模型")
	}
	data, _ := json.Marshal(map[string]any{"model": r.Model, "input": r.Input.Text, "instructions": instruction, "max_output_tokens": c.MaxOutputTokens, "stream": false, "store": false})
	path := "/responses"
	if q.adapterKind != "openai-compatible-v1" {
		path = "/v1/responses"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.origin+path, bytes.NewReader(data))
	if err != nil || ctx.Err() != nil {
		return qwenFailure("notSubmitted", "调用前中断，未调用模型")
	}
	req.Header.Set("Authorization", "Bearer "+q.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return qwenUnknown("providerUncertain", "模型结果待确认，保留预占；不会自动重发")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return qwenUnknown("providerResponseIncomplete", "模型响应不完整，保留预占；不会自动重发")
	}
	if resp.StatusCode != 200 {
		switch resp.StatusCode {
		case 400, 401, 403, 404, 413, 422, 429:
			return qwenFailure("providerRejected", "供应商拒绝请求，请核对 Key、模型权限、额度与参数；未自动重试")
		}
		return qwenUnknown("providerUncertain", "供应商请求状态未确认，保留预占；不会自动重发")
	}
	var v struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			Input  *int64 `json:"input_tokens"`
			Output *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &v) != nil || v.ID == "" || len(v.ID) > 128 || v.Usage.Input == nil || v.Usage.Output == nil || *v.Usage.Input < 0 || *v.Usage.Input > 100000 || *v.Usage.Output < 0 || *v.Usage.Output > 1000000 {
		return qwenUnknown("providerInvalidResponse", "Responses 用量未确认，保留预占；不会自动重发")
	}
	var text strings.Builder
	for _, o := range v.Output {
		if o.Type == "message" {
			for _, p := range o.Content {
				if p.Type == "output_text" {
					text.WriteString(p.Text)
				}
			}
		}
	}
	if text.Len() == 0 || text.Len() > 128000 {
		return qwenUnknown("providerInvalidResponse", "Responses 正文未确认，保留预占；不会自动重发")
	}
	rc := QwenReceipt{State: "succeeded", Text: text.String(), Usage: model.PODProviderUsage{Model: r.Model, RequestID: v.ID, InputTokens: *v.Usage.Input, OutputTokens: *v.Usage.Output}}
	if v.Status != "completed" {
		rc.State = "failed"
		rc.Code = "partialOutput"
		rc.Message = "模型返回部分文字，已记录用量；未自动重试"
	}
	return rc
}
