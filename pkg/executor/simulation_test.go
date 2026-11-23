package executor

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"testing"
)

func TestSimulationQueryDoesNotResubmit(t *testing.T) {
	p := Simulation{}
	r := Request{Key: "fixed", Scenario: "uncertain", Attempt: 1, Node: model.PODNode{Type: "textGenerate", Config: json.RawMessage(`{}`)}, Input: model.PODRunOutput{TestOnly: true, Text: "example"}}
	first := p.Submit(context.Background(), r)
	if first.State != "awaitingReconciliation" {
		t.Fatal(first)
	}
	a, b := p.Query(context.Background(), r), p.Query(context.Background(), r)
	if a.State != "succeeded" || a.Output.Text != b.Output.Text || !a.Output.TestOnly {
		t.Fatal("query not deterministic")
	}
	r.Scenario = "failOnce"
	if p.Submit(context.Background(), r).State != "failed" {
		t.Fatal("expected first failure")
	}
	r.Attempt = 2
	if p.Submit(context.Background(), r).State != "succeeded" {
		t.Fatal("recovery failed")
	}
}
