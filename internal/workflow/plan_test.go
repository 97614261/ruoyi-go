package workflow

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"testing"
)

func executionGraph() model.PODGraph {
	return model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{{ID: "A", Type: "textInput", Version: 1, Config: json.RawMessage(`{"text":"test"}`)}, {ID: "B", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{}`)}, {ID: "C", Type: "textGenerate", Version: 1, Config: json.RawMessage(`{}`)}}, Edges: []model.PODEdge{{ID: "AB", Source: "A", SourcePort: "text", Target: "B", TargetPort: "text"}, {ID: "BC", Source: "B", SourcePort: "text", Target: "C", TargetPort: "text"}}}
}
func TestPODExecutionPlan(t *testing.T) {
	g := executionGraph()
	plan, err := ExecutionPlan(g, model.PODGraphLimits{}, "", nil)
	if err != nil || len(plan) != 3 || plan[0].ID != "A" || plan[2].ID != "C" {
		t.Fatal("invalid topological plan", err)
	}
	if _, err := ExecutionPlan(g, model.PODGraphLimits{}, "B", nil); err == nil {
		t.Fatal("missing upstream accepted")
	}
	plan, err = ExecutionPlan(g, model.PODGraphLimits{}, "B", map[string]model.PODRunOutput{"A": {TestOnly: true, Text: "seed"}})
	if err != nil || len(plan) != 1 || plan[0].ID != "B" {
		t.Fatal("single node reran ancestors", err)
	}
	g.Nodes[1].Type = "imageGenerate"
	g.Edges = g.Edges[:1]
	g.Nodes = g.Nodes[:2]
	if _, err := ExecutionPlan(g, model.PODGraphLimits{}, "", nil); err == nil {
		t.Fatal("unimplemented generation accepted")
	}
}
func TestPODAdvanceFailureAndCancel(t *testing.T) {
	g := executionGraph()
	rows := []model.PODNodeRun{{NodeID: "A", State: "succeeded"}, {NodeID: "B", State: "failed"}, {NodeID: "C", State: "waiting"}}
	changes, state := Advance(g, rows, nil, false)
	if changes["C"] != "blocked" || state != model.PODRunFailed {
		t.Fatal(changes, state)
	}
	rows[1].State = "running"
	changes, state = Advance(g, rows, nil, true)
	if changes["C"] != "cancelled" || state != model.PODRunCancelRequested {
		t.Fatal(changes, state)
	}
	rows[1].State = "awaitingReconciliation"
	_, state = Advance(g, rows, nil, false)
	if state != model.PODRunAwaitingReconciliation {
		t.Fatal(state)
	}
}
