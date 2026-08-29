package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
)

// SelectExistingUserIDs returns non-deleted user IDs, optionally constrained by data scope.
func SelectExistingUserIDs(ctx context.Context, userIDs []int64, scope func(*gorm.DB) *gorm.DB) ([]int64, error) {
	if len(userIDs) == 0 {
		return []int64{}, nil
	}
	db := DB(ctx).Table("sys_user u").Where("u.user_id IN ?", userIDs).
		Where("u.del_flag = ?", model.DelFlagExist)
	if scope != nil {
		db = db.Scopes(scope)
	}
	var ids []int64
	if err := db.Distinct().Pluck("u.user_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("批量校验用户失败: %w", err)
	}
	return ids, nil
}

// SelectExistingRoleIDs returns non-deleted role IDs, optionally constrained by data scope.
func SelectExistingRoleIDs(ctx context.Context, roleIDs []int64, scope func(*gorm.DB) *gorm.DB) ([]int64, error) {
	if len(roleIDs) == 0 {
		return []int64{}, nil
	}
	db := DB(ctx).Table("sys_role r").Where("r.role_id IN ?", roleIDs).
		Where("r.del_flag = ?", model.DelFlagExist)
	if scope != nil {
		db = db.Scopes(scope)
	}
	var ids []int64
	if err := db.Distinct().Pluck("r.role_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("批量校验角色失败: %w", err)
	}
	return ids, nil
}

// SelectExistingDeptIDs returns non-deleted department IDs, optionally constrained by data scope.
func SelectExistingDeptIDs(ctx context.Context, deptIDs []int64, scope func(*gorm.DB) *gorm.DB) ([]int64, error) {
	if len(deptIDs) == 0 {
		return []int64{}, nil
	}
	db := DB(ctx).Table("sys_dept d").Where("d.dept_id IN ?", deptIDs).
		Where("d.del_flag = ?", model.DelFlagExist)
	if scope != nil {
		db = db.Scopes(scope)
	}
	var ids []int64
	if err := db.Distinct().Pluck("d.dept_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("批量校验部门失败: %w", err)
	}
	return ids, nil
}

func SelectExistingMenuIDs(ctx context.Context, menuIDs []int64) ([]int64, error) {
	return selectExistingIDs(ctx, "sys_menu", "menu_id", "", menuIDs)
}

func SelectExistingPostIDs(ctx context.Context, postIDs []int64) ([]int64, error) {
	return selectExistingIDs(ctx, "sys_post", "post_id", "", postIDs)
}

func selectExistingIDs(ctx context.Context, table, column, extraWhere string, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	db := DB(ctx).Table(table).Where(column+" IN ?", ids)
	if extraWhere != "" {
		db = db.Where(extraWhere)
	}
	var found []int64
	if err := db.Distinct().Pluck(column, &found).Error; err != nil {
		return nil, fmt.Errorf("批量校验关联数据失败: %w", err)
	}
	return found, nil
}

// SelectRolesForDelete loads role names and assignment counts in one query.
func SelectRolesForDelete(ctx context.Context, roleIDs []int64) ([]model.SysRole, map[int64]int64, error) {
	var roles []model.SysRole
	if err := DB(ctx).Where("role_id IN ?", roleIDs).Find(&roles).Error; err != nil {
		return nil, nil, fmt.Errorf("批量查询角色失败: %w", err)
	}
	type row struct {
		RoleID int64 `gorm:"column:role_id"`
		Count  int64 `gorm:"column:cnt"`
	}
	var rows []row
	if err := DB(ctx).Table("sys_user_role").Select("role_id, COUNT(*) AS cnt").
		Where("role_id IN ?", roleIDs).Group("role_id").Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("批量统计角色用户失败: %w", err)
	}
	counts := make(map[int64]int64, len(rows))
	for _, item := range rows {
		counts[item.RoleID] = item.Count
	}
	return roles, counts, nil
}

func SelectPostsForDelete(ctx context.Context, postIDs []int64) ([]model.SysPost, map[int64]int64, error) {
	var posts []model.SysPost
	if err := DB(ctx).Where("post_id IN ?", postIDs).Find(&posts).Error; err != nil {
		return nil, nil, fmt.Errorf("批量查询岗位失败: %w", err)
	}
	type row struct {
		PostID int64 `gorm:"column:post_id"`
		Count  int64 `gorm:"column:cnt"`
	}
	var rows []row
	if err := DB(ctx).Table("sys_user_post").Select("post_id, COUNT(*) AS cnt").
		Where("post_id IN ?", postIDs).Group("post_id").Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("批量统计岗位用户失败: %w", err)
	}
	counts := make(map[int64]int64, len(rows))
	for _, item := range rows {
		counts[item.PostID] = item.Count
	}
	return posts, counts, nil
}

func SelectUsersByNames(ctx context.Context, names []string) (map[string]model.SysUser, error) {
	result := make(map[string]model.SysUser, len(names))
	if len(names) == 0 {
		return result, nil
	}
	var users []model.SysUser
	if err := DB(ctx).Where("user_name IN ?", names).Where("del_flag = ?", model.DelFlagExist).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("批量查询导入用户失败: %w", err)
	}
	for _, user := range users {
		result[user.UserName] = user
	}
	return result, nil
}

func SelectDictTypesForDelete(ctx context.Context, dictIDs []int64) ([]model.SysDictType, map[string]int64, error) {
	var types []model.SysDictType
	if err := DB(ctx).Where("dict_id IN ?", dictIDs).Find(&types).Error; err != nil {
		return nil, nil, fmt.Errorf("批量查询字典类型失败: %w", err)
	}
	type row struct {
		DictType string `gorm:"column:dict_type"`
		Count    int64  `gorm:"column:cnt"`
	}
	names := make([]string, 0, len(types))
	for _, item := range types {
		names = append(names, item.DictType)
	}
	var rows []row
	if len(names) > 0 {
		if err := DB(ctx).Table("sys_dict_data").Select("dict_type, COUNT(*) AS cnt").
			Where("dict_type IN ?", names).Group("dict_type").Scan(&rows).Error; err != nil {
			return nil, nil, fmt.Errorf("批量统计字典数据失败: %w", err)
		}
	}
	counts := make(map[string]int64, len(rows))
	for _, item := range rows {
		counts[item.DictType] = item.Count
	}
	return types, counts, nil
}

func SelectDictDataByCodes(ctx context.Context, codes []int64) ([]model.SysDictData, error) {
	var data []model.SysDictData
	if err := DB(ctx).Where("dict_code IN ?", codes).Find(&data).Error; err != nil {
		return nil, fmt.Errorf("批量查询字典数据失败: %w", err)
	}
	return data, nil
}

func SelectConfigsByIDs(ctx context.Context, ids []int64) ([]model.SysConfig, error) {
	var configs []model.SysConfig
	if err := DB(ctx).Where("config_id IN ?", ids).Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("批量查询参数失败: %w", err)
	}
	return configs, nil
}
