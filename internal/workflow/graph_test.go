package workflow

import (
	"encoding/json"
	"math"
	"testing"

	"ruoyi-go/internal/model"
)

func graphNode(id, kind string) model.PODNode {
	return model.PODNode{ID: id, Type: kind, Version: 1, Config: json.RawMessage(`{}`)}
}
func TestValidateGraphDraftAndConnectedFlow(t *testing.T) {
	for _, graph := range []model.PODGraph{
		{SchemaVersion: 1, Nodes: []model.PODNode{}, Edges: []model.PODEdge{}},
		{SchemaVersion: 1, Nodes: []model.PODNode{graphNode("prompt", "textInput"), graphNode("image", "imageGenerate")}, Edges: []model.PODEdge{{ID: "edge", Source: "prompt", SourcePort: "text", Target: "image", TargetPort: "text"}}},
	} {
		if err := Validate(graph); err != nil {
			t.Fatal(err)
		}
	}
}
func TestValidateGraphRejectsInvalidDrafts(t *testing.T) {
	base := func() model.PODGraph {
		return model.PODGraph{SchemaVersion: 1, Nodes: []model.PODNode{graphNode("a", "textGenerate"), graphNode("b", "textGenerate")}, Edges: []model.PODEdge{{ID: "ab", Source: "a", SourcePort: "text", Target: "b", TargetPort: "text"}}}
	}
	cases := []struct {
		name   string
		mutate func(*model.PODGraph)
	}{
		{"schema", func(g *model.PODGraph) { g.SchemaVersion = 2 }},
		{"null array", func(g *model.PODGraph) { g.Edges = nil }},
		{"duplicate node", func(g *model.PODGraph) { g.Nodes[1].ID = "a" }},
		{"unknown node", func(g *model.PODGraph) { g.Nodes[0].Type = "arbitraryHttp" }},
		{"node version", func(g *model.PODGraph) { g.Nodes[0].Version = 2 }},
		{"invalid config", func(g *model.PODGraph) { g.Nodes[0].Config = json.RawMessage(`null`) }},
		{"missing node", func(g *model.PODGraph) { g.Edges[0].Source = "missing" }},
		{"wrong port type", func(g *model.PODGraph) { g.Edges[0].TargetPort = "images" }},
		{"cycle", func(g *model.PODGraph) {
			g.Edges = append(g.Edges, model.PODEdge{ID: "ba", Source: "b", SourcePort: "text", Target: "a", TargetPort: "text"})
		}},
		{"duplicate target", func(g *model.PODGraph) { e := g.Edges[0]; e.ID = "second"; g.Edges = append(g.Edges, e) }},
		{"infinite coordinate", func(g *model.PODGraph) { g.Nodes[0].Position.X = math.Inf(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := base()
			tc.mutate(&g)
			if Validate(g) == nil {
				t.Fatal("expected invalid draft")
			}
		})
	}
}
