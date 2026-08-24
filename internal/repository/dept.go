package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
)

// DeptTableAlias 部门表在列表查询里的别名，数据权限拼条件时要用同一个。
const DeptTableAlias = "d"

// SelectDeptList 查部门列表。
//
// scope 是数据权限过滤器，由 service 层用 datascope.Scope 构造后传入；
// 传 nil 表示不过滤（仅限确实不需要数据权限的场景）。
func SelectDeptList(ctx context.Context, query model.DeptQuery, scope func(*gorm.DB) *gorm.DB) ([]model.SysDept, error) {
	db := DB(ctx).
		Table("sys_dept "+DeptTableAlias).
		Select(DeptTableAlias+".*").
		Where(DeptTableAlias+".del_flag = ?", model.DelFlagExist)

	if query.DeptID != 0 {
		db = db.Where(DeptTableAlias+".dept_id = ?", query.DeptID)
	}
	if query.ParentID != 0 {
		db = db.Where(DeptTableAlias+".parent_id = ?", query.ParentID)
	}
	if query.DeptName != "" {
		db = db.Where(DeptTableAlias+".dept_name LIKE ?", "%"+query.DeptName+"%")
	}
	if query.Status != "" {
		db = db.Where(DeptTableAlias+".status = ?", query.Status)
	}
	if scope != nil {
		db = db.Scopes(scope)
	}

	var list []model.SysDept
	if err := db.Order(DeptTableAlias + ".parent_id, " + DeptTableAlias + ".order_num").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询部门列表失败: %w", err)
	}
	return list, nil
}

// SelectDeptByID 按 ID 查部门，附带上级部门名称。
//
// 【过滤 del_flag，这是对 Java 的刻意偏离】
// Java 的 selectDeptById 不过滤逻辑删除标记，导致已删除的部门仍能按 ID 查到 ——
// 后果不只是"查得到"：CreateDept 用它查父部门，于是可以往已删除的部门下面
// 建子部门，建出来的分支在列表里根本看不见。前端从不请求已删除的部门，
// 所以加这个过滤对前端完全无感。
func SelectDeptByID(ctx context.Context, deptID int64) (*model.SysDept, error) {
	var dept model.SysDept
	err := DB(ctx).
		Where("dept_id = ?", deptID).
		Where("del_flag = ?", model.DelFlagExist).
		Take(&dept).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询部门 %d 失败: %w", deptID, err)
	}

	if dept.ParentID != 0 {
		var parentName string
		names := []string{}
		if err := DB(ctx).Table("sys_dept").
			Where("dept_id = ?", dept.ParentID).
			Limit(1).
			Pluck("dept_name", &names).Error; err == nil && len(names) > 0 {
			parentName = names[0]
		}
		dept.ParentName = parentName
	}
	return &dept, nil
}

// SelectChildrenDeptByID 查所有子孙部门（不含自身）。
func SelectChildrenDeptByID(ctx context.Context, deptID int64) ([]model.SysDept, error) {
	var list []model.SysDept
	err := DB(ctx).Where("find_in_set(?, ancestors)", deptID).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询部门 %d 的子部门失败: %w", deptID, err)
	}
	return list, nil
}

// CountNormalChildrenDeptByID 统计未停用的子孙部门数量。
func CountNormalChildrenDeptByID(ctx context.Context, deptID int64) (int64, error) {
	var count int64
	err := DB(ctx).Model(&model.SysDept{}).
		Where("status = ?", model.DeptNormal).
		Where("del_flag = ?", model.DelFlagExist).
		Where("find_in_set(?, ancestors)", deptID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计部门 %d 的正常子部门失败: %w", deptID, err)
	}
	return count, nil
}

// HasChildByDeptID 是否存在下级部门。
func HasChildByDeptID(ctx context.Context, deptID int64) (bool, error) {
	var count int64
	err := DB(ctx).Model(&model.SysDept{}).
		Where("del_flag = ?", model.DelFlagExist).
		Where("parent_id = ?", deptID).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查部门 %d 是否有下级失败: %w", deptID, err)
	}
	return count > 0, nil
}

