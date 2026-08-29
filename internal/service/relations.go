package service

import (
	"context"
	"fmt"
	"sort"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

const maxRelationIDs = 200

func normalizeRelationIDs(ids []int64, label string) ([]int64, error) {
	if len(ids) > maxRelationIDs {
		return nil, errs.Newf("%s一次最多选择%d项", label, maxRelationIDs)
	}
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errs.Newf("%s包含无效ID", label)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func ensureAllIDs(requested, found []int64, label string) error {
	if len(requested) == len(found) {
		return nil
	}
	foundSet := make(map[int64]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := foundSet[id]; !ok {
			return errs.New(fmt.Sprintf("%sID %d 不存在或无权访问", label, id))
		}
	}
	return errs.Newf("%s包含不存在或无权访问的数据", label)
}

func checkUserIDs(ctx context.Context, operator *model.SysUser, ids []int64) ([]int64, error) {
	normalized, err := normalizeRelationIDs(ids, "用户")
	if err != nil || len(normalized) == 0 {
		return normalized, err
	}
	var scope = userScope(operator)
	if operator == nil || operator.IsAdmin() {
		scope = nil
	}
	found, err := repository.SelectExistingUserIDs(ctx, normalized, scope)
	if err != nil {
		return nil, err
	}
	return normalized, ensureAllIDs(normalized, found, "用户")
}

func checkRoleIDs(ctx context.Context, operator *model.SysUser, ids []int64) ([]int64, error) {
	normalized, err := normalizeRelationIDs(ids, "角色")
	if err != nil || len(normalized) == 0 {
		return normalized, err
	}
	var scope = roleScope(operator)
	if operator == nil || operator.IsAdmin() {
		scope = nil
	}
	found, err := repository.SelectExistingRoleIDs(ctx, normalized, scope)
	if err != nil {
		return nil, err
	}
	return normalized, ensureAllIDs(normalized, found, "角色")
}

func checkDeptIDs(ctx context.Context, operator *model.SysUser, ids []int64) ([]int64, error) {
	normalized, err := normalizeRelationIDs(ids, "部门")
	if err != nil || len(normalized) == 0 {
		return normalized, err
	}
	var scope = deptScope(operator, PermDeptList)
	if operator == nil || operator.IsAdmin() {
		scope = nil
	}
	found, err := repository.SelectExistingDeptIDs(ctx, normalized, scope)
	if err != nil {
		return nil, err
	}
	return normalized, ensureAllIDs(normalized, found, "部门")
}

func checkMenuIDs(ctx context.Context, ids []int64) ([]int64, error) {
	normalized, err := normalizeRelationIDs(ids, "菜单")
	if err != nil || len(normalized) == 0 {
		return normalized, err
	}
	found, err := repository.SelectExistingMenuIDs(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return normalized, ensureAllIDs(normalized, found, "菜单")
}

func checkPostIDs(ctx context.Context, ids []int64) ([]int64, error) {
	normalized, err := normalizeRelationIDs(ids, "岗位")
	if err != nil || len(normalized) == 0 {
		return normalized, err
	}
	found, err := repository.SelectExistingPostIDs(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return normalized, ensureAllIDs(normalized, found, "岗位")
}
