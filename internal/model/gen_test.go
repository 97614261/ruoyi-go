package model

import (
	"encoding/json"
	"testing"
)

func TestGenModelsExposeJavaComputedProperties(t *testing.T) {
	tableJSON, err := json.Marshal(GenTable{TplCategory: "crud"})
	if err != nil {
		t.Fatal(err)
	}
	var table map[string]any
	if err := json.Unmarshal(tableJSON, &table); err != nil {
		t.Fatal(err)
	}
	if table["crud"] != true || table["tree"] != false || table["sub"] != false {
		t.Fatalf("computed table properties=%v", table)
	}
	if _, ok := table["pkColumn"]; !ok {
		t.Fatal("pkColumn must be present with Java null semantics")
	}

	columnJSON, err := json.Marshal(GenTableColumn{JavaField: "parentId", IsPK: "1", IsEdit: "0"})
	if err != nil {
		t.Fatal(err)
	}
	var column map[string]any
	if err := json.Unmarshal(columnJSON, &column); err != nil {
		t.Fatal(err)
	}
	if column["capJavaField"] != "ParentId" || column["pk"] != true || column["usableColumn"] != true {
		t.Fatalf("computed column properties=%v", column)
	}
}
