package repository

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestQuoteViewUsesFrozenModelAndImageCountWithoutPrivateMetadata(t *testing.T) {
	plan := model.PODQuotePlan{Models: []model.PODBoundModel{{Kind: "image", ModelID: 10, ModelVersionID: 21, Name: "Original model", ImageCount: 3, Points: 75, Version: model.PODCatalogModelVersion{OfficialModelID: "frozen-image-model", Endpoint: "https://internal-provider.invalid"}}}}
	raw, _ := json.Marshal(plan)
	view, err := quoteView(model.PODExecutionQuote{QuoteID: "original-quote", SelectionJSON: raw, PointsUpperBound: 75})
	if err != nil || len(view.Lines) != 1 {
		t.Fatal("quote unreadable", err)
	}
	line := view.Lines[0]
	if line.OfficialModelID != "frozen-image-model" || line.Kind != "image" || line.Images != 3 || line.Points != 75 || line.ModelVersionID != 21 {
		t.Fatal("quote lost version or count", line)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "internal-provider") || strings.Contains(string(encoded), "endpoint") || strings.Contains(string(encoded), "costRule") {
		t.Fatal("private catalog metadata leaked into creator quote")
	}
}
