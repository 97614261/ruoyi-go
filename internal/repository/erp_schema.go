package repository

import (
	"context"
	"fmt"
	"ruoyi-go/pkg/tenantctx"
	"strings"
)

// Read-only startup gate. No CREATE/ALTER/seed or cache work happens here.
func VerifyERPControls(ctx context.Context) error {
	q := DB(tenantctx.System(ctx))
	var installed int64
	if err := q.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('erp_sku','erp_warehouse','erp_document','erp_document_line','erp_stock','erp_stock_ledger','erp_warehouse_access','erp_warehouse_policy')").Scan(&installed).Error; err != nil {
		return err
	}
	if installed == 0 {
		return nil
	} // Pure multitenant foundation need not install ERP.
	if installed != 8 {
		return fmt.Errorf("ERP表未完整初始化，请备份后显式执行erpinit")
	}
	var columns []struct {
		TableName    string
		ColumnName   string
		ColumnType   string
		IsNullable   string
		DefaultValue string
	}
	if err := q.Raw("SELECT table_name,column_name,column_type,is_nullable,COALESCE(column_default,'<null>') AS default_value FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ('erp_document','erp_document_line','erp_stock','erp_warehouse_access','erp_warehouse_policy')").Scan(&columns).Error; err != nil {
		return err
	}
	byName := map[string]struct{ typ, nullable, def string }{}
	for _, c := range columns {
		byName[c.TableName+"."+c.ColumnName] = struct{ typ, nullable, def string }{c.ColumnType, c.IsNullable, c.DefaultValue}
	}
	for _, s := range []struct{ key, typ, nullable, def string }{
		{"erp_document.version", "bigint", "NO", "1"}, {"erp_document.source_document_id", "bigint", "YES", "<null>"}, {"erp_document.reversal", "tinyint(1)", "NO", "0"},
		{"erp_document_line.expected_on_hand", "bigint", "NO", "0"}, {"erp_document_line.expected_reserved", "bigint", "NO", "0"}, {"erp_document_line.expected_version", "bigint", "NO", "0"},
		{"erp_stock.version", "bigint", "NO", "0"}, {"erp_warehouse_access.tenant_id", "bigint", "NO", "<null>"}, {"erp_warehouse_access.user_id", "bigint", "NO", "<null>"}, {"erp_warehouse_access.warehouse_id", "bigint", "NO", "<null>"},
		{"erp_warehouse_policy.tenant_id", "bigint", "NO", "<null>"}, {"erp_warehouse_policy.user_id", "bigint", "NO", "<null>"}, {"erp_warehouse_policy.version", "bigint", "NO", "1"},
	} {
		c, ok := byName[s.key]
		if !ok || !strings.HasPrefix(c.typ, s.typ) || c.nullable != s.nullable || c.def != s.def {
			return fmt.Errorf("ERP结构未就绪或存在漂移：%s；请备份后显式执行erpinit", s.key)
		}
	}
	var indexes []struct {
		TableName  string
		IndexName  string
		ColumnName string
		SeqInIndex int
		NonUnique  int
	}
	if err := q.Raw("SELECT table_name,index_name,column_name,seq_in_index,non_unique FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN ('erp_document','erp_warehouse_access','erp_warehouse_policy') ORDER BY table_name,index_name,seq_in_index").Scan(&indexes).Error; err != nil {
		return err
	}
	indexCols := map[string]string{}
	uniques := map[string]bool{}
	for _, i := range indexes {
		key := i.TableName + "." + i.IndexName
		if indexCols[key] != "" {
			indexCols[key] += ","
		}
		indexCols[key] += i.ColumnName
		uniques[key] = i.NonUnique == 0
	}
	for _, s := range []struct {
		key, cols string
		unique    bool
	}{
		{"erp_document.ix_erp_document_source", "tenant_id,source_document_id,status,document_id", false},
		{"erp_warehouse_access.uk_erp_access_user", "tenant_id,user_id,warehouse_id", true},
		{"erp_warehouse_policy.uk_erp_policy_user", "tenant_id,user_id", true},
	} {
		if indexCols[s.key] != s.cols || uniques[s.key] != s.unique {
			return fmt.Errorf("ERP索引缺失或漂移：%s", s.key)
		}
	}
	var foreignKeys []struct {
		TableName            string
		ConstraintName       string
		ColumnName           string
		ReferencedTableName  string
		ReferencedColumnName string
		OrdinalPosition      int
	}
	if err := q.Raw("SELECT table_name,constraint_name,column_name,referenced_table_name,referenced_column_name,ordinal_position FROM information_schema.key_column_usage WHERE constraint_schema=DATABASE() AND referenced_table_name IS NOT NULL AND table_name IN ('erp_document','erp_warehouse_access','erp_warehouse_policy') ORDER BY table_name,constraint_name,ordinal_position").Scan(&foreignKeys).Error; err != nil {
		return err
	}
	fkCols := map[string]string{}
	for _, f := range foreignKeys {
		key := f.TableName + "." + f.ConstraintName
		fkCols[key] += f.ColumnName + ">" + f.ReferencedTableName + "." + f.ReferencedColumnName + ";"
	}
	for key, want := range map[string]string{
		"erp_document.fk_erp_document_source":          "tenant_id>erp_document.tenant_id;source_document_id>erp_document.document_id;",
		"erp_warehouse_access.fk_erp_access_user":      "tenant_id>sys_user.tenant_id;user_id>sys_user.user_id;",
		"erp_warehouse_access.fk_erp_access_warehouse": "tenant_id>erp_warehouse.tenant_id;warehouse_id>erp_warehouse.warehouse_id;",
		"erp_warehouse_policy.fk_erp_policy_user":      "tenant_id>sys_user.tenant_id;user_id>sys_user.user_id;",
	} {
		if fkCols[key] != want {
			return fmt.Errorf("ERP复合外键缺失或漂移：%s", key)
		}
	}
	return nil
}
