package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"ruoyi-go/internal/model"
)

// ActionGraph derives an execution snapshot without editing the saved canvas.
func ActionGraph(g model.PODGraph, mode, target string) (model.PODGraph, error) {
	var selected *model.PODNode
	for i := range g.Nodes {
		if g.Nodes[i].ID == target {
			selected = &g.Nodes[i]
			break
		}
	}
	if selected == nil {
		return model.PODGraph{}, fmt.Errorf("节点不存在，请刷新画布")
	}
	if mode == "polish" {
		if selected.Type != "textInput" {
			return model.PODGraph{}, fmt.Errorf("AI 润色仅支持文本输入节点")
		}
		var c struct {
			Text          string `json:"text"`
			PolishModelID string `json:"polishModelId"`
		}
		if json.Unmarshal(selected.Config, &c) != nil || strings.TrimSpace(c.Text) == "" {
			return model.PODGraph{}, fmt.Errorf("请先填写需要润色的文字")
		}
		sourceID := "polish-source"
		if sourceID == target {
			sourceID = "polish-source-1"
		}
		input := *selected
		input.ID = sourceID
		input.Position = model.PODPosition{}
		config, _ := json.Marshal(map[string]string{"title": "AI 润色", "instruction": "将用户文字润色为可直接用于图片生成的提示词。保留用户的语言、主题和明确要求，补充必要的构图、风格与配色描述，不擅自更换主体，不虚构品牌或产品信息。只返回润色后的提示词，不添加解释，不超过 1200 字。"})
		if c.PolishModelID != "" {
			var fields map[string]string
			_ = json.Unmarshal(config, &fields)
			fields["modelId"] = c.PolishModelID
			config, _ = json.Marshal(fields)
		}
		output := model.PODNode{ID: target, Type: "textGenerate", Version: 1, Position: model.PODPosition{X: 400}, Config: config}
		return model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{input, output}, Edges: []model.PODEdge{{ID: "polish-text", Source: sourceID, SourcePort: "text", Target: target, TargetPort: "text"}}}, nil
	}
	if mode != "branch" {
		return model.PODGraph{}, fmt.Errorf("不支持的节点操作")
	}
	needed := map[string]bool{target: true}
	// A bounded worklist also handles shared ancestors without duplicating calls.
	queue := []string{target}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, e := range g.Edges {
			if e.Target == id && !needed[e.Source] {
				needed[e.Source] = true
				queue = append(queue, e.Source)
			}
		}
	}
	result := model.PODGraph{SchemaVersion: g.SchemaVersion, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}}
	for _, n := range g.Nodes {
		if needed[n.ID] {
			result.Nodes = append(result.Nodes, n)
		}
	}
	for _, e := range g.Edges {
		if needed[e.Source] && needed[e.Target] {
			result.Edges = append(result.Edges, e)
		}
	}
	return result, nil
}
