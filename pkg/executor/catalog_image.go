package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"ruoyi-go/internal/model"
)

func CatalogImageModel(origin, id string) bool {
	switch origin {
	case VolcengineOrigin:
		return id == volcengineImageModel
	case "https://open.bigmodel.cn":
		return id == "glm-image" || id == "cogview-4" || id == "cogview-4-250304" || id == "cogview-3-flash"
	case "https://api.siliconflow.cn":
		switch id {
		case "Kwai-Kolors/Kolors", "Tongyi-MAI/Z-Image-Turbo", "Tongyi-MAI/Z-Image", "Qwen/Qwen-Image", "Qwen/Qwen-Image-Edit", "Qwen/Qwen-Image-Edit-2509", "OpenMOSS-Team/MOSS-Image":
			return true
		}
	case "https://api.openai.com":
		// GPT Image official token billing needs a different pricing rule.
		return id == "dall-e-3"
	}
	return false
}

// Protocol limits are checked in addition to the administrator's saved limits.
func ValidateCatalogCapabilities(adapter, origin, kind, id string, c model.PODModelCapabilities) error {
	if !ValidateCatalogModel(adapter, origin, kind, id) {
		return errors.New("模型 ID 与供应商协议不匹配")
	}
	if c.TextAPI != "" && adapter != "openai-compatible-v1" && origin != "https://api.openai.com" {
		return errors.New("该供应商不支持自定义文字协议")
	}
	if origin == VolcengineOrigin && kind == "text" && id == volcengineMiniModel && c.MaxInputBytes > volcengineMiniMaxText {
		return errors.New("方舟 Seed 2.0 Mini 固定单价仅支持不超过 16000 字节的文字输入")
	}
	if (len(c.ImageSizes) > 0 || c.ImageQuality != "" || c.ImageResponse != "") && adapter != "openai-compatible-v1" {
		return errors.New("仅自定义兼容渠道支持覆盖图片尺寸和质量")
	}
	if kind != "image" || adapter == "qwen-v1" {
		return nil
	}
	if err := ValidateCatalogImageOptions(c); err != nil {
		return err
	}
	maxCount, maxRefs := 1, 0
	if adapter == "openai-compatible-v1" {
		maxCount, maxRefs = 4, 3
	}
	if origin == VolcengineOrigin {
		maxRefs = 3
	}
	if origin == "https://api.siliconflow.cn" {
		if id == "Qwen/Qwen-Image-Edit" {
			maxRefs = 1
		}
		if id == "Qwen/Qwen-Image-Edit-2509" {
			maxRefs = 3
		}
		if maxRefs > 0 && (c.MaxReferenceImages < 1 || len(c.Ratios) != 1 || c.Ratios[0] != "1:1") {
			return errors.New("硅基图片编辑须允许参考图；输出沿用输入尺寸，比例仅保留默认值")
		}
	}
	if c.MaxImages > maxCount || c.MaxReferenceImages > maxRefs {
		return errors.New("该协议不支持所填生成张数或参考图数量")
	}
	for _, ratio := range c.Ratios {
		if c.ImageSizes[ratio] == "" && catalogImageSize(origin, id, ratio) == "" {
			return errors.New("该型号不支持所填图片比例")
		}
	}
	return nil
}

func ValidateCatalogImageOptions(c model.PODModelCapabilities) error {
	if c.ImageResponse != "" && c.ImageResponse != "url" && c.ImageResponse != "b64_json" {
		return errors.New("图片结果格式无效")
	}
	if c.ImageQuality != "" && c.ImageQuality != "auto" && c.ImageQuality != "low" && c.ImageQuality != "medium" && c.ImageQuality != "high" && c.ImageQuality != "standard" && c.ImageQuality != "hd" {
		return errors.New("图片质量参数无效")
	}
	for ratio, size := range c.ImageSizes {
		allowed := false
		for _, v := range c.Ratios {
			if v == ratio {
				allowed = true
			}
		}
		parts := strings.Split(size, "x")
		if !allowed || len(parts) != 2 {
			return errors.New("图片尺寸须对应已选比例，格式为 宽x高")
		}
		w, e1 := strconv.Atoi(parts[0])
		h, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || w < 256 || h < 256 || w > 4096 || h > 4096 || w*h > 4194304 || strconv.Itoa(w)+"x"+strconv.Itoa(h) != size {
			return errors.New("图片尺寸须在 256—4096 范围内，总像素不超过 4194304")
		}
	}
	return nil
}

