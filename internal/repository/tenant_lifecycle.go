package repository

import (
	"context"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func lifecycleNextState(status, action string) (string, bool) {
	switch action {
	case "requestCancellation":
		if status == model.TenantActive || status == model.TenantDisabled {
			return model.TenantCancellationPending, true
		}
	case "archive":
		if status == model.TenantCancellationPending {
			return model.TenantArchived, true
		}
	case "restore":
		if status == model.TenantArchived {
			return model.TenantDisabled, true
		}
	case "rejectCancellation":
		if status == model.TenantCancellationPending {
			return model.TenantDisabled, true
		}
	}
	return "", false
}

func customerMutable(t *model.SysTenant) error {
	if t.Status != model.TenantActive && t.Status != model.TenantDisabled {
		return errs.New("企业待注销或已归档，请先通过生命周期流程恢复")
	}
	return nil
}

func ChangeCustomerLifecycle(ctx context.Context, id, version int64, code, action, reason string, actor model.PlatformActor) error {
	scope := tenantctx.Get(ctx)
	if scope.ID < 1 || actor.TenantID != scope.ID || actor.UserID < 1 {
		return errs.New("操作身份无效")
	}
	if !scope.Platform && (scope.ID != id || action != "requestCancellation") {
		return errs.New("仅企业所有者可为本企业申请注销")
	}
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		t, err := lockCustomerAction(tx, id, version, code)
		if err != nil {
			return err
		}
		if !scope.Platform {
			if t.OwnerUserID == nil || *t.OwnerUserID != actor.UserID {
				return errs.New("仅当前企业所有者可申请注销")
			}
			var owner model.SysUser
			if err = tx.Where("tenant_id=? AND user_id=? AND user_type='01' AND del_flag='0' AND status='0'", id, actor.UserID).Take(&owner).Error; err != nil {
				return errs.New("企业所有者身份已变更")
			}
		}
		next, ok := lifecycleNextState(t.Status, action)
		if !ok {
			return errs.New("当前企业状态不允许此生命周期操作")
		}
		if err = tx.Model(&model.SysTenant{}).Where("tenant_id=?", id).Updates(map[string]any{"status": next, "version": gorm.Expr("version+1"), "update_by": actor.UserName, "update_time": types.Now()}).Error; err != nil {
			return err
		}
		auditActor := actor
		if !scope.Platform {
			var platform model.SysTenant
			if err = tx.Where("tenant_code='platform' AND tenant_kind='00'").Take(&platform).Error; err != nil {
				return err
			}
			auditActor.TenantID = platform.TenantID
		}
		return insertTenantActionAudit(tx, t, auditActor, "企业生命周期", "lifecycle", reason, map[string]any{"action": action, "previousStatus": t.Status, "nextStatus": next, "actorTenantId": actor.TenantID}, model.BusinessTypeUpdate)
	})
}
