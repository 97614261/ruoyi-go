package modelcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/executor"
)

// The canonical typed snapshot binds every execution and pricing field.
func SpecHash(provider int64, v model.PODCatalogModelVersion, r model.PODCatalogWrite) string {
	b, _ := json.Marshal(struct {
		Provider                                            int64
		Official, Adapter, AdapterVersion, Endpoint, Region string
		Capabilities                                        *model.PODModelCapabilities
		Cost, Points                                        *model.PODModelRate
		Timeout                                             int
	}{provider, v.OfficialModelID, v.AdapterKind, v.AdapterVersion, v.Endpoint, v.Region, r.Capabilities, r.CostRule, r.PointRule, v.TimeoutSeconds})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func versionSpec(v model.PODCatalogModelVersion) (model.PODCatalogWrite, error) {
	r := model.PODCatalogWrite{OfficialModelID: v.OfficialModelID, TimeoutSeconds: v.TimeoutSeconds}
	if json.Unmarshal(v.CapabilitiesJSON, &r.Capabilities) != nil || json.Unmarshal(v.CostRuleJSON, &r.CostRule) != nil || json.Unmarshal(v.PointRuleJSON, &r.PointRule) != nil {
		return r, errors.New("模型版本格式无效")
	}
	return r, nil
}

func ValidateVersion(p model.PODProvider, m model.PODCatalogModel, v model.PODCatalogModelVersion) error {
	if v.ModelID != m.ModelID || v.AdapterVersion != "1" || v.AdapterKind != p.AdapterKind || v.Endpoint != p.Endpoint || v.Region != p.Region {
		return errors.New("模型版本与供应商规格不一致")
	}
	r, err := versionSpec(v)
	if err != nil {
		return err
	}
	if err := ValidateProvider(p.AdapterKind, p.Region, p.Endpoint); err != nil {
		return err
	}
	if err := ValidateSpec(m.Kind, r); err != nil {
		return err
	}
	if !executor.ValidateCatalogModel(v.AdapterKind, v.Endpoint, m.Kind, v.OfficialModelID) {
		return errors.New("模型与供应商协议或官方地址不匹配")
	}
	if v.SpecHash != SpecHash(p.ProviderID, v, r) {
		return errors.New("模型版本摘要不一致")
	}
	if err := executor.ValidateCatalogCapabilities(v.AdapterKind, v.Endpoint, m.Kind, v.OfficialModelID, *r.Capabilities); err != nil {
		return err
	}
	return nil
}

func ValidateEndpointChange(p model.PODProvider, endpoint string) error {
	if p.AdapterKind != "openai-compatible-v1" {
		return errors.New("官方供应商地址不可修改")
	}
	if err := ValidateProvider(p.AdapterKind, p.Region, endpoint); err != nil {
		return err
	}
	previous, previousErr := url.Parse(p.Endpoint)
	next, nextErr := url.Parse(endpoint)
	if previousErr != nil || nextErr != nil || !executor.ValidCatalogEndpoint(p.Endpoint) || previous.Host != next.Host {
		return errors.New("仅可修改原域名下的接口路径；更换域名请新建供应商并配置凭据")
	}
	return nil
}

// Historical quotes and runs keep the original path. The current version must match the configured endpoint.
func ValidateHistoricalVersion(p model.PODProvider, m model.PODCatalogModel, v model.PODCatalogModelVersion) error {
	if v.Endpoint != p.Endpoint && (m.CurrentVersionID == nil || v.ModelVersionID != *m.CurrentVersionID) {
		if err := ValidateEndpointChange(p, v.Endpoint); err != nil {
			return err
		}
		p.Endpoint = v.Endpoint
	}
	return ValidateVersion(p, m, v)
}

func VersionWithEndpoint(p model.PODProvider, m model.PODCatalogModel, v model.PODCatalogModelVersion, endpoint string) (model.PODCatalogModelVersion, error) {
	if err := ValidateEndpointChange(p, endpoint); err != nil {
		return v, err
	}
	if err := ValidateVersion(p, m, v); err != nil {
		return v, err
	}
	r, err := versionSpec(v)
	if err != nil {
		return v, err
	}
	v.Endpoint = endpoint
	v.SpecHash = SpecHash(p.ProviderID, v, r)
	p.Endpoint = endpoint
	return v, ValidateVersion(p, m, v)
}

func ValidateProvider(adapter, region, endpoint string) error {
	if !executor.ValidateCatalogOrigin(adapter, region, endpoint) {
		return errors.New("供应商协议、地域或 HTTPS Base URL 不匹配；中转渠道须为公网域名，不带凭据、查询参数或末尾斜线")
	}
	return nil
}

func ValidateSpec(kind string, r model.PODCatalogWrite) error {
	if r.Capabilities == nil || r.CostRule == nil || r.PointRule == nil || r.TimeoutSeconds < 10 || r.TimeoutSeconds > 300 {
		return errors.New("请填写完整能力、成本、点数和 10—300 秒超时")
	}
	c := r.Capabilities
	if kind == "text" {
		if !executor.CatalogTextModel(r.OfficialModelID) {
			return errors.New("文字模型标识不在当前协议支持范围")
		}
		if c.MaxInputBytes < 1 || c.MaxInputBytes > 32000 || c.MaxOutputTokens < 1 || c.MaxOutputTokens > 2048 || c.MaxPromptChars != 0 || c.MaxImages != 0 || c.MaxReferenceImages != 0 || len(c.Ratios) != 0 || len(c.ImageSizes) != 0 || c.ImageQuality != "" || c.ImageResponse != "" {
			return errors.New("文字能力仅支持最多 32000 输入字节、2048 输出 Token")
		}
	} else if kind == "image" {
		if !executor.ValidCatalogModelID(r.OfficialModelID) || c.MaxPromptChars < 1 || c.MaxPromptChars > 1200 || c.MaxImages < 1 || c.MaxImages > 4 || c.MaxReferenceImages < 0 || c.MaxReferenceImages > 3 || c.MaxInputBytes != 0 || c.MaxOutputTokens != 0 || len(c.Ratios) < 1 || len(c.Ratios) > 5 || c.TextAPI != "" {
			return errors.New("图片能力须为有界提示词、1—4 张输出、0—3 张参考图及有效比例")
		}
		seen := map[string]bool{}
		for _, ratio := range c.Ratios {
			if seen[ratio] || (ratio != "1:1" && ratio != "3:4" && ratio != "4:3" && ratio != "9:16" && ratio != "16:9") {
				return errors.New("图片比例无效或重复")
			}
			seen[ratio] = true
		}
		if err := executor.ValidateCatalogImageOptions(*c); err != nil {
			return err
		}
	} else {
		return errors.New("模型类型必须为 text 或 image")
	}
	if kind == "text" && c.TextAPI != "" && c.TextAPI != "chat-completion-tokens" && c.TextAPI != "responses" {
		return errors.New("文字协议须为默认 Chat、max_completion_tokens 或 Responses")
	}
	for i, rule := range []*model.PODModelRate{r.CostRule, r.PointRule} {
		unit, currency := "micros", "CNY"
		if i == 1 {
			unit, currency = "points", ""
		}
		if rule.Version != 1 || rule.Unit != unit || rule.Currency != currency || rule.InputPerMillion < 0 || rule.InputPerMillion > 1_000_000_000_000 || rule.OutputPerMillion < 0 || rule.OutputPerMillion > 1_000_000_000_000 || rule.PerImage < 0 || rule.PerImage > 1_000_000_000 {
			return errors.New("计价规则仅支持 v1、非负整数及规定单位/上限")
		}
		if (kind == "text" && rule.PerImage != 0) || (kind == "image" && (rule.InputPerMillion != 0 || rule.OutputPerMillion != 0)) {
			return errors.New("计价方式与模型类型不一致")
		}
	}
	return nil
}
