package repository

import (
	"context"
	"ruoyi-go/internal/model"
)

func HasProtectedUsers(ctx context.Context, ids []int64) (bool, error) {
	var n int64
	err := DB(ctx).Model(&model.SysUser{}).Where("user_id IN ? AND user_type IN ?", ids, []string{"01", "02"}).Count(&n).Error
	return n > 0, err
}
func ProtectedRoleCount(ctx context.Context, ids []int64) (int64, error) {
	var n int64
	err := DB(ctx).Model(&model.SysRole{}).Where("role_id IN ? AND role_key IN ?", ids, []string{"tenant_admin", "platform_admin"}).Count(&n).Error
	return n, err
}
