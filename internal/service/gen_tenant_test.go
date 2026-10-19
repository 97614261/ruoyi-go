package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestGenerationRequiresTenantColumnAndHidesClientControls(t *testing.T) {
	table := &model.GenTable{Columns: []model.GenTableColumn{{ColumnName: "id", JavaField: "id"}}}
	if requireTenantColumn(table) == nil {
		t.Fatal("missing tenant column accepted")
	}
	table.Columns = append(table.Columns, model.GenTableColumn{ColumnName: "tenant_id", JavaField: "tenantId", ColumnType: "bigint", IsInsert: "1", IsEdit: "1", IsQuery: "1", IsList: "1"})
	if err := requireTenantColumn(table); err != nil {
		t.Fatal(err)
	}
	c := table.Columns[1]
	if c.IsInsert != "0" || c.IsEdit != "0" || c.IsQuery != "0" || c.IsList != "0" {
		t.Fatal("client controls tenant column")
	}
	s := tenantAwareRepositorySource(`DB(ctx).Table("biz_order").Count(&count); tx.Table("biz_line").Delete(&model.Line{})`)
	if strings.Contains(s, `.Table(`) || !strings.Contains(s, `TenantTable(DB(ctx), "biz_order")`) || !strings.Contains(s, `TenantTable(tx, "biz_line")`) {
		t.Fatal("generated scalar/subtable queries are not scoped:", s)
	}
}
