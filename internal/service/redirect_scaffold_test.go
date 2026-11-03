package service

import (
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"testing"
)

// Explicitly render the normal CRUD before specializing URL/publication boundaries.
func TestRedirectScaffold(t *testing.T) {
	out := os.Getenv("PORTAL_REDIRECT_SCAFFOLD_OUT")
	if out == "" {
		t.Skip("explicit output required")
	}
	table := sampleRenderTable()
	table.TableName = "cms_redirect"
	table.TableComment = "旧地址跳转"
	table.ClassName = "CmsRedirect"
	table.ModuleName = "cms"
	table.BusinessName = "redirect"
	table.FunctionName = "旧地址跳转"
	table.Columns = []model.GenTableColumn{
		{ColumnName: "redirect_id", ColumnType: "bigint", ColumnComment: "编号", JavaType: "Long", JavaField: "redirectId", IsPK: "1", IsIncrement: "1", IsList: "1"},
		{ColumnName: "source_path", ColumnType: "varchar(200)", ColumnComment: "旧路径", JavaType: "String", JavaField: "sourcePath", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", QueryType: "LIKE", HTMLType: "input"},
		{ColumnName: "target_id", ColumnType: "bigint", ColumnComment: "目标内容编号", JavaType: "Long", JavaField: "targetId", IsInsert: "1", IsEdit: "1", IsList: "1", HTMLType: "input"},
		{ColumnName: "status", ColumnType: "char(1)", ColumnComment: "状态", JavaType: "String", JavaField: "status", IsList: "1", IsQuery: "1", QueryType: "EQ", HTMLType: "select"},
	}
	table.PKColumn = &table.Columns[0]
	files, e := renderGeneratedFiles(table)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range files {
		p := filepath.Join(out, filepath.FromSlash(f.Path))
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p, []byte(f.Content), 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log("generated CRUD scaffold for permission/CAS/redirect specialization")
}
