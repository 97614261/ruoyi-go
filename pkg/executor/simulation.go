// Package executor contains provider adapters without business database access.
package executor

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"time"
)

type Request struct {
	Key      string
	Scenario string
	Attempt  int
	Node     model.PODNode
	Input    model.PODRunOutput
}
type Result struct {
	State   string
	Output  model.PODRunOutput
	Code    string
	Message string
}
type Provider interface {
	Submit(context.Context, Request) Result
	Query(context.Context, Request) Result
}
type Simulation struct{}

// Query-by-key is a property of this deterministic adapter, not a promise about paid providers.
func (Simulation) Submit(ctx context.Context, r Request) Result {
	delay := 20 * time.Millisecond
	if r.Scenario == "delay" {
		delay = 3 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Result{State: "awaitingReconciliation", Code: "callInterrupted", Message: "调用中断，需确认原请求结果"}
	case <-timer.C:
	}
	if r.Node.Type == "textGenerate" && r.Scenario == "uncertain" {
		return Result{State: "awaitingReconciliation", Code: "testUncertain", Message: "测试响应丢失，正在按原请求标识确认结果"}
	}
	return simulationResult(r)
}
func (Simulation) Query(ctx context.Context, r Request) Result {
	if ctx.Err() != nil {
		return Result{State: "awaitingReconciliation", Code: "queryInterrupted", Message: "查询中断，保留待确认状态"}
	}
	return simulationResult(r)
}
func simulationResult(r Request) Result {
	output := model.PODRunOutput{TestOnly: true}
	var c struct {
		Text        string   `json:"text"`
		Instruction string   `json:"instruction"`
		AssetIDs    []string `json:"assetIds"`
	}
	_ = json.Unmarshal(r.Node.Config, &c)
	switch r.Node.Type {
	case "textInput":
		output.Text = c.Text
	case "imageInput":
		output.AssetIDs = c.AssetIDs
	case "textGenerate":
		if r.Scenario == "fail" || (r.Scenario == "failOnce" && r.Attempt == 1) {
			return Result{State: "failed", Code: "testFailure", Message: "测试执行器模拟失败，可恢复已确认失败的步骤"}
		}
		text := strings.TrimSpace(c.Instruction)
		if text != "" {
			text += "\n"
		}
		text += r.Input.Text
		// Bound chained output, independently of the number of text nodes.
		runes := []rune(text)
		if len(runes) > 16000 {
			runes = runes[:16000]
		}
		output.Text = "[测试输出，非 AI 生成]\n" + string(runes)
	default:
		return Result{State: "failed", Code: "unsupportedNode", Message: "该节点尚未接通执行"}
	}
	return Result{State: "succeeded", Output: output}
}