// CheckDeptExistUser 部门下是否还有未删除的用户。
func CheckDeptExistUser(ctx context.Context, deptID int64) (bool, error) {
	var count int64
	err := DB(ctx).Table("sys_user").
		Where("dept_id = ?", deptID).
		Where("del_flag = ?", model.DelFlagExist).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查部门 %d 是否有用户失败: %w", deptID, err)
	}
	return count > 0, nil
}

// CountDeptByNameAndParent 同一父部门下的同名部门数量，excludeID 用于修改时排除自身。
func CountDeptByNameAndParent(ctx context.Context, deptName string, parentID, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysDept{}).
		Where("dept_name = ?", deptName).
		Where("parent_id = ?", parentID).
		Where("del_flag = ?", model.DelFlagExist)
	if excludeID > 0 {
		db = db.Where("dept_id <> ?", excludeID)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验部门名称唯一性失败: %w", err)
	}
	return count, nil
}

// InsertDept 新增部门。
func InsertDept(ctx context.Context, dept *model.SysDept) error {
	if err := DB(ctx).Create(dept).Error; err != nil {
		return fmt.Errorf("新增部门失败: %w", err)
	}
	return nil
}

// UpdateDept 更新部门，同时改子部门的 ancestors、按需启用上级部门。
//
// 三步必须在同一事务里：ancestors 是冗余字段，改了自己不改子孙，
// 整棵树的数据权限判断就全乱了。
func UpdateDept(ctx context.Context, dept *model.SysDept, children []model.SysDept, enableParentIDs []int64) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		updates := map[string]any{
			"parent_id":   dept.ParentID,
			"ancestors":   dept.Ancestors,
			"dept_name":   dept.DeptName,
			"order_num":   model.IntValue(dept.OrderNum),
			"leader":      dept.Leader,
			"phone":       dept.Phone,
			"email":       dept.Email,
			"status":      dept.Status,
			"update_by":   dept.UpdateBy,
			"update_time": dept.UpdateTime,
		}
		if err := tx.Model(&model.SysDept{}).Where("dept_id = ?", dept.DeptID).Updates(updates).Error; err != nil {
			return fmt.Errorf("更新部门 %d 失败: %w", dept.DeptID, err)
		}

		for i := range children {
			child := children[i]
			if err := tx.Model(&model.SysDept{}).
				Where("dept_id = ?", child.DeptID).
				Update("ancestors", child.Ancestors).Error; err != nil {
				return fmt.Errorf("更新子部门 %d 的祖级路径失败: %w", child.DeptID, err)
			}
		}

		if len(enableParentIDs) > 0 {
			if err := tx.Model(&model.SysDept{}).
				Where("dept_id IN ?", enableParentIDs).
				Update("status", model.DeptNormal).Error; err != nil {
				return fmt.Errorf("启用上级部门失败: %w", err)
			}
		}
		return nil
	})
}

// DeleteDeptByID 删除部门（逻辑删除，与 Java 一致）。
func DeleteDeptByID(ctx context.Context, deptID int64) error {
	err := DB(ctx).Model(&model.SysDept{}).
		Where("dept_id = ?", deptID).
		Update("del_flag", model.DelFlagDeleted).Error
	if err != nil {
		return fmt.Errorf("删除部门 %d 失败: %w", deptID, err)
	}
	return nil
}

// UpdateDeptSort 批量保存部门排序。
func UpdateDeptSort(ctx context.Context, sorts map[int64]int) error {
	if len(sorts) == 0 {
		return nil
	}
	return Transaction(ctx, func(tx *gorm.DB) error {
		for deptID, orderNum := range sorts {
			if err := tx.Model(&model.SysDept{}).
				Where("dept_id = ?", deptID).
				Update("order_num", orderNum).Error; err != nil {
				return fmt.Errorf("更新部门 %d 排序失败: %w", deptID, err)
			}
		}
		return nil
	})
}
