package repository

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"regexp"
	"ruoyi-go/pkg/tenantctx"
	"strings"
)

var tenantTables = map[string]bool{
	"sys_user": true, "sys_role": true, "sys_dept": true, "sys_post": true,
	"sys_config": true, "sys_dict_type": true, "sys_dict_data": true, "sys_notice": true,
	"sys_user_role": true, "sys_user_post": true, "sys_role_menu": true, "sys_role_dept": true,
	"sys_notice_read": true, "sys_oper_log": true, "sys_logininfor": true,
	"sys_job": true, "sys_job_log": true, "sys_file": true, "sys_tenant_menu": true,
	"sys_tenant_policy": true,
	"erp_sku":           true, "erp_warehouse": true, "erp_document": true,
	"erp_document_line": true, "erp_stock": true, "erp_stock_ledger": true,
	"erp_warehouse_access": true,
	"erp_warehouse_policy": true,
	"erp_shop":             true, "erp_shop_access": true, "erp_shop_policy": true,
	"erp_shop_item": true, "erp_shop_sku_mapping": true,
	"erp_order": true, "erp_order_line": true, "erp_order_event": true,
	"erp_package": true, "erp_package_line": true,
	"erp_import_job": true, "erp_import_result": true,
	"erp_supplier": true, "erp_purchase_order": true, "erp_purchase_order_line": true,
	"erp_purchase_receipt": true, "erp_purchase_receipt_line": true, "erp_purchase_event": true,
}
var tenantJoinPattern = regexp.MustCompile(`(?i)((?:(?:LEFT|RIGHT|INNER|CROSS)(?:\s+OUTER)?\s+)?JOIN\s+)([a-z_]+)(?:\s+(?:AS\s+)?([a-z_]+))?(\s+ON\s+)`)
var tenantBusinessNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func registerTenantGuard(g *gorm.DB) error {
	for _, reg := range []func() error{
		func() error {
			return g.Callback().Query().Before("gorm:query").Register("tenant:query", tenantReadGuard)
		},
		func() error { return g.Callback().Row().Before("gorm:row").Register("tenant:row", tenantReadGuard) },
		func() error {
			return g.Callback().Create().Before("gorm:before_create").Register("tenant:create", tenantCreateGuard)
		},
		func() error {
			return g.Callback().Update().Before("gorm:before_update").Register("tenant:update", tenantWriteGuard)
		},
		func() error {
			return g.Callback().Delete().Before("gorm:before_delete").Register("tenant:delete", tenantWriteGuard)
		},
		func() error {
			return g.Callback().Raw().Before("gorm:raw").Register("tenant:raw", func(tx *gorm.DB) {
				s := tenantctx.Get(tx.Statement.Context)
				if !s.System && !s.Platform {
					tx.AddError(errors.New("租户请求禁止执行原始SQL"))
				}
			})
		},
	} {
		if err := reg(); err != nil {
			return err
		}
	}
	return nil
}
func tenantTable(tx *gorm.DB) (string, string) {
	table, alias := tx.Statement.Table, tx.Statement.Table
	if tx.Statement.TableExpr != nil {
		parts := strings.Fields(strings.ReplaceAll(tx.Statement.TableExpr.SQL, "`", ""))
		if len(parts) > 0 {
			table = parts[0]
			alias = table
			if len(parts) > 1 {
				alias = parts[len(parts)-1]
			}
		}
	} else if tx.Statement.Schema != nil {
		table = tx.Statement.Schema.Table
		if alias == "" {
			alias = table
		}
	}
	return table, alias
}
func scopedTenantTable(tx *gorm.DB, table string) bool {
	marked, _ := tx.Get("tenant:business_table")
	return tenantTables[table] || marked == table || (tx.Statement.Schema != nil && tx.Statement.Schema.LookUpField("TenantID") != nil)
}

// TenantTable marks a known generated business table even for Count/Scan or
// map updates, which have no model schema. The marker never disables filtering.
func TenantTable(q *gorm.DB, name string) *gorm.DB {
	q = q.Session(&gorm.Session{})
	if !tenantBusinessNamePattern.MatchString(name) || name == "erp_schema_migration" || name == "sys_menu" || name == "sys_tenant" || name == "sys_tenant_menu" || name == "sys_tenant_policy" || strings.HasPrefix(name, "gen_") {
		q.AddError(errors.New("非法或平台专属的业务表"))
		return q
	}
	return q.Table(name).Set("tenant:business_table", name)
}

type tenantGroupedCondition struct{ expression clause.Expression }

