package workflow

import (
	"encoding/json"
	"fmt"
	"ruoyi-go/internal/model"
	"strings"
)

func ExecutionLimits(l model.PODExecutionLimits) model.PODExecutionLimits {
	if l.GlobalConcurrency == 0 {
		l.GlobalConcurrency = 2
	}
	if l.UserConcurrency == 0 {
		l.UserConcurrency = 1
	}
	if l.ProviderConcurrency == 0 {
		l.ProviderConcurrency = 2
	}
	if l.MaxPending == 0 {
		l.MaxPending = 100
	}
	if l.MaxUserPending == 0 {
		l.MaxUserPending = 10
	}
	if l.InitialCredits == 0 {
		l.InitialCredits = 1000
	}
	return l
}
func TestExecutable(kind string) bool {
	return kind == "textInput" || kind == "imageInput" || kind == "textGenerate" || kind == "imageProcess" || kind == "productPreview" || kind == "export"
}

func Executable(kind, mode string) bool {
	return TestExecutable(kind) || ((mode == "qwen" || mode == "managed") && kind == "imageGenerate")
}

// ExecutionPlan preserves graph order within each topological level.
func ExecutionPlan(g model.PODGraph, limits model.PODGraphLimits, target string, base map[string]model.PODRunOutput) ([]model.PODNode, error) {
	return ExecutionPlanMode(g, limits, target, base, "simulation")
}

func ExecutionPlanMode(g model.PODGraph, limits model.PODGraphLimits, target string, base map[string]model.PODRunOutput, mode string) ([]model.PODNode, error) {
	if err := ValidateWithLimits(g, limits); err != nil {
		return nil, err
	}
	if len(g.Nodes) == 0 {
		return nil, fmt.Errorf("请先添加节点")
	}
	nodes := map[string]model.PODNode{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	selected := map[string]bool{}
	if target != "" {
		if _, ok := nodes[target]; !ok {
			return nil, fmt.Errorf("执行节点不存在")
		}
		selected[target] = true
	} else {
		for id := range nodes {
			selected[id] = true
		}
	}
	definitions := map[string]model.PODNodeDefinition{}
	for _, d := range Definitions() {
		definitions[d.Type] = d
	}
	for _, n := range g.Nodes {
		if !selected[n.ID] {
			continue
		}
		if !Executable(n.Type, mode) {
			return nil, fmt.Errorf("%s 尚未接通当前模式的执行", definitions[n.Type].Label)
		}
		var config struct {
			Text     string   `json:"text"`
			AssetIDs []string `json:"assetIds"`
		}
		_ = json.Unmarshal(n.Config, &config)
		if n.Type == "textInput" && strings.TrimSpace(config.Text) == "" {
			return nil, fmt.Errorf("文本输入不能为空")
		}
		if n.Type == "imageInput" && len(config.AssetIDs) == 0 {
			return nil, fmt.Errorf("图片输入至少选择一张素材")
		}
		for _, p := range definitions[n.Type].Inputs {
			found := false
			for _, e := range g.Edges {
				if e.Target != n.ID || e.TargetPort != p.Key {
					continue
				}
				found = true
				if !selected[e.Source] {
					output, ok := base[e.Source]
					if !ok || output.TestOnly != (mode == "simulation") || (p.Key == "text" && output.Text == "") || (p.Key == "images" && len(output.AssetIDs) == 0) {
						return nil, fmt.Errorf("缺少同版本成功上游结果，请先整条执行")
					}
				}
			}
			if (mode == "qwen" || mode == "managed") && n.Type == "textGenerate" && p.Key == "images" && found {
				return nil, fmt.Errorf("千问 Flash 文字节点当前不支持参考图，请将图片连接至生图节点")
			}
			if p.Required && !found {
				return nil, fmt.Errorf("%s 缺少必填输入 %s", definitions[n.Type].Label, p.Key)
			}
		}
	}
	result := []model.PODNode{}
	done := map[string]bool{}
	for len(result) < len(selected) {
		progress := false
		for _, n := range g.Nodes {
			if !selected[n.ID] || done[n.ID] {
				continue
			}
			ready := true
			for _, e := range g.Edges {
				if e.Target == n.ID && selected[e.Source] && !done[e.Source] {
					ready = false
				}
			}
			if ready {
				result = append(result, n)
				done[n.ID] = true
				progress = true
			}
		}
		if !progress {
			return nil, fmt.Errorf("执行计划存在循环")
		}
	}
	return result, nil
}

func NodeInputs(g model.PODGraph, id string, outputs map[string]model.PODRunOutput) model.PODRunOutput {
	input := model.PODRunOutput{TestOnly: true}
	for _, e := range g.Edges {
		if e.Target == id {
			if e.TargetPort == "text" {
				input.Text = outputs[e.Source].Text
			}
			if e.TargetPort == "images" {
				input.AssetIDs = outputs[e.Source].AssetIDs
				input.Images = outputs[e.Source].Images
			}
		}
	}
	return input
}

func RunTerminal(s model.PODRunState) bool {
	return s == model.PODRunSucceeded || s == model.PODRunFailed || s == model.PODRunCancelled
}

// Advance changes only not-yet-started nodes. In-flight results must be confirmed.
func Advance(g model.PODGraph, rows []model.PODNodeRun, base map[string]model.PODRunOutput, cancelling bool) (map[string]string, model.PODRunState) {
	states := map[string]string{}
	for id := range base {
		states[id] = "succeeded"
	}
	for _, r := range rows {
		states[r.NodeID] = r.State
	}
	changes := map[string]string{}
	for pass := 0; pass < len(rows); pass++ {
		changed := false
		for _, r := range rows {
			s := states[r.NodeID]
			if s != "waiting" && s != "ready" {
				continue
			}
			next := "ready"
			if cancelling {
				next = "cancelled"
			} else {
				for _, e := range g.Edges {
					if e.Target == r.NodeID {
						u := states[e.Source]
						if u == "failed" || u == "blocked" || u == "cancelled" {
							next = "blocked"
							break
						}
						if u != "succeeded" {
							next = "waiting"
						}
					}
				}
			}
			if next != s {
				states[r.NodeID] = next
				changes[r.NodeID] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	active, failed, uncertain, cancelled := false, false, false, false
	for _, r := range rows {
		switch states[r.NodeID] {
		case "awaitingReconciliation":
			uncertain = true
		case "waiting", "ready", "running":
			active = true
		case "failed", "blocked":
			failed = true
		case "cancelled":
			cancelled = true
		}
	}
	if cancelling && (active || uncertain) {
		return changes, model.PODRunCancelRequested
	}
	if uncertain {
		return changes, model.PODRunAwaitingReconciliation
	}
	if active {
		return changes, model.PODRunRunning
	}
	if cancelling || cancelled {
		return changes, model.PODRunCancelled
	}
	if failed {
		return changes, model.PODRunFailed
	}
	return changes, model.PODRunSucceeded
}
