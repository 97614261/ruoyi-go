package service

import (
	"context"

	"gorm.io/gorm"

	"ruoyi-go/internal/datascope"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// PermRoleList 角色列表的权限标识，数据权限按此过滤角色。
const PermRoleList = "system:role:list"

// roleScope 角色查询的数据权限过滤器。
//
// 注意只有 deptAlias 没有 userAlias —— 与 Java 版
// @DataScope(deptAlias = "d") 一致。所以"仅本人"数据范围在角色列表上
// 会退化成查不到数据，这是 Java 的既定行为，不要"修正"。
func roleScope(user *model.SysUser) func(*gorm.DB) *gorm.DB {
	return datascope.Scope(datascope.Params{
		User:       user,
		DeptAlias:  "d",
		Permission: PermRoleList,
	})
}

// ListRolePage 分页查询角色。
func ListRolePage(ctx context.Context, user *model.SysUser, query model.RoleQuery, pg page.Query) ([]model.SysRole, int64, error) {
	return repository.SelectRolePage(ctx, query, pg, roleScope(user))
}

// ListRoleExport 导出用的全量查询。
func ListRoleExport(ctx context.Context, user *model.SysUser, query model.RoleQuery) ([]model.SysRole, error) {
	list, err := repository.SelectRoleList(ctx, query, roleScope(user), MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// ListRoleAll 查全部角色，供下拉选择。
func ListRoleAll(ctx context.Context) ([]model.SysRole, error) {
	return repository.SelectRoleAll(ctx)
}

// GetRole 查角色详情，带数据权限校验。
func GetRole(ctx context.Context, user *model.SysUser, roleID int64) (*model.SysRole, error) {
	if err := CheckRoleDataScope(ctx, user, roleID); err != nil {
		return nil, err
	}
	role, err := repository.SelectRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errs.New("角色不存在")
	}
	return role, nil
}

// CheckRoleAllowed 超级管理员角色不允许被修改或删除。
func CheckRoleAllowed(roleID int64) error {
	if roleID == model.AdminRoleID {
		return errs.New("不允许操作超级管理员角色")
	}
	return nil
}

// CheckRoleDataScope 校验当前用户是否有权操作该角色。
func CheckRoleDataScope(ctx context.Context, user *model.SysUser, roleID int64) error {
	if user == nil || user.IsAdmin() || roleID == 0 {
		return nil
	}
	list, err := repository.SelectRoleList(ctx, model.RoleQuery{RoleID: roleID}, roleScope(user))
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errs.New("没有权限访问角色数据！")
	}
	return nil
}

// CreateRole 新增角色。
func CreateRole(ctx context.Context, user *model.SysUser, role *model.SysRole, operator string) error {
	menuIDs, err := checkMenuIDsForUser(ctx, user, role.MenuIDs)
	if err != nil {
		return err
	}
	role.MenuIDs = menuIDs
	if err := checkRoleUnique(ctx, role, "新增"); err != nil {
		return err
	}
	role.RoleID = 0
	role.DelFlag = model.DelFlagExist
	if role.DataScope == "" {
		role.DataScope = model.DataScopeAll
	}
	role.CreateBy = operator
	role.CreateTime = types.Now()
	return repository.InsertRole(ctx, role)
}

// UpdateRole 修改角色。
func UpdateRole(ctx context.Context, user *model.SysUser, role *model.SysRole, operator string) error {
	if role.RoleID == 0 {
		return errs.New("角色ID不能为空")
	}
	if err := CheckRoleAllowed(role.RoleID); err != nil {
		return err
	}
	if err := CheckRoleDataScope(ctx, user, role.RoleID); err != nil {
		return err
	}
	menuIDs, err := checkMenuIDsForUser(ctx, user, role.MenuIDs)
	if err != nil {
		return err
	}
	role.MenuIDs = menuIDs
	if err := checkRoleUnique(ctx, role, "修改"); err != nil {
		return err
	}

	existing, err := repository.SelectRoleByID(ctx, role.RoleID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("角色不存在")
	}

	role.UpdateBy = operator
	role.UpdateTime = types.Now()
	if err := repository.UpdateRole(ctx, role); err != nil {
		return err
	}
	// 权限变了，在线用户的会话要跟着刷新，否则要等下次登录才生效
	return RefreshOnlineUsersByRole(ctx, role.RoleID)
}

// ChangeRoleStatus 启用/停用角色。
func ChangeRoleStatus(ctx context.Context, user *model.SysUser, roleID int64, status, operator string) error {
	if err := CheckRoleAllowed(roleID); err != nil {
		return err
	}
	if err := CheckRoleDataScope(ctx, user, roleID); err != nil {
		return err
	}
	if status == "" {
		return errs.New("状态不能为空")
	}
	if err := repository.UpdateRoleStatus(ctx, roleID, status, operator); err != nil {
		return err
	}
	return RefreshOnlineUsersByRole(ctx, roleID)
}

// AuthDataScope 保存角色的数据权限配置。
func AuthDataScope(ctx context.Context, user *model.SysUser, body model.RoleDataScopeBody, operator string) error {
	if err := CheckRoleAllowed(body.RoleID); err != nil {
		return err
	}
	if err := CheckRoleDataScope(ctx, user, body.RoleID); err != nil {
		return err
	}
	deptIDs, err := checkDeptIDs(ctx, user, body.DeptIDs)
	if err != nil {
		return err
	}
	body.DeptIDs = deptIDs

	role := &model.SysRole{
		RoleID:            body.RoleID,
		DataScope:         body.DataScope,
		DeptCheckStrictly: body.DeptCheckStrictly,
		DeptIDs:           body.DeptIDs,
		UpdateBy:          operator,
		UpdateTime:        types.Now(),
	}
	if err := repository.AuthDataScope(ctx, role); err != nil {
		return err
	}
	return RefreshOnlineUsersByRole(ctx, body.RoleID)
}

// DeleteRoles 批量删除角色。
func DeleteRoles(ctx context.Context, user *model.SysUser, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return errs.New("请选择要删除的角色")
	}
	var err error
	roleIDs, err = checkRoleIDs(ctx, user, roleIDs)
	if err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := CheckRoleAllowed(roleID); err != nil {
			return err
		}
	}
	roles, counts, err := repository.SelectRolesForDelete(ctx, roleIDs)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if counts[role.RoleID] > 0 {
			return errs.Newf("%s已分配,不能删除", role.RoleName)
		}
	}
	return repository.DeleteRoleByIDs(ctx, roleIDs)
}

