package repository

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Catalog lookups are deliberately separate from business queries. Callers must
// validate tenant state/membership before binding the result to a request.
func ResolveTenant(ctx context.Context, code string) (*model.SysTenant, error) {
	var t model.SysTenant
	err := DB(tenantctx.System(ctx)).Where("tenant_code = ?", code).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}
func TenantByID(ctx context.Context, id int64) (*model.SysTenant, error) {
	var t model.SysTenant
	err := DB(tenantctx.System(ctx)).Where("tenant_id = ?", id).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &t, err
}
func TenantPage(ctx context.Context, name string, pg page.Query) ([]model.SysTenant, int64, error) {
	q := DB(tenantctx.System(ctx)).Model(&model.SysTenant{}).Where("tenant_kind = '01'")
	if name != "" {
		q = q.Where("tenant_name LIKE ?", "%"+name+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.SysTenant{}
	err := q.Order("tenant_id").Limit(pg.PageSize).Offset(pg.Offset()).Find(&rows).Error
	return rows, total, err
}
func UpdateTenant(ctx context.Context, id int64, name, status string, expiry types.Time, ownerID int64, actor model.PlatformActor, version int64) error {
	if !validPlatformActor(ctx, actor) {
		return errs.New("平台操作身份无效")
	}
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var tenant model.SysTenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND tenant_kind='01'", id).Take(&tenant).Error; err != nil {
			return errors.New("租户不存在或不允许修改")
		}
		if tenant.Version != version {
			return errs.New("企业信息已变更，请刷新后重试")
		}
		if tenant.Status == model.TenantArchived || tenant.Status == model.TenantCancellationPending {
			return errs.New("请先通过生命周期流程恢复企业")
		}
		updates := map[string]any{"tenant_name": name, "status": status, "expire_time": expiry, "version": gorm.Expr("version + 1"), "update_by": actor.UserName, "update_time": types.Now()}
		if ownerID > 0 && (tenant.OwnerUserID == nil || *tenant.OwnerUserID != ownerID) {
			var next model.SysUser
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND del_flag='0' AND status='0'", id, ownerID).Take(&next).Error; err != nil {
				return errors.New("新所有者必须是本企业已启用员工")
			}
			var role model.SysRole
			if err := tx.Where("tenant_id=? AND role_key='tenant_admin' AND del_flag='0'", id).Take(&role).Error; err != nil {
				return err
			}
			if tenant.OwnerUserID != nil {
				if err := tx.Model(&model.SysUser{}).Where("tenant_id=? AND user_id=?", id, *tenant.OwnerUserID).Update("user_type", "00").Error; err != nil {
					return err
				}
				if err := tx.Where("tenant_id=? AND user_id=? AND role_id=?", id, *tenant.OwnerUserID, role.RoleID).Delete(&model.SysUserRole{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.SysUser{}).Where("tenant_id=? AND user_id=?", id, ownerID).Update("user_type", "01").Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SysUserRole{TenantRecord: model.TenantRecord{TenantID: id}, UserID: ownerID, RoleID: role.RoleID}).Error; err != nil {
				return err
			}
			updates["owner_user_id"] = ownerID
		}
		if err := tx.Model(&model.SysTenant{}).Where("tenant_id=?", id).Updates(updates).Error; err != nil {
			return err
		}
		return insertTenantActionAudit(tx, &tenant, actor, "企业信息变更", "update", "企业信息或所有者变更", map[string]any{"tenantName": name, "status": status, "ownerUserId": ownerID}, model.BusinessTypeUpdate)
	})
}
func TenantMenuPermissions(ctx context.Context) ([]string, error) {
	var perms []string
	err := DB(ctx).Model(&model.SysMenu{}).Where("status = '0' AND perms IS NOT NULL AND perms <> ''").Distinct().Pluck("perms", &perms).Error
	return perms, err
}

