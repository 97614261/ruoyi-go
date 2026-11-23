package workflow

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"testing"
)

func TestDraftConfigurationAndLimits(t *testing.T) {
	for _, item := range []struct {
		kind, config string
		valid        bool
	}{
		{"textInput", `{"text":"hello","title":"brief"}`, true},
		{"imageGenerate", `{"ratio":"16:9","count":4}`, true},
		{"imageProcess", `{"operation":"resize","width":2048,"height":2048}`, true},
		{"productPreview", `{"product":"tshirt","color":"#ffffff","printX":0.5,"printScale":0.6}`, true},
		{"export", `{"format":"png","fileName":"印花-1","mode":"print"}`, true},
		{"imageInput", `{"assetIds":[]}`, true},
		{"imageInput", `{"assetIds":["1"]}`, true},
		{"imageInput", `{"assetIds":["1","1"]}`, false},
		{"imageInput", `{"assetIds":["01"]}`, false},
		{"imageInput", `{"assetIds":[1]}`, false},
		{"imageGenerate", `{"count":1.5}`, false},
		{"imageGenerate", `{"ratio":"unknown"}`, false},
		{"imageProcess", `{"width":90000}`, false},
		{"imageProcess", `{"operation":"crop","rotate":90,"cropX":0.1,"cropY":0.2,"cropWidth":0.5,"cropHeight":0.5}`, true},
		{"imageProcess", `{"method":"ai","operation":"upscale","scale":4}`, true},
		{"imageProcess", `{"rotate":45}`, false},
		{"imageProcess", `{"cropWidth":0}`, false},
		{"imageProcess", `{"tolerance":121}`, false},
		{"imageProcess", `{"method":"shell"}`, false},
		{"productPreview", `{"side":"back"}`, true},
		{"export", `{"printWidthMM":0}`, false},
		{"export", `{"fileName":"../../secret"}`, false},
		{"textInput", `{"apiKey":"test-value"}`, false},
		{"textInput", `{"text":null}`, false},
	} {
		t.Run(item.kind+item.config, func(t *testing.T) {
			g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{graphNode("node", item.kind)}, Edges: []model.PODEdge{}}
			g.Nodes[0].Config = json.RawMessage(item.config)
			if got := Validate(g) == nil; got != item.valid {
				t.Fatalf("valid=%v want %v", got, item.valid)
			}
		})
	}
	g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{graphNode("a", "textInput"), graphNode("b", "textInput")}, Edges: []model.PODEdge{}}
	if ValidateWithLimits(g, model.PODGraphLimits{MaxNodes: 1}) == nil {
		t.Fatal("node config limit ignored")
	}
	if ValidateWithLimits(g, model.PODGraphLimits{MaxGraphBytes: 10}) == nil {
		t.Fatal("graph byte limit ignored")
	}
}
