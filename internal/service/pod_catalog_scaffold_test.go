package service

import (
	"os"
	"path/filepath"
	"testing"

	"ruoyi-go/internal/model"
)

func TestGeneratePODCatalogScaffold(t *testing.T) {
	root := os.Getenv("POD_CATALOG_SCAFFOLD")
	if root == "" { t.Skip("explicit scaffold output required") }
	for _, entity := range []struct{ table, class, business, label, primary string }{
		{"pod_provider", "PODProvider", "provider", "供应商", "provider_id"},
		{"pod_model", "PODCatalogModel", "catalogmodel", "模型", "model_id"},
	} {
		table := &model.GenTable{TableName: entity.table, TableComment: entity.label, ClassName: entity.class, TplCategory: "crud", TplWebType: "element-plus", PackageName: "ruoyi-go", ModuleName: "podAdmin", BusinessName: entity.business, FunctionName: entity.label, FunctionAuthor: "POD", Params: map[string]any{"parentMenuId": 0}, Columns: []model.GenTableColumn{
			{ColumnName: entity.primary, JavaType: "Long", JavaField: "id", IsPK: "1", IsIncrement: "1", IsList: "1", QueryType: "EQ", HTMLType: "input"},
			{ColumnName: "name", JavaType: "String", JavaField: "name", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", IsRequired: "1", QueryType: "LIKE", HTMLType: "input"},
		}}
		table.PKColumn = &table.Columns[0]
		files, err := renderGeneratedFiles(table)
		if err != nil { t.Fatal(err) }
		for _, file := range files {
			target := filepath.Join(root, entity.business, filepath.FromSlash(file.Path))
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil { t.Fatal(err) }
			if err := os.WriteFile(target, []byte(file.Content), 0600); err != nil { t.Fatal(err) }
		}
	}
}
