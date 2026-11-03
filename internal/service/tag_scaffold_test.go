package service

import (
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"testing"
)

func TestTagScaffold(t *testing.T) {
	out := os.Getenv("PORTAL_TAG_SCAFFOLD_OUT")
	if out == "" {
		t.Skip("explicit output required")
	}
	table := sampleRenderTable()
	table.TableName = "cms_tag"
	table.TableComment = "内容标签"
	table.ClassName = "CmsTag"
	table.ModuleName = "cms"
	table.BusinessName = "tag"
	table.FunctionName = "内容标签"
	table.Columns = []model.GenTableColumn{{ColumnName: "tag_id", ColumnType: "bigint", ColumnComment: "编号", JavaType: "Long", JavaField: "tagId", IsPK: "1", IsIncrement: "1", IsList: "1"}, {ColumnName: "name", ColumnType: "varchar(40)", ColumnComment: "标签名称", JavaType: "String", JavaField: "name", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", QueryType: "LIKE", HTMLType: "input"}, {ColumnName: "status", ColumnType: "char(1)", ColumnComment: "状态", JavaType: "String", JavaField: "status", IsInsert: "1", IsEdit: "1", IsList: "1", HTMLType: "select"}}
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
	t.Log("tag CRUD scaffold rendered before specialization")
}