// ListAuthUserPage 查角色的已分配/未分配用户。
func ListAuthUserPage(ctx context.Context, operator *model.SysUser, roleID int64,
	query model.UserQuery, pg page.Query, allocated bool) ([]model.SysUser, int64, error) {
	if err := CheckRoleDataScope(ctx, operator, roleID); err != nil {
		return nil, 0, err
	}
	return repository.SelectAuthUserPage(ctx, roleID, query, pg, allocated, userScope(operator))
}

// CancelAuthUser 取消若干用户的角色授权。
func CancelAuthUser(ctx context.Context, operator *model.SysUser, roleID int64, userIDs []int64) error {
	if err := CheckRoleAllowed(roleID); err != nil {
		return err
	}
	if err := CheckRoleDataScope(ctx, operator, roleID); err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return errs.New("请选择要取消授权的用户")
	}
	var err error
	userIDs, err = checkUserIDs(ctx, operator, userIDs)
	if err != nil {
		return err
	}
	if err := repository.DeleteUserRole(ctx, roleID, userIDs); err != nil {
		return err
	}
	return refreshUsers(ctx, userIDs)
}

// GrantAuthUser 批量授予角色。
func GrantAuthUser(ctx context.Context, operator *model.SysUser, roleID int64, userIDs []int64) error {
	if err := CheckRoleAllowed(roleID); err != nil {
		return err
	}
	if err := CheckRoleDataScope(ctx, operator, roleID); err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return errs.New("请选择要授权的用户")
	}
	var err error
	userIDs, err = checkUserIDs(ctx, operator, userIDs)
	if err != nil {
		return err
	}
	if err := repository.InsertUserRole(ctx, roleID, userIDs); err != nil {
		return err
	}
	return refreshUsers(ctx, userIDs)
}

