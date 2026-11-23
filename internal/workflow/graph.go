// Package workflow validates POD graph contracts without database or network access.
package workflow

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"

	"ruoyi-go/internal/model"
)

const MaxNodes = 100
const MaxEdges = 300
const MaxConfigBytes = 64 << 10

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Definitions is a bounded, versioned catalog, not a list of available providers.
// Executable stays false until the corresponding executor passes acceptance.
func Definitions() []model.PODNodeDefinition {
	t := model.PODPort{Key: "text", ValueType: "text", Required: true}
	i := model.PODPort{Key: "images", ValueType: "images", Required: true}
	optionalImages := model.PODPort{Key: "images", ValueType: "images"}
	f := model.PODPort{Key: "files", ValueType: "files", Required: true}
	return []model.PODNodeDefinition{
		{Type: "textInput", Version: 1, Label: "文本输入", Description: "写下主题、风格与设计要求", Inputs: []model.PODPort{}, Outputs: []model.PODPort{t}},
		{Type: "imageInput", Version: 1, Label: "图片输入", Description: "使用参考图或已有印花素材", Inputs: []model.PODPort{}, Outputs: []model.PODPort{i}},
		{Type: "textGenerate", Version: 1, Label: "AI 文本", Description: "整理创意，完善提示词", Inputs: []model.PODPort{t, optionalImages}, Outputs: []model.PODPort{t}},
		{Type: "imageGenerate", Version: 1, Label: "AI 生图", Description: "从文字或参考图生成图案", Inputs: []model.PODPort{t, optionalImages}, Outputs: []model.PODPort{i}},
		{Type: "imageProcess", Version: 1, Label: "图片处理", Description: "调整尺寸、处理背景与细节", Inputs: []model.PODPort{i}, Outputs: []model.PODPort{i}},
		{Type: "productPreview", Version: 1, Label: "商品预览", Description: "检查图案在商品上的位置", Inputs: []model.PODPort{i}, Outputs: []model.PODPort{i}},
		{Type: "export", Version: 1, Label: "结果导出", Description: "分别保存印刷图和商品展示图", Inputs: []model.PODPort{i}, Outputs: []model.PODPort{f}},
	}
}

// Validate validates a saveable draft. Missing required edges are permitted in
// drafts; execution will additionally validate completeness and provider params.
func Validate(g model.PODGraph) error {
	return ValidateWithLimits(g, DefaultLimits(model.PODGraphLimits{}))
}

func DefaultLimits(l model.PODGraphLimits) model.PODGraphLimits {
	if l.MaxNodes == 0 {
		l.MaxNodes = MaxNodes
	}
	if l.MaxEdges == 0 {
		l.MaxEdges = MaxEdges
	}
	if l.MaxGraphBytes == 0 {
		l.MaxGraphBytes = 1 << 20
	}
	return l
}

func ValidateWithLimits(g model.PODGraph, limits model.PODGraphLimits) error {
	limits = DefaultLimits(limits)
	if g.SchemaVersion != model.PODGraphVersion {
		return fmt.Errorf("不支持的画布版本")
	}
	if g.Nodes == nil || g.Edges == nil {
		return fmt.Errorf("nodes 和 edges 必须是数组")
	}
	if len(g.Nodes) > limits.MaxNodes || len(g.Edges) > limits.MaxEdges {
		return fmt.Errorf("画布节点或连线超过上限")
	}
	encoded, err := json.Marshal(g)
	if err != nil || len(encoded) > limits.MaxGraphBytes {
		return fmt.Errorf("画布格式不合法或超过大小限制")
	}
	definitions := map[string]model.PODNodeDefinition{}
	for _, d := range Definitions() {
		definitions[d.Type] = d
	}
	nodes := map[string]model.PODNodeDefinition{}
	indegree := map[string]int{}
	for _, n := range g.Nodes {
		if !identifier.MatchString(n.ID) {
			return fmt.Errorf("节点 ID 不合法")
		}
		if _, found := nodes[n.ID]; found {
			return fmt.Errorf("节点 ID 重复")
		}
		d, ok := definitions[n.Type]
		if !ok || n.Version != d.Version {
			return fmt.Errorf("节点类型或版本不受支持")
		}
		if math.IsNaN(n.Position.X) || math.IsNaN(n.Position.Y) || math.IsInf(n.Position.X, 0) || math.IsInf(n.Position.Y, 0) || math.Abs(n.Position.X) > 1e6 || math.Abs(n.Position.Y) > 1e6 {
			return fmt.Errorf("节点位置不合法")
		}
		var config map[string]json.RawMessage
		if len(n.Config) > MaxConfigBytes || json.Unmarshal(n.Config, &config) != nil || config == nil {
			return fmt.Errorf("节点配置必须是有大小限制的 JSON 对象")
		}
		if err := validateNodeConfig(n.Type, config); err != nil {
			return fmt.Errorf("%s 的配置字段或取值不合法", d.Label)
		}
		nodes[n.ID] = d
		indegree[n.ID] = 0
	}
	edgeIDs, occupied := map[string]bool{}, map[string]bool{}
	next := map[string][]string{}
	for _, e := range g.Edges {
		if !identifier.MatchString(e.ID) || edgeIDs[e.ID] {
			return fmt.Errorf("连线 ID 不合法或重复")
		}
		edgeIDs[e.ID] = true
		src, sok := nodes[e.Source]
		dst, dok := nodes[e.Target]
		if !sok || !dok || e.Source == e.Target {
			return fmt.Errorf("连线引用缺失节点或自身")
		}
		sourceType, targetType := portType(src.Outputs, e.SourcePort), portType(dst.Inputs, e.TargetPort)
		if sourceType == "" || targetType == "" || sourceType != targetType {
			return fmt.Errorf("连线端口不存在或类型不兼容")
		}
		// A single port has one upstream. A multi-image value is explicit; no
		// implicit merging or Cartesian product is introduced by connecting edges.
		key := e.Target + ":" + e.TargetPort
		if occupied[key] {
			return fmt.Errorf("输入端口已连接上游")
		}
		occupied[key] = true
		next[e.Source] = append(next[e.Source], e.Target)
		indegree[e.Target]++
	}
	queue := []string{}
	for id, degree := range indegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	for head := 0; head < len(queue); head++ {
		for _, id := range next[queue[head]] {
			indegree[id]--
			if indegree[id] == 0 {
				queue = append(queue, id)
			}
		}
	}
	if len(queue) != len(nodes) {
		return fmt.Errorf("画布不能包含循环")
	}
	return nil
}

func portType(ports []model.PODPort, key string) string {
	for _, p := range ports {
		if p.Key == key {
			return p.ValueType
		}
	}
	return ""
}
