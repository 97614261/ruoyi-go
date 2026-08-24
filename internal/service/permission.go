package service

import (
	"context"
	"sort"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
)

// GetRolePermission 取用户的角色标识集合。
//
// 超级管理员固定返回 ["admin"]。普通用户取 role_key，
// role_key 支持逗号分隔多个标识（Java 版 split(",") 的行为）。
func GetRolePermission(user *model.SysUser) []string {
	if user.IsAdmin() {
		return []string{model.AdminRoleKey}
	}
	set := make(map[string]struct{})
	for _, role := range user.Roles {
		addSplit(set, role.RoleKey)
	}
	return sortedKeys(set)
}

// GetMenuPermission 取用户的权限标识集合，并按角色回填 role.Permissions。
//
// 对应 Java 版 SysPermissionService.getMenuPermission，注意两点：
//  1. 有角色时按“每个启用角色”分别查权限，顺便把结果塞回 role.Permissions，
//     数据权限过滤要用；停用角色直接跳过。
//  2. 没有任何角色时才退回按用户维度查一次。
//
// 会修改 user.Roles 里的元素，所以传指针。
func GetMenuPermission(ctx context.Context, user *model.SysUser) ([]string, error) {
	if user.IsAdmin() {
		return []string{model.AllPermission}, nil
	}

	set := make(map[string]struct{})

	if len(user.Roles) == 0 {
		perms, err := repository.SelectMenuPermsByUserID(ctx, user.UserID)
		if err != nil {
			return nil, err
		}
		for _, p := range perms {
			addSplit(set, p)
		}
		return sortedKeys(set), nil
	}

	for i := range user.Roles {
		role := &user.Roles[i]
		if role.Status != model.StatusNormal {
			continue
		}
		perms, err := repository.SelectMenuPermsByRoleID(ctx, role.RoleID)
		if err != nil {
			return nil, err
		}
		roleSet := make(map[string]struct{})
		for _, p := range perms {
			addSplit(roleSet, p)
			addSplit(set, p)
		}
		role.Permissions = sortedKeys(roleSet)
	}
	return sortedKeys(set), nil
}

// addSplit 按逗号拆分后去空白写入集合，空串丢弃。
func addSplit(set map[string]struct{}, raw string) {
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			set[part] = struct{}{}
		}
	}
}

// sortedKeys 输出稳定顺序，避免每次登录返回的权限数组顺序抖动
// （Java 用 HashSet 顺序本就不稳定，这里比它更确定一些）。
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
