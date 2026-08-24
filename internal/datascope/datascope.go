// Package datascope 数据权限过滤。[L2]
//
// 对齐 Java 版 DataScopeAspect：按用户角色的 data_scope 拼出 WHERE 条件，
// 多角色取并集（OR 连接）。
//
// 放在 internal 而不是 pkg：它直接依赖 SysUser/SysRole 的结构，
// 是业务专有逻辑，不具备通用性。
package datascope

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
)

// Params 数据权限的构造参数。
type Params struct {
	// User 当前登录用户，Roles 必须已填充 Permissions（登录时由 GetMenuPermission 回填）
	User *model.SysUser
	// DeptAlias 部门字段所在表的别名，如 "d"；为空表示不加前缀
	DeptAlias string
	// UserAlias 用户字段所在表的别名，如 "u"；为空表示该查询没有用户维度
	UserAlias string
	// DeptField 部门字段名，缺省 dept_id
	DeptField string
	// UserField 用户字段名，缺省 user_id
	UserField string
	// Permission 权限标识，如 "system:user:list"。
	// 非空时只有拥有该权限的角色参与数据范围计算。
	Permission string
}

// Scope 返回可直接用于 db.Scopes(...) 的数据权限过滤器。
//
// 超级管理员不过滤。所有角色都不匹配时返回"查不到任何数据"的条件，
// 而不是不加条件 —— 这一点必须与 Java 一致，否则会把全部数据暴露出去。
func Scope(p Params) func(*gorm.DB) *gorm.DB {
	condition, args := build(p)
	return func(db *gorm.DB) *gorm.DB {
		if condition == "" {
			return db
		}
		return db.Where(condition, args...)
	}
}

func build(p Params) (string, []any) {
	user := p.User
	if user == nil {
		// 拿不到用户时一律查不到数据，宁可少给也不能多给
		return denyAll(p), nil
	}
	if user.IsAdmin() {
		return "", nil
	}

	deptCol := qualify(p.DeptAlias, orDefault(p.DeptField, "dept_id"))
	userCol := qualify(p.UserAlias, orDefault(p.UserField, "user_id"))

	// 自定义数据权限可能命中多个角色，合并成一次 IN 查询，避免拼出多个子查询
	customRoleIDs := collectCustomRoleIDs(p)

	var (
		clauses []string
		args    []any
		// 同一种 data_scope 只拼一次
		seen = make(map[string]bool)
		// 是否有任何角色参与了计算，用于区分"没有匹配角色"和"匹配到全部数据权限"
		matched bool
	)

	for _, role := range user.Roles {
		if role.Status != model.StatusNormal {
			continue
		}
		if !roleHasPermission(role, p.Permission) {
			continue
		}
		matched = true

		if seen[role.DataScope] {
			continue
		}
		seen[role.DataScope] = true

		switch role.DataScope {
		case model.DataScopeAll:
			// 全部数据权限直接放行，忽略其它角色的限制
			return "", nil

		case model.DataScopeCustom:
			if len(customRoleIDs) == 0 {
				continue
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(customRoleIDs)), ",")
			clauses = append(clauses, fmt.Sprintf(
				"%s IN (SELECT dept_id FROM sys_role_dept WHERE role_id IN (%s))", deptCol, placeholders))
			for _, id := range customRoleIDs {
				args = append(args, id)
			}

		case model.DataScopeDept:
			clauses = append(clauses, fmt.Sprintf("%s = ?", deptCol))
			args = append(args, deptIDOf(user))

		case model.DataScopeDeptAndChild:
			// 与 Java 一致使用 find_in_set；优化方案见 docs/DECISIONS.md
			clauses = append(clauses, fmt.Sprintf(
				"%s IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors))", deptCol))
			args = append(args, deptIDOf(user), deptIDOf(user))

		case model.DataScopeSelf:
			// 只看 UserAlias，与 Java 的 isNotBlank(userAlias) 一致。
			// 不能把 UserField 也算进来 —— 那样在没有别名的联表查询里
			// 会拼出裸的 user_id，可能命中错误的表。
			if p.UserAlias == "" {
				// 没有用户维度时无法表达"仅本人"，与 Java 一致：查不到任何数据
				clauses = append(clauses, fmt.Sprintf("%s = 0", deptCol))
				continue
			}
			clauses = append(clauses, fmt.Sprintf("%s = ?", userCol))
			args = append(args, user.UserID)
		}
	}

	// 没有任何角色拥有该权限：查不到数据。
	// 【重要】这里绝不能返回空条件，那等于放开全部数据。
	if !matched || len(clauses) == 0 {
		return denyAll(p), nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

// collectCustomRoleIDs 收集自定义数据权限的角色 ID。
func collectCustomRoleIDs(p Params) []int64 {
	var ids []int64
	for _, role := range p.User.Roles {
		if role.DataScope != model.DataScopeCustom || role.Status != model.StatusNormal {
			continue
		}
		if !roleHasPermission(role, p.Permission) {
			continue
		}
		ids = append(ids, role.RoleID)
	}
	return ids
}

// roleHasPermission 权限标识为空时不做过滤，与 Java 的
// `StringUtils.isEmpty(permission) || containsAny(...)` 一致。
func roleHasPermission(role model.SysRole, permission string) bool {
	if permission == "" {
		return true
	}
	for _, p := range role.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// denyAll "查不到任何数据"的条件。
//
// 【不要写成 dept_id = 0】Java 版是这么写的，它依赖 LEFT JOIN 出来的
// d.dept_id 为 NULL 才恒假。一旦把别名换成 u（用户表自身的列），
// dept_id 真的等于 0 的用户就会被放行 —— 兜底条件反而漏了人。
// 用 1 = 0 与别名无关，任何情况下都恒假。
func denyAll(Params) string {
	return "1 = 0"
}

func deptIDOf(user *model.SysUser) int64 {
	if user.DeptID == nil {
		return 0
	}
	return *user.DeptID
}

func qualify(alias, field string) string {
	if alias == "" {
		return field
	}
	return alias + "." + field
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
