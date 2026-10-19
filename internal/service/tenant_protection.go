package service

import (
	"context"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

func checkReservedTenantAdminRole(ctx context.Context, userID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := repository.ProtectedRoleCount(ctx, ids)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if userID > 0 {
		u, err := repository.SelectUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if u != nil && u.IsAdmin() {
			return nil
		}
	}
	return errs.New("管理员角色只能通过企业所有者交接流程授予")
}
