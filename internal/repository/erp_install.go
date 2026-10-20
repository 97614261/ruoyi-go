package repository

import (
	"context"
	"fmt"
	"ruoyi-go/pkg/tenantctx"
	"strings"
)

// InstallERPSchema is invoked by the explicit local CLI, never server startup.
func InstallERPSchema(ctx context.Context, script string) error {
	q := DB(tenantctx.System(ctx))
	var name string
	if err := q.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
		return err
	}
	if name != TenantDatabaseName {
		return fmt.Errorf("ERP初始化拒绝原库和未知数据库")
	}
	lines := []string{}
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if !strings.HasPrefix(statement, "CREATE TABLE IF NOT EXISTS erp_") {
			return fmt.Errorf("ERP初始化仅允许新增ERP表")
		}
		if err := q.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