func catalogImageSize(origin, id, ratio string) string {
	if ratio == "" {
		ratio = "1:1"
	}
	if origin == VolcengineOrigin {
		return map[string]string{"1:1": "2048x2048", "3:4": "1776x2368", "4:3": "2368x1776", "9:16": "1584x2816", "16:9": "2816x1584"}[ratio]
	}
	if origin == "https://open.bigmodel.cn" {
		if id == "glm-image" {
			return map[string]string{"1:1": "1280x1280", "3:4": "1088x1472", "4:3": "1472x1088", "9:16": "960x1728", "16:9": "1728x960"}[ratio]
		}
		return map[string]string{"1:1": "1024x1024", "3:4": "864x1152", "4:3": "1152x864", "9:16": "768x1344", "16:9": "1344x768"}[ratio]
	}
	if origin == "https://api.siliconflow.cn" {
		if strings.HasPrefix(id, "Qwen/Qwen-Image") {
			return map[string]string{"1:1": "1328x1328", "3:4": "1140x1472", "4:3": "1472x1140", "9:16": "928x1664", "16:9": "1664x928"}[ratio]
		}
		return map[string]string{"1:1": "1024x1024", "3:4": "960x1280", "4:3": "1280x960", "9:16": "720x1280", "16:9": "1280x720"}[ratio]
	}
	// Standard Images API sizes. Other aspect ratios must not silently become square.
	return map[string]string{"1:1": "1024x1024", "9:16": "1024x1792", "16:9": "1792x1024"}[ratio]
}

