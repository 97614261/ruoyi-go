package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// RoleTableAlias 角色表在列表查询里的别名。
const RoleTableAlias = "r"

// SelectRolesByUserID 查用户拥有的角色。
//
// 对应 Java 版 SysRoleMapper.selectRolePermissionByUserId。
// 注意只过滤 del_flag，不过滤 status —— 停用的角色也要返回，
// 由上层在计算权限时跳过（数据权限的并集计算依赖完整角色列表）。
func SelectRolesByUserID(ctx context.Context, userID int64) ([]model.SysRole, error) {
	var roles []model.SysRole
	err := DB(ctx).
		Table("sys_role r").
		Distinct("r.*").
		Joins("LEFT JOIN sys_user_role ur ON ur.role_id = r.role_id").
		Where("r.del_flag = ?", model.DelFlagExist).
		Where("ur.user_id = ?", userID).
		Find(&roles).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的角色失败: %w", userID, err)
	}
	return roles, nil
}

// SelectRoleIDsByUserID 查用户的角色 ID 列表。
func SelectRoleIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).
		Table("sys_role r").
		Joins("LEFT JOIN sys_user_role ur ON ur.role_id = r.role_id").
		Where("ur.user_id = ?", userID).
		Order("r.role_id").
		Pluck("r.role_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的角色ID失败: %w", userID, err)
	}
	return ids, nil
}

// roleListDB 构造角色列表的基础查询。
//
// 与 Java 版 selectRoleVo 一致，join 出 sys_dept 供数据权限过滤使用
// （@DataScope(deptAlias = "d")，注意角色列表没有 userAlias）。
func roleListDB(ctx context.Context, query model.RoleQuery, scope func(*gorm.DB) *gorm.DB) *gorm.DB {
	db := DB(ctx).
		Table("sys_role r").
		Joins("LEFT JOIN sys_user_role ur ON ur.role_id = r.role_id").
		Joins("LEFT JOIN sys_user u ON u.user_id = ur.user_id").
		Joins("LEFT JOIN sys_dept d ON u.dept_id = d.dept_id").
		Where("r.del_flag = ?", model.DelFlagExist)

	if query.RoleID != 0 {
		db = db.Where("r.role_id = ?", query.RoleID)
	}
	if query.RoleName != "" {
		db = db.Where("r.role_name LIKE ?", "%"+query.RoleName+"%")
	}
	if query.RoleKey != "" {
		db = db.Where("r.role_key LIKE ?", "%"+query.RoleKey+"%")
	}
	if query.Status != "" {
		db = db.Where("r.status = ?", query.Status)
	}
	if scope != nil {
		db = db.Scopes(scope)
	}
	return db
}

// SelectRolePage 分页查询角色。
//
// join 出来会有重复行（一个角色对应多个用户），必须 DISTINCT；
// 统计总数也要按 role_id 去重，否则 total 会虚高。
func SelectRolePage(ctx context.Context, query model.RoleQuery, pg page.Query, scope func(*gorm.DB) *gorm.DB) ([]model.SysRole, int64, error) {
	var total int64
	if err := roleListDB(ctx, query, scope).Distinct("r.role_id").Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计角色总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysRole{}, 0, nil
	}

	// role_sort 常有并列（默认都建成同一个值），必须带主键兜底。
	// 与 SelectRoleAll 的 "role_sort, role_id" 保持一致
	orderBy := pg.Stable("r.role_sort, r.role_id", "r.role_id")

	var list []model.SysRole
	err := roleListDB(ctx, query, scope).
		Distinct("r.*").
		Order(orderBy).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询角色列表失败: %w", err)
	}
	return list, total, nil
}

// SelectRoleList 不分页查询，供导出和数据权限校验使用。
func SelectRoleList(ctx context.Context, query model.RoleQuery, scope func(*gorm.DB) *gorm.DB, limit ...int) ([]model.SysRole, error) {
	var list []model.SysRole
	err := applyOptionalLimit(roleListDB(ctx, query, scope), limit).
		Distinct("r.*").
		Order("r.role_sort").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色列表失败: %w", err)
	}
	return list, nil
}

