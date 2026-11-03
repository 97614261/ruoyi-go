package service

import (
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestDownloadCategoryScaffold(t *testing.T) {
	out := os.Getenv("PORTAL_DOWNLOAD_SCAFFOLD_OUT")
	if out == "" {
		t.Skip("explicit evidence directory required")
	}
	absolute, err := filepath.Abs(out)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(repo, absolute)
	if err == nil && (rel == "." || !(strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "..")) {
		t.Fatal("生成留证必须放仓库外，避免被go test ./...当成源码")
	}
	table := sampleRenderTable()
	table.TableName = "cms_download_category"
	table.TableComment = "资料分类"
	table.ClassName = "CmsDownloadCategory"
	table.ModuleName = "cms"
	table.BusinessName = "downloadCategory"
	table.FunctionName = "资料分类"
	table.Columns = []model.GenTableColumn{{ColumnName: "category_id", ColumnType: "bigint", JavaType: "Long", JavaField: "categoryId", IsPK: "1", IsIncrement: "1", IsList: "1"}, {ColumnName: "name", ColumnType: "varchar(60)", JavaType: "String", JavaField: "name", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", QueryType: "LIKE"}}
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
	t.Log("generated CRUD retained; adapted into permission/CAS/private attachment boundaries")
}
