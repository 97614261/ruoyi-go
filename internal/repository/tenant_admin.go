package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// CustomerTenantMenuState uses a consistent snapshot for the optimistic version
// and its checked keys. The caller must already be in the platform workspace.
func CustomerTenantMenuState(ctx context.Context, id int64, limit int) (*model.SysTenant, []model.SysMenu, []int64, error) {
	if !tenantctx.Get(ctx).Platform {
		return nil, nil, nil, errs.New("仅平台管理员可操作")
	}
	var tenant model.SysTenant
	menus, checked := []model.SysMenu{}, []int64{}
	err := Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id=? AND tenant_kind='01'", id).Take(&tenant).Error; err != nil {
			return customerTenantError(err)
		}
		if err := tx.Where(customerMenuCondition("")).Where("status='0' AND menu_type IN ?", []string{"M", "C", "F"}).
			Order("parent_id, order_num, menu_id").Limit(limit + 1).Find(&menus).Error; err != nil {
			return err
		}
		if len(menus) > limit {
			return errs.New("功能目录超过节点上限，请先整理目录")
		}
		return tx.Model(&model.SysTenantMenu{}).Where("tenant_id=?", id).Order("menu_id").Pluck("menu_id", &checked).Error
	})
	return &tenant, menus, checked, err
}

func customerTenantError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errs.New("企业不存在或不允许操作平台空间")
	}
	return err
}

func lockCustomerAction(tx *gorm.DB, id, version int64, code string) (*model.SysTenant, error) {
	var tenant model.SysTenant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND tenant_kind='01'", id).Take(&tenant).Error; err != nil {
		return nil, customerTenantError(err)
	}
	if tenant.TenantCode != code {
		return nil, errs.New("企业编码确认不一致")
	}
	if tenant.Version != version {
		return nil, errs.New("企业信息已变更，请刷新后重试")
	}
	return &tenant, nil
}

// ReplaceCustomerTenantMenus deletes role grants before their referenced tenant
// grants. Reopening a feature must not resurrect a normal role's revoked rights.
func ReplaceCustomerTenantMenus(ctx context.Context, id, version int64, code string, ids []int64, reason string, actor model.PlatformActor) error {
	if !validPlatformActor(ctx, actor) {
		return errs.New("平台操作身份无效")
	}
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		tenant, err := lockCustomerAction(tx, id, version, code)
		if err != nil {
			return err
		}
		if err = customerMutable(tenant); err != nil {
			return err
		}
		if len(ids) > 0 {
			var found int64
			if err = tx.Model(&model.SysMenu{}).Where("menu_id IN ?", ids).Where(customerMenuCondition("")).Where("status='0'").Count(&found).Error; err != nil {
				return err
			}
			if found != int64(len(ids)) {
				return errs.New("授权目录已变化或包含平台专属功能，请刷新后重试")
			}
		}
		removeRole := tx.Where("tenant_id=?", id)
		removeGrant := tx.Where("tenant_id=?", id)
		if len(ids) > 0 {
			removeRole = removeRole.Where("menu_id NOT IN ?", ids)
			removeGrant = removeGrant.Where("menu_id NOT IN ?", ids)
		}
		if err = removeRole.Delete(&model.SysRoleMenu{}).Error; err != nil {
			return err
		}
		if err = removeGrant.Delete(&model.SysTenantMenu{}).Error; err != nil {
			return err
		}
		var admin model.SysRole
		if err = tx.Where("tenant_id=? AND role_key='tenant_admin' AND del_flag='0'", id).Take(&admin).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			grants := make([]model.SysTenantMenu, 0, len(ids))
			ownerGrants := make([]model.SysRoleMenu, 0, len(ids))
			for _, menuID := range ids {
				grants = append(grants, model.SysTenantMenu{TenantRecord: model.TenantRecord{TenantID: id}, MenuID: menuID})
				ownerGrants = append(ownerGrants, model.SysRoleMenu{TenantRecord: model.TenantRecord{TenantID: id}, RoleID: admin.RoleID, MenuID: menuID})
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grants).Error; err != nil {
				return err
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ownerGrants).Error; err != nil {
				return err
			}
		}
		if err = advanceTenantActionVersion(tx, id, actor.UserName); err != nil {
			return err
		}
		encoded, _ := json.Marshal(ids)
		details := map[string]any{"menuCount": len(ids), "menuIdsSHA256": fmt.Sprintf("%x", sha256.Sum256(encoded))}
		return insertTenantActionAudit(tx, tenant, actor, "企业功能授权", "menus", reason, details, model.BusinessTypeGrant)
	})
}

