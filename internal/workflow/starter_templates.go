package workflow

import (
	_ "embed"
	"encoding/json"
	"ruoyi-go/internal/model"
)

//go:embed starter_templates.json
var starterTemplatesJSON []byte

type StarterTemplate struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Sort        int            `json:"sort"`
	Graph       model.PODGraph `json:"graph"`
}

// Return independent graphs so callers cannot mutate another installation's seed.
func StarterTemplates() ([]StarterTemplate, error) {
	var templates []StarterTemplate
	err := json.Unmarshal(starterTemplatesJSON, &templates)
	return templates, err
}
