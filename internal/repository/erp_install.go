package repository

import (
	"context"
	"fmt"
	"regexp"
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

// MySQL 5.7 has no ADD COLUMN IF NOT EXISTS. This explicit local-only runner
// verifies the named object before adding it. It is not a server auto-migrator.
func InstallERPControls(ctx context.Context, script string) error {
	q := DB(tenantctx.System(ctx))
	var name string
	if err := q.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
		return err
	}
	if name != TenantDatabaseName {
		return fmt.Errorf("ERP升级仅允许独立本地库")
	}
	var clean []string
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			clean = append(clean, line)
		}
	}
	pattern := regexp.MustCompile(`^ALTER TABLE (erp_document|erp_document_line|erp_stock) ADD (COLUMN|INDEX|CONSTRAINT) ([a-z_]+) (.+)$`)
	statements := []string{}
	for _, s := range strings.Split(strings.Join(clean, "\n"), ";") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, "CREATE TABLE IF NOT EXISTS erp_warehouse_") && pattern.FindStringSubmatch(s) == nil {
			return fmt.Errorf("ERP升级包含不允许的语句")
		}
		statements = append(statements, s)
	}
	for _, s := range statements {
		if strings.HasPrefix(s, "CREATE TABLE IF NOT EXISTS erp_warehouse_") {
			if err := q.Exec(s).Error; err != nil {
				return err
			}
			continue
		}
		m := pattern.FindStringSubmatch(s)
		table, kind, object := m[1], m[2], m[3]
		var count int64
		var query string
		switch kind {
		case "COLUMN":
			query = "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?"
		case "INDEX":
			query = "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?"
		case "CONSTRAINT":
			query = "SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema=DATABASE() AND table_name=? AND constraint_name=?"
		}
		if err := q.Raw(query, table, object).Scan(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := q.Exec(s).Error; err != nil {
				return err
			}
		}
	}
	return VerifyERPControls(ctx)
}