func (q *Qwen) submitCatalogImage(ctx context.Context, r QwenRequest) QwenReceipt {
	c := r.Capabilities
	if c == nil || ValidateCatalogCapabilities(q.adapterKind, q.origin, "image", r.Model, *c) != nil {
		return qwenFailure("unsupportedModel", "图片模型协议或能力版本无效，未调用模型")
	}
	var conf struct{ Prompt, Ratio string }
	if json.Unmarshal(r.Node.Config, &conf) != nil {
		return qwenFailure("invalidInput", "图片配置无效，未调用模型")
	}
	prompt := strings.TrimSpace(r.Input.Text + "\n" + conf.Prompt)
	count := QwenImageCount(r.Node)
	ratio := conf.Ratio
	if ratio == "" {
		ratio = "1:1"
	}
	allowed := false
	for _, v := range c.Ratios {
		if ratio == v {
			allowed = true
		}
	}
	edit := q.origin == "https://api.siliconflow.cn" && strings.HasPrefix(r.Model, "Qwen/Qwen-Image-Edit")
	if !allowed || prompt == "" || utf8.RuneCountInString(prompt) > c.MaxPromptChars || count < 1 || count > c.MaxImages || count > 4 || len(r.Images) != len(r.Input.AssetIDs) || len(r.Images) > c.MaxReferenceImages || (edit && len(r.Images) == 0) {
		return qwenFailure("invalidInput", "提示词、比例、张数或参考图不符合所选模型的能力，未调用模型")
	}
	for _, data := range r.Images {
		imageBytes, err := decodeCatalogInline(data, catalogInlineImageBytes)
		if err != nil {
			return qwenFailure("invalidImage", "参考图须为有效图片且每张不超过 8 MiB，未调用模型")
		}
		if q.origin == VolcengineOrigin {
			dim, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
			w, h := int64(dim.Width), int64(dim.Height)
			if err != nil || w <= 14 || h <= 14 || w > 36_000_000/h || w > 16*h || h > 16*w {
				return qwenFailure("invalidImage", "方舟参考图长宽均须大于 14 像素、比例在 1:16 至 16:1、总像素不超过 3600 万；未调用模型")
			}
		}
	}
	payload := map[string]any{"model": r.Model, "prompt": prompt, "size": catalogImageSize(q.origin, r.Model, ratio)}
	if customSize := c.ImageSizes[ratio]; customSize != "" {
		payload["size"] = customSize
	}
	if c.ImageQuality != "" {
		payload["quality"] = c.ImageQuality
	}
	format := "url"
	if q.adapterKind == "openai-compatible-v1" {
		format = c.ImageResponse
	}
	if format != "" && (q.adapterKind == "openai-compatible-v1" || q.origin == "https://api.openai.com") {
		payload["response_format"] = format
	}
	path := "/v1/images/generations"
	switch q.origin {
	case VolcengineOrigin:
		path = "/api/v3/images/generations"
		payload["response_format"] = "url"
		payload["output_format"] = "jpeg"
		payload["layer_decomposition"] = false
		payload["watermark"] = false
		if len(r.Images) == 1 {
			payload["image"] = r.Images[0]
		} else if len(r.Images) > 1 {
			payload["image"] = r.Images
		}
	case "https://open.bigmodel.cn":
		path = "/api/paas/v4/images/generations"
	case "https://api.siliconflow.cn":
		delete(payload, "size")
		if !edit {
			payload["image_size"] = catalogImageSize(q.origin, r.Model, ratio)
		}
		for i, data := range r.Images {
			key := "image"
			if i > 0 {
				key += strconv.Itoa(i + 1)
			}
			payload[key] = data
		}
	default:
		payload["n"] = count
		if q.adapterKind == "openai-compatible-v1" {
			path = "/images/generations"
		}
		if r.Model == "dall-e-3" {
			payload["quality"] = "standard"
		}
	}
	var data []byte
	contentType := "application/json"
	var err error
	if len(r.Images) > 0 && q.adapterKind == "openai-compatible-v1" {
		path = "/images/edits"
		data, contentType, err = catalogEditBody(payload, r.Images)
	} else {
		data, err = json.Marshal(payload)
	}
	if err != nil {
		return qwenFailure("invalidInput", "无法准备图片请求，未调用模型")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.origin+path, bytes.NewReader(data))
	if err != nil || ctx.Err() != nil {
		return qwenFailure("notSubmitted", "调用前中断，未调用模型")
	}
	trace := uuid.NewString()
	req.Header.Set("Authorization", "Bearer "+q.key)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Trace-Id", trace)
	resp, err := q.client.Do(req)
	if err != nil {
		return qwenUnknown("providerUncertain", "图片调用结果待确认，保留预占；不会自动重发")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (48<<20)+1))
	if err != nil || len(body) > 48<<20 {
		return qwenUnknown("providerResponseIncomplete", "图片响应不完整或过大；保留预占，不会自动重发")
	}
	if resp.StatusCode != 200 {
		switch resp.StatusCode {
		case 400, 401, 403, 404, 413, 422, 429:
			return qwenFailure("providerRejected", "供应商拒绝图片请求，请核对 Key、模型权限、额度与参数；未自动重试")
		default:
			return qwenUnknown("providerUncertain", "供应商图片调用状态未确认，保留预占；不会自动重发")
		}
	}
	for _, header := range []string{"x-request-id", "x-siliconcloud-trace-id", "request-id"} {
		if id := resp.Header.Get(header); id != "" && len(id) <= 128 && !strings.ContainsAny(id, "\r\n") {
			trace = id
			break
		}
	}
	return decodeCatalogImagesReceipt(body, r.Model, trace, count, format, q.origin == VolcengineOrigin)
}

