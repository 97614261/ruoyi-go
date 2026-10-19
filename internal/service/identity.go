package service

import (
	"context"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
)

// CurrentIdentity resolves only the workspace already bound by Auth. Role names
// are presentation data; APIs continue to enforce platform identity/permissions.
func CurrentIdentity(ctx context.Context, user *model.SysUser) (model.SessionIdentity, error) {
	if user == nil || user.TenantID < 1 || tenantctx.Get(ctx).ID != user.TenantID {
		return model.SessionIdentity{}, errs.New("租户会话无效，请重新登录")
	}
	t, err := repository.TenantByID(ctx, user.TenantID)
	if err != nil {
		return model.SessionIdentity{}, err
	}
	if err = ValidateTenantState(t); err != nil {
		return model.SessionIdentity{}, err
	}
	return buildIdentity(t, user), nil
}

func buildIdentity(t *model.SysTenant, user *model.SysUser) model.SessionIdentity {
	identity := model.SessionIdentity{
		TenantID: t.TenantID, TenantCode: t.TenantCode, TenantName: t.TenantName,
		UserName: user.UserName, RoleNames: []string{},
		PlatformAdmin: user.IsPlatform(), TenantAdmin: user.IsAdmin() && !user.IsPlatform(),
	}
	switch {
	case identity.PlatformAdmin:
		identity.RoleNames = []string{"平台超级管理员"}
	case identity.TenantAdmin:
		identity.RoleNames = []string{"企业管理员"}
	default:
		seen := map[string]bool{}
		for _, role := range user.Roles {
			name := strings.TrimSpace(role.RoleName)
			if role.Status != model.StatusNormal || role.DelFlag == model.DelFlagDeleted || name == "" || seen[name] {
				continue
			}
			identity.RoleNames = append(identity.RoleNames, name)
			seen[name] = true
		}
	}
	return identity
}
