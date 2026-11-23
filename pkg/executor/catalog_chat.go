package executor

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	VolcengineOrigin      = "https://ark.cn-beijing.volces.com"
	volcengineImageModel  = "doubao-seedream-5-0-flash-260915"
	volcengineMiniModel   = "doubao-seed-2-0-mini-260428"
	volcengineLiteModel   = "doubao-seed-2-1-lite-260915"
	volcengineMiniMaxText = 16000 // Keep fixed-rate text requests below the 32K input tier, including framing.
)

// Official profiles stay fixed; custom compatible channels use a separate adapter.
var catalogChatOrigins = map[string]struct{ Region, Path string }{
	"https://api.deepseek.com":   {"cn", "/chat/completions"},
	"https://open.bigmodel.cn":   {"cn", "/api/paas/v4/chat/completions"},
	"https://api.siliconflow.cn": {"cn", "/v1/chat/completions"},
	"https://api.openai.com":     {"global", "/v1/chat/completions"},
	"https://api.moonshot.cn":    {"cn", "/v1/chat/completions"},
	VolcengineOrigin:             {"cn-beijing", "/api/v3/chat/completions"},
}

func QwenCatalogTextModel(id string) bool {
	switch id {
	case "qwen-flash", "qwen-plus", "qwen-turbo",
		"qwen3.8-flash", "qwen3.8-27b", "qwen3.8-omni-flash",
		"deepseek-v4.1-flash": // This exact model is also hosted by Bailian.
		return true
	}
	return false
}

func QwenCatalogImageModel(id string) bool {
	return id == QwenImageModel || id == "qwen-image-2.1-pro" || id == "qwen-image-2.0" || id == "qwen-image-2.0-pro"
}

func CatalogTextModel(id string) bool {
	return ValidCatalogModelID(id)
}

func ValidateCatalogOrigin(adapter, region, origin string) bool {
	if adapter == "openai-compatible-v1" {
		return (region == "cn" || region == "global") && ValidCatalogEndpoint(origin)
	}
	if adapter == "qwen-v1" {
		return region == "cn-beijing" && ValidateQwenOrigin(origin)
	}
	p, ok := catalogChatOrigins[origin]
	return adapter == "openai-chat-v1" && ok && region == p.Region
}

func ValidateCatalogModel(adapter, origin, kind, id string) bool {
	if adapter == "qwen-v1" {
		if kind == "text" {
			return QwenCatalogTextModel(id)
		}
		return kind == "image" && QwenCatalogImageModel(id)
	}
	if adapter == "openai-compatible-v1" {
		return ValidCatalogEndpoint(origin) && (kind == "text" || kind == "image") && ValidCatalogModelID(id)
	}
	if adapter != "openai-chat-v1" || !ValidCatalogModelID(id) {
		return false
	}
	if kind == "image" {
		return CatalogImageModel(origin, id)
	}
	if kind != "text" {
		return false
	}
	switch origin {
	case VolcengineOrigin:
		return id == volcengineMiniModel || id == volcengineLiteModel
	case "https://api.deepseek.com":
		return strings.HasPrefix(id, "deepseek-")
	case "https://open.bigmodel.cn":
		return strings.HasPrefix(id, "glm-")
	case "https://api.openai.com":
		return strings.HasPrefix(id, "gpt-")
	case "https://api.moonshot.cn":
		return strings.HasPrefix(id, "kimi-") || strings.HasPrefix(id, "moonshot-")
	case "https://api.siliconflow.cn":
		return strings.Contains(id, "/")
	}
	return false
}

// Uses the same durable intent/receipt path; no adapter retries or new billing path.
func NewCatalogProvider(adapter, origin, key string, client *http.Client) (*Qwen, error) {
	if adapter == "qwen-v1" {
		return NewQwen(origin, key, client)
	}
	p, ok := catalogChatOrigins[origin]
	compatible := adapter == "openai-compatible-v1" && ValidCatalogEndpoint(origin)
	if (!compatible && (adapter != "openai-chat-v1" || !ok)) || len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, "\r\n \t") {
		return nil, errors.New("供应商协议、官方地址或凭据格式无效")
	}
	if client == nil {
		client = NewCatalogHTTPClient(300 * time.Second)
	}
	if compatible {
		p.Path = "/chat/completions"
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("供应商接口不允许跳转") }
	return &Qwen{origin: origin, key: key, client: &c, chatPath: p.Path, adapterKind: adapter}, nil
}
