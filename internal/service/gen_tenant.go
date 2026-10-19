package service

import (
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"strings"
)

var generatedTableCall = regexp.MustCompile(`(DB\(ctx\)|[A-Za-z_][A-Za-z0-9_]*)\.Table\(("[A-Za-z_][A-Za-z0-9_]*")\)`)

func tenantAwareRepositorySource(source string) string {
	return generatedTableCall.ReplaceAllString(source, `TenantTable($1, $2)`)
}

func requireTenantColumn(t *model.GenTable) error {
	found := false
	for i := range t.Columns {
		c := &t.Columns[i]
		if strings.EqualFold(c.ColumnName, "tenant_id") {
			if !strings.HasPrefix(strings.ToLower(c.ColumnType), "bigint") {
				return errs.New("tenant_id必须是BIGINT类型")
			}
			c.IsInsert = "0"
			c.IsEdit = "0"
			c.IsList = "0"
			c.IsQuery = "0"
			c.IsRequired = "0"
			found = true
		}
	}
	if !found {
		return errs.New("多租户业务表必须先添加无默认值的tenant_id BIGINT NOT NULL；禁止生成无租户隔离的业务代码")
	}
	return nil
}
