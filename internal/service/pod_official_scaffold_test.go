package service

import (
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"testing"
)

func TestGeneratePODOfficialScaffold(t *testing.T) {
	root := os.Getenv("POD_OFFICIAL_SCAFFOLD")
	if root == "" {
		t.Skip("explicit scaffold output required")
	}
	for _, e := range []struct{ table, class, business, pk string }{{"pod_catalog_template", "PODOfficialTemplate", "officialtemplate", "template_id"}, {"pod_catalog_asset", "PODOfficialAsset", "officialasset", "asset_id"}} {
		table := &model.GenTable{TableName: e.table, TableComment: e.business, ClassName: e.class, TplCategory: "crud", TplWebType: "element-plus", PackageName: "ruoyi-go", ModuleName: "podAdmin", BusinessName: e.business, FunctionName: e.business, FunctionAuthor: "POD", Params: map[string]any{"parentMenuId": 0}, Columns: []model.GenTableColumn{{ColumnName: e.pk, JavaType: "Long", JavaField: "id", IsPK: "1", IsIncrement: "1", IsList: "1", QueryType: "EQ", HTMLType: "input"}, {ColumnName: "name", JavaType: "String", JavaField: "name", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", IsRequired: "1", QueryType: "LIKE", HTMLType: "input"}}}
		table.PKColumn = &table.Columns[0]
		files, err := renderGeneratedFiles(table)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			path := filepath.Join(root, e.business, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(f.Content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
