package service

import (
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"testing"
)

func TestCollaborationScaffold(t *testing.T) {
	out := os.Getenv("PORTAL_COLLABORATION_SCAFFOLD_OUT")
	if out == "" {
		t.Skip("explicit output required")
	}
	table := sampleRenderTable()
	table.TableName = "cms_inquiry_followup"
	table.TableComment = "询盘跟进"
	table.ClassName = "CmsInquiryFollowup"
	table.ModuleName = "cms"
	table.BusinessName = "followup"
	table.FunctionName = "询盘跟进"
	table.Columns = []model.GenTableColumn{{ColumnName: "followup_id", ColumnType: "bigint", JavaType: "Long", JavaField: "followupId", IsPK: "1", IsIncrement: "1", IsList: "1"}, {ColumnName: "inquiry_id", ColumnType: "bigint", JavaType: "Long", JavaField: "inquiryId", IsInsert: "1", IsList: "1", IsQuery: "1", QueryType: "EQ"}, {ColumnName: "text", ColumnType: "varchar(2000)", JavaType: "String", JavaField: "text", IsInsert: "1", HTMLType: "textarea"}}
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
	t.Log("scaffold retained as evidence; generic row CRUD replaced by scoped transactional workflow commands")
}
func TestWorkflowInput(t *testing.T) {
	in := model.CMSWorkflowInput{Required: map[string]bool{"ARTICLE": false, "PRODUCT": false, "PAGE": false, "SITE": false}, Reason: "test"}
	if e := cmsValidateWorkflow(in); e != nil {
		t.Fatal(e)
	}
	in.Required["OTHER"] = true
	if cmsValidateWorkflow(in) == nil {
		t.Fatal("unknown kind accepted")
	}
	delete(in.Required, "OTHER")
	in.ManagerIds = []int64{1, 1}
	if cmsValidateWorkflow(in) == nil {
		t.Fatal("duplicate recipients accepted")
	}
}
