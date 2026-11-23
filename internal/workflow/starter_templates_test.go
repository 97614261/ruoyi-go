package workflow

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestStarterTemplatesArePortableAndValidate(t *testing.T) {
	seeds, err := StarterTemplates()
	if err != nil || len(seeds) != 3 {
		t.Fatal("starter templates unavailable", err)
	}
	codes := map[string]bool{}
	for _, s := range seeds {
		if codes[s.Code] || s.Name == "" || !strings.Contains(s.Description, "1.") {
			t.Fatal("missing identity or instructions")
		}
		codes[s.Code] = true
		if err := ValidateWithLimits(s.Graph, model.PODGraphLimits{}); err != nil {
			t.Fatalf("%s: %v", s.Code, err)
		}
		for _, node := range s.Graph.Nodes {
			raw := string(node.Config)
			if strings.Contains(raw, "modelId") || strings.Contains(raw, "apiKey") || strings.Contains(raw, "https:") {
				t.Fatal("environment or credentials leaked into starter", s.Code)
			}
		}
	}
	seeds[0].Graph.Nodes[0].ID = "changed"
	again, _ := StarterTemplates()
	if again[0].Graph.Nodes[0].ID == "changed" {
		t.Fatal("starter graphs share mutable state")
	}
}
