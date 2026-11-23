package workflow

import (
	"encoding/json"
	"testing"

	"ruoyi-go/internal/model"
)

func TestPolishSnapshotDoesNotMutateCanvas(t *testing.T) {
	g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "polish-source", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"blue flowers"}`)}}, Edges: []model.PODEdge{}}
	before, _ := json.Marshal(g)
	action, err := ActionGraph(g, "polish", "polish-source")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ExecutionPlanMode(action, model.PODGraphLimits{}, "", nil, "qwen")
	if err != nil || len(plan) != 2 || plan[1].Type != "textGenerate" || plan[1].ID != "polish-source" {
		t.Fatalf("bad polish execution: %v", err)
	}
	after, _ := json.Marshal(g)
	if string(before) != string(after) {
		t.Fatal("polish mutated the saved source")
	}
	if _, err := ActionGraph(g, "polish", "missing"); err == nil {
		t.Fatal("missing target accepted")
	}
	g.Nodes[0].Config = json.RawMessage(`{"text":"  "}`)
	if _, err := ActionGraph(g, "polish", "polish-source"); err == nil {
		t.Fatal("empty text accepted")
	}
	g.Nodes[0].Type = "imageInput"
	if _, err := ActionGraph(g, "polish", "polish-source"); err == nil {
		t.Fatal("non-text source accepted")
	}
}

func TestBranchRunsOnlyAncestorsAndTarget(t *testing.T) {
	node := func(id, kind, config string) model.PODNode {
		return model.PODNode{ID: id, Type: kind, Version: 1, Config: json.RawMessage(config)}
	}
	edge := func(id, source, target, port string) model.PODEdge {
		return model.PODEdge{ID: id, Source: source, Target: target, SourcePort: port, TargetPort: port}
	}
	g := model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{node("A", "textInput", `{"text":"flowers"}`), node("B", "imageGenerate", `{"count":1,"ratio":"1:1"}`), node("C", "imageGenerate", `{"count":1,"ratio":"1:1"}`), node("D", "imageProcess", `{"operation":"resize"}`)}, Edges: []model.PODEdge{edge("ab", "A", "B", "text"), edge("ac", "A", "C", "text"), edge("bd", "B", "D", "images")}}
	action, err := ActionGraph(g, "branch", "B")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ExecutionPlanMode(action, model.PODGraphLimits{}, "", nil, "qwen")
	if err != nil || len(plan) != 2 || plan[0].ID != "A" || plan[1].ID != "B" || len(action.Edges) != 1 {
		t.Fatalf("unrelated nodes reached execution: %v", err)
	}
	if len(g.Nodes) != 4 || len(g.Edges) != 3 {
		t.Fatal("branch changed original graph")
	}
	if _, err := ActionGraph(g, "other", "B"); err == nil {
		t.Fatal("unknown action accepted")
	}
}
