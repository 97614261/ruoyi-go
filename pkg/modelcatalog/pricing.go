package modelcatalog

import (
	"encoding/json"
	"errors"
	"math/big"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/executor"
	"slices"
)

// Price rounds once after adding both token terms. No money passes through float64.
func Price(r model.PODModelRate, input, output, images int64) (int64, error) {
	if input < 0 || output < 0 || images < 0 || r.InputPerMillion < 0 || r.OutputPerMillion < 0 || r.PerImage < 0 {
		return 0, errors.New("invalid usage or price")
	}
	n := new(big.Int).Mul(big.NewInt(input), big.NewInt(r.InputPerMillion))
	n.Add(n, new(big.Int).Mul(big.NewInt(output), big.NewInt(r.OutputPerMillion)))
	n.Add(n, big.NewInt(999999))
	n.Quo(n, big.NewInt(1000000))
	n.Add(n, new(big.Int).Mul(big.NewInt(images), big.NewInt(r.PerImage)))
	if !n.IsInt64() {
		return 0, errors.New("price overflow")
	}
	return n.Int64(), nil
}

func Bind(m model.PODCatalogModel, v model.PODCatalogModelVersion, n model.PODNode, item int) (model.PODBoundModel, error) {
	b := model.PODBoundModel{ItemNo: item, NodeID: n.ID, ModelID: m.ModelID, ModelVersionID: v.ModelVersionID, ProviderID: m.ProviderID, Name: m.Name, Kind: m.Kind, Version: v}
	if json.Unmarshal(v.CapabilitiesJSON, &b.Capabilities) != nil || json.Unmarshal(v.CostRuleJSON, &b.CostRule) != nil || json.Unmarshal(v.PointRuleJSON, &b.PointRule) != nil {
		return b, errors.New("invalid model version")
	}
	input, output, images := int64(0), int64(0), int64(0)
	if n.Type == "textGenerate" && m.Kind == "text" {
		// Input is capped in UTF-8 bytes, including the system message; reserve extra chat framing tokens.
		input = int64(b.Capabilities.MaxInputBytes) + 256
		output = int64(b.Capabilities.MaxOutputTokens)
		b.InputTokenBound = input
	} else if n.Type == "imageGenerate" && m.Kind == "image" {
		var c struct {
			Ratio string `json:"ratio"`
		}
		_ = json.Unmarshal(n.Config, &c)
		if c.Ratio == "" {
			c.Ratio = "1:1"
		}
		b.ImageCount = executor.QwenImageCount(n)
		if b.ImageCount > b.Capabilities.MaxImages || !slices.Contains(b.Capabilities.Ratios, c.Ratio) {
			return b, errors.New("图片数量或比例不符合所选模型，请修改后重新报价")
		}
		images = int64(b.ImageCount)
	} else {
		return b, errors.New("模型类型与节点不一致")
	}
	var err error
	b.Points, err = Price(b.PointRule, input, output, images)
	if err != nil {
		return b, err
	}
	b.CostMicros, err = Price(b.CostRule, input, output, images)
	return b, err
}

func UsagePrice(b model.PODBoundModel, u model.PODProviderUsage) (cost, points int64, exceeded bool, err error) {
	cost, err = Price(b.CostRule, u.InputTokens, u.OutputTokens, u.ImageCount)
	if err != nil {
		return
	}
	points, err = Price(b.PointRule, u.InputTokens, u.OutputTokens, u.ImageCount)
	exceeded = cost > b.CostMicros || points > b.Points
	if b.Kind == "text" {
		exceeded = exceeded || u.InputTokens > b.InputTokenBound || u.OutputTokens > int64(b.Capabilities.MaxOutputTokens) || u.ImageCount != 0
	} else {
		exceeded = exceeded || u.ImageCount > int64(b.ImageCount) || u.InputTokens != 0 || u.OutputTokens != 0
	}
	return
}