// SelectRoleByID 按 ID 查角色，不存在返回 (nil, nil)。
//
// 过滤 del_flag，理由同 SelectDeptByID：已删除的角色不该还能被查到或授权。
func SelectRoleByID(ctx context.Context, roleID int64) (*model.SysRole, error) {
	var role model.SysRole
	err := DB(ctx).
		Where("role_id = ?", roleID).
		Where("del_flag = ?", model.DelFlagExist).
		Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询角色 %d 失败: %w", roleID, err)
	}
	return &role, nil
}

// SelectRoleAll 查全部未删除角色，供下拉选择使用。
func SelectRoleAll(ctx context.Context) ([]model.SysRole, error) {
	var list []model.SysRole
	err := DB(ctx).
		Where("del_flag = ?", model.DelFlagExist).
		// 【必须按 role_sort 排，别再当成"多余的排序"删掉】
		// Java 的 selectRoleAll() 不是一条独立 SQL，它转调 selectRoleList：
		//   SysRoleServiceImpl.java:112
		//     return SpringUtils.getAopProxy(this).selectRoleList(new SysRole());
		// 而 SysRoleMapper.xml 的 selectRoleList 结尾有 `order by r.role_sort`。
		//
		// 曾经以"mapper 里没有同名 select、所以 Java 没排序"为由删过这一行，
		// 双端对拍还通过了 —— 因为种子数据里 role_id 和 role_sort 恰好同序，
		// 样本把差异盖住了。判断 Java 有没有排序要看 service 转调到哪条 SQL。
		//
		// 追加的 role_id 比 Java 严一点：Java 只写了 role_sort，
		// 并列时行序由执行计划决定。这里让它确定下来，
		// 代价是双端对拍在有并列 role_sort 时会报差异 —— 那是预期的，
		// **不要靠去掉 role_id 来消差异**。
		Order("role_sort, role_id").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询全部角色失败: %w", err)
	}
	return list, nil
}

// CountRoleByName 同名角色数量，excludeID 用于修改时排除自身。
func CountRoleByName(ctx context.Context, roleName string, excludeID int64) (int64, error) {
	return countRole(ctx, "role_name = ?", roleName, excludeID)
}

// CountRoleByKey 同权限字符的角色数量，excludeID 用于修改时排除自身。
func CountRoleByKey(ctx context.Context, roleKey string, excludeID int64) (int64, error) {
	return countRole(ctx, "role_key = ?", roleKey, excludeID)
}

func countRole(ctx context.Context, cond string, value any, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysRole{}).
		Where(cond, value).
		Where("del_flag = ?", model.DelFlagExist)
	if excludeID > 0 {
		db = db.Where("role_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验角色唯一性失败: %w", err)
	}
	return count, nil
}

// CountUserRoleByRoleID 统计该角色下已分配的用户数，删除前检查用。
func CountUserRoleByRoleID(ctx context.Context, roleID int64) (int64, error) {
	var count int64
	err := DB(ctx).Table("sys_user_role").Where("role_id = ?", roleID).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计角色 %d 的用户数失败: %w", roleID, err)
	}
	return count, nil
}

// SelectDeptIDsByRoleID 查角色已选的部门 ID。
//
// checkStrictly 为真时排除掉"有子节点被选中"的父节点 ——
// 前端的树组件用半选状态表达父节点，父节点回显成全选会导致勾选状态错乱。
// 菜单侧的同名逻辑在 menu.go 的 SelectMenuIDsByRoleID。
func SelectDeptIDsByRoleID(ctx context.Context, roleID int64, checkStrictly bool) ([]int64, error) {
	db := DB(ctx).
		Table("sys_dept d").
		Joins("LEFT JOIN sys_role_dept rd ON d.dept_id = rd.dept_id").
		Where("rd.role_id = ?", roleID)
	if checkStrictly {
		db = db.Where(`d.dept_id NOT IN (
			SELECT d2.parent_id FROM sys_dept d2
			INNER JOIN sys_role_dept rd2 ON d2.dept_id = rd2.dept_id AND rd2.role_id = ?)`, roleID)
	}

	var ids []int64
	if err := db.Order("d.parent_id, d.order_num").Pluck("d.dept_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("查询角色 %d 的部门失败: %w", roleID, err)
	}
	return ids, nil
}