// ProvisionTenant copies only platform-owned onboarding defaults, never another
// customer's current data. It commits the complete owner and ACL atomically.
func ProvisionTenant(ctx context.Context, t *model.SysTenant, username, hash string) error {
	requestedStatus := t.Status
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		t.Status = "2"
		t.TenantKind = "01"
		t.CreateTime = types.Now()
		t.Version = 1
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		if err := provisionOwner(tx, t, username, hash, "01"); err != nil {
			return err
		}
		if requestedStatus == "1" {
			t.Status = "1"
			return tx.Model(&model.SysTenant{}).Where("tenant_id=?", t.TenantID).Update("status", "1").Error
		}
		return nil
	})
}
func provisionOwner(tx *gorm.DB, t *model.SysTenant, username, hash, userType string) error {
	if err := provisionTenantPolicy(tx, t); err != nil {
		return err
	}
	zero := 0
	dept := model.SysDept{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, DeptName: t.TenantName, Ancestors: "0", OrderNum: &zero, Status: "0", DelFlag: "0", CreateBy: "bootstrap", CreateTime: types.Now()}
	if err := tx.Create(&dept).Error; err != nil {
		return err
	}
	roleKey := "tenant_admin"
	if userType == "02" {
		roleKey = "platform_admin"
	}
	roleName := "企业管理员"
	if userType == "02" {
		roleName = "平台超级管理员"
	}
	role := model.SysRole{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, RoleName: roleName, RoleKey: roleKey, RoleSort: &zero, Status: "0", DelFlag: "0", DataScope: "1", CreateBy: "bootstrap", CreateTime: types.Now()}
	if err := tx.Create(&role).Error; err != nil {
		return err
	}
	u := model.SysUser{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, UserName: username, NickName: "管理员", UserType: userType, Password: hash, DeptID: &dept.DeptID, Status: "0", DelFlag: "0", CreateBy: "bootstrap", CreateTime: types.Now(), PwdUpdateDate: types.Now()}
	if err := tx.Create(&u).Error; err != nil {
		return err
	}
	if err := tx.Create(&model.SysUserRole{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, UserID: u.UserID, RoleID: role.RoleID}).Error; err != nil {
		return err
	}
	if userType != "02" {
		queries := []string{
			`INSERT INTO sys_config(tenant_id,config_name,config_key,config_value,config_type,create_by,create_time,remark) SELECT ?,config_name,config_key,CASE WHEN config_key='sys.login.blackIPList' THEN '' ELSE config_value END,config_type,'bootstrap',NOW(),remark FROM sys_config WHERE tenant_id=1 AND config_key IN (` + onboardingConfigKeys + `)`,
			`INSERT INTO sys_dict_type(tenant_id,dict_name,dict_type,status,create_by,create_time,remark) SELECT ?,dict_name,dict_type,status,'bootstrap',NOW(),remark FROM sys_dict_type WHERE tenant_id=1 AND dict_type IN (` + onboardingDictTypes + `)`,
			`INSERT INTO sys_dict_data(tenant_id,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,create_by,create_time,remark) SELECT ?,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,'bootstrap',NOW(),remark FROM sys_dict_data WHERE tenant_id=1 AND dict_type IN (` + onboardingDictTypes + `)`,
			`INSERT INTO sys_tenant_menu(tenant_id,menu_id) SELECT ?,menu_id FROM sys_menu WHERE ` + customerMenuCondition(""),
		}
		for _, sql := range queries {
			if err := tx.Exec(sql, t.TenantID).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Exec("INSERT INTO sys_role_menu(tenant_id,role_id,menu_id) SELECT ?,?,menu_id FROM sys_tenant_menu WHERE tenant_id=?", t.TenantID, role.RoleID, t.TenantID).Error; err != nil {
		return err
	}
	t.OwnerUserID = &u.UserID
	t.Status = "0"
	return tx.Model(&model.SysTenant{}).Where("tenant_id = ?", t.TenantID).Updates(map[string]any{"owner_user_id": u.UserID, "status": "0"}).Error
}
func BootstrapPlatformOwner(ctx context.Context, hash string) error {
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var t model.SysTenant
		if err := tx.Where("tenant_id=1").Take(&t).Error; err != nil {
			return err
		}
		if t.OwnerUserID != nil {
			return fmt.Errorf("平台所有者已存在")
		}
		return provisionOwner(tx, &t, "admin", hash, "02")
	})
}
