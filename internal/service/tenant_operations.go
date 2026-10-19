package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
)

type TenantLifecycleInput struct {
	Version           int64  `json:"version" binding:"gt=0"`
	ConfirmTenantCode string `json:"confirmTenantCode" binding:"required,max=32"`
	Action            string `json:"action" binding:"required"`
	Reason            string `json:"reason" binding:"notblank,xss,max=200"`
}
type TenantQuotaInput struct {
	Version           int64  `json:"version" binding:"gt=0"`
	ConfirmTenantCode string `json:"confirmTenantCode" binding:"required,max=32"`
	MaxUsers          *int64 `json:"maxUsers"`
	MaxStorageBytes   *int64 `json:"maxStorageBytes"`
	MaxUploadBytes    *int64 `json:"maxUploadBytes"`
	Reason            string `json:"reason" binding:"notblank,xss,max=200"`
}

func TenantDetail(ctx context.Context, operator *model.SysUser, id int64) (*model.SysTenant, error) {
	if _, err := platformActor(ctx, operator, ""); err != nil {
		return nil, err
	}
	t, err := repository.TenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil || t.TenantKind != "01" {
		return nil, errs.New("企业不存在或不允许操作平台空间")
	}
	return t, nil
}
func TenantQuota(ctx context.Context, operator *model.SysUser, id int64) (model.TenantQuotaSettings, error) {
	if _, err := platformActor(ctx, operator, ""); err != nil {
		return model.TenantQuotaSettings{}, err
	}
	return repository.CustomerQuotaSettings(ctx, id)
}
func OwnWorkspace(ctx context.Context, user *model.SysUser) (model.TenantQuotaSettings, error) {
	s := tenantctx.Get(ctx)
	if user == nil || s.ID < 1 || s.Platform || user.TenantID != s.ID || user.IsPlatform() {
		return model.TenantQuotaSettings{}, errs.New("仅企业账号可访问本企业信息")
	}
	return repository.CustomerQuotaSettings(ctx, s.ID)
}

func ChangeTenantLifecycle(ctx context.Context, user *model.SysUser, id int64, body TenantLifecycleInput, ip string) error {
	if user == nil || user.TenantID != tenantctx.ID(ctx) || body.Version < 1 || body.ConfirmTenantCode == "" || strings.TrimSpace(body.Reason) == "" || utf8.RuneCountInString(body.Reason) > 200 {
		return errs.New("生命周期信息或操作身份不完整")
	}
	if tenantctx.Get(ctx).Platform {
		if _, err := platformActor(ctx, user, ip); err != nil {
			return err
		}
	} else if !user.IsAdmin() || user.IsPlatform() || body.Action != "requestCancellation" || id != user.TenantID {
		return errs.New("仅企业所有者可申请注销")
	}
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	roleWriteMu.Lock()
	defer roleWriteMu.Unlock()
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	actor := model.PlatformActor{TenantID: user.TenantID, UserID: user.UserID, UserName: user.UserName, IP: ip}
	return repository.ChangeCustomerLifecycle(ctx, id, body.Version, body.ConfirmTenantCode, body.Action, strings.TrimSpace(body.Reason), actor)
}

func SaveTenantQuota(ctx context.Context, user *model.SysUser, id int64, body TenantQuotaInput, ip string) error {
	actor, err := platformActor(ctx, user, ip)
	if err != nil {
		return err
	}
	if err = validateTenantQuota(body); err != nil {
		return err
	}
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	p := model.TenantPolicy{MaxUsers: *body.MaxUsers, MaxStorageBytes: *body.MaxStorageBytes, MaxUploadBytes: *body.MaxUploadBytes}
	return repository.UpdateCustomerQuota(ctx, id, body.Version, body.ConfirmTenantCode, strings.TrimSpace(body.Reason), p, actor)
}
func validateTenantQuota(body TenantQuotaInput) error {
	if body.Version < 1 || body.ConfirmTenantCode == "" || body.MaxUsers == nil || body.MaxStorageBytes == nil || body.MaxUploadBytes == nil || strings.TrimSpace(body.Reason) == "" || utf8.RuneCountInString(body.Reason) > 200 {
		return errs.New("请完整填写版本、配额、企业确认及原因")
	}
	if *body.MaxUsers < 0 || *body.MaxUsers > 100000 || *body.MaxStorageBytes < 0 || *body.MaxStorageBytes > 1<<40 || *body.MaxUploadBytes < 0 || *body.MaxUploadBytes > 1<<30 {
		return errs.New("配额超出允许范围；0表示不限（单文件仍受服务器限制）")
	}
	return nil
}