// InsertRole 新增角色并写入角色-菜单关联。
func InsertRole(ctx context.Context, role *model.SysRole) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return fmt.Errorf("新增角色失败: %w", err)
		}
		return batchRoleMenu(tx, role.RoleID, role.MenuIDs)
	})
}

// UpdateRole 更新角色并重建角色-菜单关联。
func UpdateRole(ctx context.Context, role *model.SysRole) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		updates := map[string]any{
			"role_name":           role.RoleName,
			"role_key":            role.RoleKey,
			"role_sort":           model.IntValue(role.RoleSort),
			"status":              role.Status,
			"menu_check_strictly": role.MenuCheckStrictly,
			"dept_check_strictly": role.DeptCheckStrictly,
			"update_by":           role.UpdateBy,
			"update_time":         role.UpdateTime,
		}
		if role.Remark != nil {
			updates["remark"] = *role.Remark
		}
		if err := tx.Model(&model.SysRole{}).Where("role_id = ?", role.RoleID).Updates(updates).Error; err != nil {
			return fmt.Errorf("更新角色 %d 失败: %w", role.RoleID, err)
		}

		if err := tx.Where("role_id = ?", role.RoleID).Delete(&model.SysRoleMenu{}).Error; err != nil {
			return fmt.Errorf("清除角色 %d 的菜单关联失败: %w", role.RoleID, err)
		}
		return batchRoleMenu(tx, role.RoleID, role.MenuIDs)
	})
}

// UpdateRoleStatus 只改状态。
func UpdateRoleStatus(ctx context.Context, roleID int64, status, operator string) error {
	err := DB(ctx).Model(&model.SysRole{}).
		Where("role_id = ?", roleID).
		Updates(map[string]any{"status": status, "update_by": operator}).Error
	if err != nil {
		return fmt.Errorf("更新角色 %d 状态失败: %w", roleID, err)
	}
	return nil
}

// AuthDataScope 保存数据权限：改 data_scope 并重建角色-部门关联。
func AuthDataScope(ctx context.Context, role *model.SysRole) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		updates := map[string]any{
			"data_scope":          role.DataScope,
			"dept_check_strictly": role.DeptCheckStrictly,
			"update_by":           role.UpdateBy,
			"update_time":         role.UpdateTime,
		}
		if err := tx.Model(&model.SysRole{}).Where("role_id = ?", role.RoleID).Updates(updates).Error; err != nil {
			return fmt.Errorf("更新角色 %d 数据权限失败: %w", role.RoleID, err)
		}

		if err := tx.Where("role_id = ?", role.RoleID).Delete(&model.SysRoleDept{}).Error; err != nil {
			return fmt.Errorf("清除角色 %d 的部门关联失败: %w", role.RoleID, err)
		}
		if len(role.DeptIDs) == 0 {
			return nil
		}
		rows := make([]model.SysRoleDept, 0, len(role.DeptIDs))
		for _, deptID := range role.DeptIDs {
			rows = append(rows, model.SysRoleDept{RoleID: role.RoleID, DeptID: deptID})
		}
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("写入角色 %d 的部门关联失败: %w", role.RoleID, err)
		}
		return nil
	})
}

// DeleteRoleByIDs 批量删除角色（逻辑删除）并清理关联。
func DeleteRoleByIDs(ctx context.Context, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("role_id IN ?", roleIDs).Delete(&model.SysRoleMenu{}).Error; err != nil {
			return fmt.Errorf("清除角色菜单关联失败: %w", err)
		}
		if err := tx.Where("role_id IN ?", roleIDs).Delete(&model.SysRoleDept{}).Error; err != nil {
			return fmt.Errorf("清除角色部门关联失败: %w", err)
		}
		if err := tx.Model(&model.SysRole{}).
			Where("role_id IN ?", roleIDs).
			Update("del_flag", model.DelFlagDeleted).Error; err != nil {
			return fmt.Errorf("删除角色失败: %w", err)
		}
		return nil
	})
}

func batchRoleMenu(tx *gorm.DB, roleID int64, menuIDs []int64) error {
	if len(menuIDs) == 0 {
		return nil
	}
	rows := make([]model.SysRoleMenu, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		rows = append(rows, model.SysRoleMenu{RoleID: roleID, MenuID: menuID})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("写入角色 %d 的菜单关联失败: %w", roleID, err)
	}
	return nil
}
