package handler

import (
	"reflect"
	"testing"

	"ruoyi-go/internal/model"
)

func TestContractGenTableProjectionKeepsJavaNullShape(t *testing.T) {
	table := model.GenTable{TableID: 7, TableName: "demo_order", TableComment: "订单", TplCategory: "crud", ClassName: "DemoOrder"}
	db := contractGenTable(&table, genProjectionDB)
	if db["tableId"] != nil || db["className"] != nil || db["columns"] != nil || db["tableName"] != "demo_order" {
		t.Fatalf("unexpected db projection: %#v", db)
	}
	list := contractGenTable(&table, genProjectionList)
	if list["tableId"] != int64(7) || list["className"] != "DemoOrder" || list["columns"] != nil {
		t.Fatalf("unexpected list projection: %#v", list)
	}
	if list["crud"] != true || list["sub"] != false || list["tree"] != false {
		t.Fatalf("computed template flags drifted: %#v", list)
	}
}

func TestContractGenColumnsPreservesNilAndComputedFields(t *testing.T) {
	if contractGenColumns(nil) != nil {
		t.Fatal("nil Java list must stay nil")
	}
	columns := contractGenColumns([]model.GenTableColumn{{ColumnID: 1, JavaField: "parentId", IsPK: "1", IsRequired: "1"}})
	want := map[string]any{"capJavaField": "ParentId", "pk": true, "required": true, "superColumn": true, "usableColumn": true}
	for key, value := range want {
		if !reflect.DeepEqual(columns[0][key], value) {
			t.Fatalf("%s = %#v, want %#v", key, columns[0][key], value)
		}
	}
}