// ResetCustomerOwnerPassword changes only the explicitly confirmed current
// owner. The password, persistent session fence and success audit commit together.
func ResetCustomerOwnerPassword(ctx context.Context, id, version, ownerID int64, code, hash, reason string, actor model.PlatformActor) error {
	if !validPlatformActor(ctx, actor) {
		return errs.New("平台操作身份无效")
	}
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		tenant, err := lockCustomerAction(tx, id, version, code)
		if err != nil {
			return err
		}
		if err = customerMutable(tenant); err != nil {
			return err
		}
		if tenant.OwnerUserID == nil || *tenant.OwnerUserID != ownerID {
			return errs.New("企业所有者已变化，请刷新后重试")
		}
		var owner model.SysUser
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND user_type='01' AND del_flag='0' AND status='0'", id, ownerID).Take(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.New("企业管理员不存在或未启用")
			}
			return err
		}
		if err = tx.Model(&model.SysUser{}).Where("tenant_id=? AND user_id=?", id, ownerID).
			Updates(map[string]any{"password": hash, "pwd_update_date": types.Now(), "update_by": actor.UserName, "update_time": types.Now()}).Error; err != nil {
			return err
		}
		if err = advanceTenantActionVersion(tx, id, actor.UserName); err != nil {
			return err
		}
		return insertTenantActionAudit(tx, tenant, actor, "重置企业管理员密码", "resetOwnerPwd", reason,
			map[string]any{"ownerUserId": ownerID, "ownerUserName": owner.UserName}, model.BusinessTypeUpdate)
	})
}

func validPlatformActor(ctx context.Context, actor model.PlatformActor) bool {
	s := tenantctx.Get(ctx)
	return s.Platform && s.ID > 0 && actor.TenantID == s.ID && actor.UserID > 0 && actor.UserName != ""
}

func advanceTenantActionVersion(tx *gorm.DB, id int64, actor string) error {
	return tx.Model(&model.SysTenant{}).Where("tenant_id=?", id).
		Updates(map[string]any{"version": gorm.Expr("version+1"), "update_by": actor, "update_time": types.Now()}).Error
}

func insertTenantActionAudit(tx *gorm.DB, tenant *model.SysTenant, actor model.PlatformActor, title, action, reason string, details map[string]any, businessType int) error {
	params := map[string]any{"tenantId": tenant.TenantID, "tenantCode": tenant.TenantCode, "previousVersion": tenant.Version, "operatorUserId": actor.UserID, "reason": reason, "details": details}
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if len(encoded) > 1900 {
		return errs.New("审计信息过长")
	}
	record := model.SysOperLog{TenantRecord: model.TenantRecord{TenantID: actor.TenantID}, Title: title, BusinessType: businessType,
		Method: "platform.tenant." + action, RequestMethod: "PUT", OperatorType: 1, OperName: actor.UserName, OperIP: actor.IP,
		OperURL: fmt.Sprintf("/platform/tenant/%d/%s", tenant.TenantID, action), OperParam: string(encoded), JSONResult: `{"code":200}`, Status: model.OperStatusSuccess, OperTime: types.Now()}
	return tx.Create(&record).Error
}