func (g tenantGroupedCondition) Build(b clause.Builder) {
	b.WriteByte('(')
	g.expression.Build(b)
	b.WriteByte(')')
}
func appendTenantCondition(tx *gorm.DB, condition clause.Expression) {
	var expressions []clause.Expression
	if old, ok := tx.Statement.Clauses["WHERE"]; ok && old.Expression != nil {
		expressions = append(expressions, tenantGroupedCondition{old.Expression})
	}
	expressions = append(expressions, condition)
	tx.Statement.Clauses["WHERE"] = clause.Clause{Name: "WHERE", Expression: clause.Where{Exprs: expressions}}
}
func applyTenantWhere(tx *gorm.DB) {
	s := tenantctx.Get(tx.Statement.Context)
	if s.System {
		return
	}
	if s.ID < 1 {
		tx.AddError(tenantctx.ErrMissing)
		return
	}
	table, alias := tenantTable(tx)
	switch {
	case scopedTenantTable(tx, table), table == "sys_tenant":
		appendTenantCondition(tx, clause.Eq{Column: clause.Column{Table: alias, Name: "tenant_id"}, Value: s.ID})
	case table == "sys_menu":
		if !s.Platform {
			appendTenantCondition(tx, clause.Expr{SQL: alias + ".menu_id IN (SELECT menu_id FROM sys_tenant_menu WHERE tenant_id = ?) AND " + customerMenuCondition(alias), Vars: []any{s.ID}})
		}
	case table == "gen_table", table == "gen_table_column", strings.HasPrefix(table, "information_schema."):
		if !s.Platform {
			tx.AddError(errors.New("仅平台可访问开发元数据"))
		}
	default:
		tx.AddError(fmt.Errorf("未登记租户归属的表: %s", table))
	}
	for i := range tx.Statement.Joins {
		j := &tx.Statement.Joins[i]
		j.Name = guardTenantJoin(tx, j.Name, s.ID, s.Platform)
	}
}

// Only the project's simple table joins are supported. Complex/subquery joins
// must receive an explicit reviewed implementation rather than silently escape.
func guardTenantJoin(tx *gorm.DB, sql string, id int64, platform bool) string {
	matches := tenantJoinPattern.FindAllStringSubmatchIndex(sql, -1)
	if len(matches) == 0 {
		tx.AddError(errors.New("无法识别关联表的租户归属"))
		return sql
	}
	var out strings.Builder
	cursor := 0
	for i, m := range matches {
		out.WriteString(sql[cursor:m[0]])
		table := sql[m[4]:m[5]]
		alias := table
		if m[6] >= 0 {
			alias = sql[m[6]:m[7]]
		}
		end := len(sql)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		condition := strings.TrimSpace(sql[m[1]:end])
		guard := "1=1"
		switch {
		case tenantTables[table] || table == "sys_tenant":
			guard = fmt.Sprintf("%s.tenant_id = %d", alias, id)
		case table == "sys_menu":
			if !platform {
				guard = fmt.Sprintf("%s.menu_id IN (SELECT menu_id FROM sys_tenant_menu WHERE tenant_id=%d) AND ", alias, id) + customerMenuCondition(alias)
			}
		default:
			tx.AddError(fmt.Errorf("未登记关联表的租户归属: %s", table))
		}
		out.WriteString(sql[m[0]:m[1]])
		out.WriteString(guard + " AND (" + condition + ") ")
		cursor = end
	}
	out.WriteString(sql[cursor:])
	return strings.TrimSpace(out.String())
}
func tenantReadGuard(tx *gorm.DB) { applyTenantWhere(tx) }
func tenantWriteGuard(tx *gorm.DB) {
	s := tenantctx.Get(tx.Statement.Context)
	if s.System {
		return
	}
	table, _ := tenantTable(tx)
	if (table == "sys_menu" || table == "sys_tenant" || table == "sys_tenant_menu" || table == "sys_tenant_policy" || table == "gen_table" || table == "gen_table_column") && !s.Platform {
		tx.AddError(errors.New("仅平台可修改平台资源"))
		return
	}
	if m, ok := tx.Statement.Dest.(map[string]interface{}); ok {
		if _, exists := m["tenant_id"]; exists {
			tx.AddError(errors.New("不允许修改数据所属租户"))
			return
		}
	}
	tx.Statement.Omits = append(tx.Statement.Omits, "tenant_id")
	applyTenantWhere(tx)
}
func tenantCreateGuard(tx *gorm.DB) {
	s := tenantctx.Get(tx.Statement.Context)
	if s.System {
		return
	}
	if s.ID < 1 {
		tx.AddError(tenantctx.ErrMissing)
		return
	}
	table, _ := tenantTable(tx)
	if !scopedTenantTable(tx, table) {
		if !s.Platform {
			tx.AddError(errors.New("仅平台可新增平台资源"))
		}
		return
	}
	if (table == "sys_tenant_menu" || table == "sys_tenant_policy") && !s.Platform {
		tx.AddError(errors.New("仅平台可开通租户功能"))
		return
	}
	if tx.Statement.Schema == nil {
		tx.AddError(errors.New("租户写入必须使用已登记实体"))
		return
	}
	field := tx.Statement.Schema.LookUpField("TenantID")
	if field == nil {
		tx.AddError(errors.New("租户实体缺少TenantID字段"))
		return
	}
	var set func(reflect.Value)
	set = func(v reflect.Value) {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			for i := 0; i < v.Len(); i++ {
				set(v.Index(i))
			}
			return
		}
		if v.Kind() != reflect.Struct {
			tx.AddError(errors.New("租户写入必须使用已登记实体"))
			return
		}
		existing, _ := field.ValueOf(tx.Statement.Context, v)
		if id, ok := existing.(int64); ok && id != 0 && id != s.ID {
			tx.AddError(errors.New("禁止跨租户写入"))
			return
		}
		if err := field.Set(tx.Statement.Context, v, s.ID); err != nil {
			tx.AddError(err)
		}
	}
	set(tx.Statement.ReflectValue)
}