func catalogEditBody(payload map[string]any, images []string) ([]byte, string, error) {
	var out bytes.Buffer
	w := multipart.NewWriter(&out)
	for _, key := range []string{"model", "prompt", "size", "n", "quality", "response_format"} {
		if value, ok := payload[key]; ok {
			v := ""
			switch n := value.(type) {
			case string:
				v = n
			case int:
				v = strconv.Itoa(n)
			}
			if err := w.WriteField(key, v); err != nil {
				return nil, "", err
			}
		}
	}
	for i, raw := range images {
		data, err := decodeCatalogInline(raw, catalogInlineImageBytes)
		if err != nil {
			return nil, "", err
		}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[http.DetectContentType(data)]
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="image[]"; filename="reference-`+strconv.Itoa(i+1)+`.`+ext+`"`)
		header.Set("Content-Type", http.DetectContentType(data))
		part, err := w.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), w.FormDataContentType(), nil
}

func decodeCatalogImages(body []byte, modelID, trace string, count int) QwenReceipt {
	return decodeCatalogImagesFormat(body, modelID, trace, count, "")
}

func decodeCatalogImagesFormat(body []byte, modelID, trace string, count int, format string) QwenReceipt {
	return decodeCatalogImagesReceipt(body, modelID, trace, count, format, false)
}

func decodeCatalogImagesReceipt(body []byte, modelID, trace string, count int, format string, ark bool) QwenReceipt {
	type item struct {
		URL    string `json:"url"`
		Base64 string `json:"b64_json"`
	}
	var v struct {
		ID        string          `json:"id"`
		RequestID string          `json:"request_id"`
		Data      []item          `json:"data"`
		Images    []item          `json:"images"`
		Model     string          `json:"model"`
		Usage     json.RawMessage `json:"usage"`
	}
	invalid := func() QwenReceipt {
		return qwenUnknown("providerInvalidResponse", "图片结果或数量无法确认，保留预占；不会自动重发")
	}
	if json.Unmarshal(body, &v) != nil || (len(v.Data) > 0 && len(v.Images) > 0) {
		return invalid()
	}
	if ark {
		var usage struct {
			Generated *int64 `json:"generated_images"`
		}
		if json.Unmarshal(v.Usage, &usage) != nil || v.Model != modelID || usage.Generated == nil || *usage.Generated != int64(len(v.Data)) || len(v.Images) != 0 {
			return invalid()
		}
	}
	items := v.Data
	if len(items) == 0 {
		items = v.Images
	}
	if len(items) < 1 || len(items) > 16 {
		return invalid()
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if (format == "url" && item.Base64 != "") || (format == "b64_json" && item.URL != "") {
			return invalid()
		}
		if item.URL != "" && item.Base64 != "" {
			return invalid()
		}
		if item.Base64 != "" {
			if len(item.Base64) > base64.StdEncoding.EncodedLen(catalogInlineImageBytes) {
				return invalid()
			}
			data, err := base64.StdEncoding.Strict().DecodeString(item.Base64)
			media := http.DetectContentType(data)
			if err != nil || (media != "image/png" && media != "image/jpeg" && media != "image/webp") {
				return invalid()
			}
			item.URL = "data:" + media + ";base64," + item.Base64
		} else if !ValidCatalogImageURL(item.URL) {
			return invalid()
		}
		urls = append(urls, item.URL)
	}
	if v.RequestID != "" {
		trace = v.RequestID
	} else if v.ID != "" {
		trace = v.ID
	}
	if len(trace) == 0 || len(trace) > 128 || strings.ContainsAny(trace, "\r\n") {
		return invalid()
	}
	r := QwenReceipt{State: "succeeded", ImageURLs: urls, ImageSource: "catalog-v1", Usage: model.PODProviderUsage{Model: modelID, RequestID: trace, ImageCount: int64(len(urls))}}
	if len(urls) != count {
		r.State = "failed"
		r.Code = "partialImages"
		r.Message = "返回张数与请求不一致，已记录实际结果和用量；未自动补图"
	}
	return r
}