// refreshUsers 按用户会话索引批量刷新在线会话。
func refreshUsers(ctx context.Context, userIDs []int64) error {
	return RefreshOnlineUsersByID(ctx, userIDs...)
}

// RoleDeptTree 返回部门树和该角色已选中的部门 ID。
//
// 对应 /system/role/deptTree/{roleId}，响应是平铺的 depts + checkedKeys。
func RoleDeptTree(ctx context.Context, user *model.SysUser, roleID int64) ([]model.TreeSelect, []int64, error) {
	if err := CheckRoleDataScope(ctx, user, roleID); err != nil {
		return nil, nil, err
	}
	role, err := repository.SelectRoleByID(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}
	checkStrictly := role != nil && role.DeptCheckStrictly

	checkedKeys, err := repository.SelectDeptIDsByRoleID(ctx, roleID, checkStrictly)
	if err != nil {
		return nil, nil, err
	}

	depts, err := ListDepts(ctx, user, model.DeptQuery{})
	if err != nil {
		return nil, nil, err
	}
	visible := make(map[int64]struct{}, len(depts))
	for _, dept := range depts {
		visible[dept.DeptID] = struct{}{}
	}
	filteredKeys := make([]int64, 0, len(checkedKeys))
	for _, id := range checkedKeys {
		if _, ok := visible[id]; ok {
			filteredKeys = append(filteredKeys, id)
		}
	}
	return BuildDeptTreeSelect(depts), filteredKeys, nil
}

// BuildDeptTreeSelect 把部门列表组装成前端树选择组件要的结构。
//
// children 为空时不输出该键（不是空数组），与 Java 一致 —— 前端据此判断叶子节点。
func BuildDeptTreeSelect(depts []model.SysDept) []model.TreeSelect {
	byParent := make(map[int64][]model.SysDept, len(depts))
	exists := make(map[int64]bool, len(depts))
	for _, dept := range depts {
		byParent[dept.ParentID] = append(byParent[dept.ParentID], dept)
		exists[dept.DeptID] = true
	}

	visited := make(map[int64]bool, len(depts))
	var build func(parentID int64) []model.TreeSelect
	build = func(parentID int64) []model.TreeSelect {
		children := byParent[parentID]
		if len(children) == 0 {
			return nil
		}
		nodes := make([]model.TreeSelect, 0, len(children))
		for _, dept := range children {
			if visited[dept.DeptID] {
				continue
			}
			visited[dept.DeptID] = true
			nodes = append(nodes, model.TreeSelect{
				ID:       dept.DeptID,
				Label:    dept.DeptName,
				Disabled: dept.Status == model.DeptDisable,
				Children: build(dept.DeptID),
			})
		}
		return nodes
	}

	// 父节点不在结果集里的（被数据权限过滤掉了）当作根节点，避免整棵树丢失
	roots := make([]model.TreeSelect, 0)
	for _, dept := range depts {
		if exists[dept.ParentID] || visited[dept.DeptID] {
			continue
		}
		visited[dept.DeptID] = true
		roots = append(roots, model.TreeSelect{
			ID:       dept.DeptID,
			Label:    dept.DeptName,
			Disabled: dept.Status == model.DeptDisable,
			Children: build(dept.DeptID),
		})
	}
	return roots
}

// checkRoleUnique 校验角色名称和权限字符唯一。
func checkRoleUnique(ctx context.Context, role *model.SysRole, action string) error {
	nameCount, err := repository.CountRoleByName(ctx, role.RoleName, role.RoleID)
	if err != nil {
		return err
	}
	if nameCount > 0 {
		return errs.Newf("%s角色'%s'失败，角色名称已存在", action, role.RoleName)
	}

	keyCount, err := repository.CountRoleByKey(ctx, role.RoleKey, role.RoleID)
	if err != nil {
		return err
	}
	if keyCount > 0 {
		return errs.Newf("%s角色'%s'失败，角色权限已存在", action, role.RoleName)
	}
	return nil
}
