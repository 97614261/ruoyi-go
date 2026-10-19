package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// VerifyTenantSchema is read-only and runs before Redis/jobs are initialized.
func VerifyTenantSchema(ctx context.Context) error {
	q := DB(tenantctx.System(ctx))
	var tables, columns int64
	if err := q.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('sys_tenant','sys_tenant_menu','sys_tenant_policy','sys_file')").Scan(&tables).Error; err != nil {
		return err
	}
	if err := q.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='sys_user' AND column_name='tenant_id'").Scan(&columns).Error; err != nil {
		return err
	}
	if tables != 4 || columns != 1 {
		return errs.New("当前库缺少多租户结构；请初始化独立新库，禁止使用原单租户库")
	}
	return nil
}

// EnsureTenantOperationsSchema is never called by the server startup path.
func EnsureTenantOperationsSchema(ctx context.Context, ddl string) error {
	q := DB(tenantctx.System(ctx))
	var database string
	if err := q.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
		return err
	}
	if database != TenantDatabaseName || !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS sys_tenant_policy") {
		return errs.New("拒绝在原库或未知脚本上执行配额迁移")
	}
	if err := q.Exec(ddl).Error; err != nil {
		return err
	}
	// Preserve policies, never set defaults below existing committed usage.
	return q.Exec(`INSERT INTO sys_tenant_policy(tenant_id,max_users,max_storage_bytes,max_upload_bytes,create_by,create_time)
 SELECT t.tenant_id,CASE WHEN t.tenant_kind='00' THEN 0 ELSE GREATEST(100,COALESCE(u.n,0)) END,
 CASE WHEN t.tenant_kind='00' THEN 0 ELSE GREATEST(1073741824,COALESCE(f.n,0)) END,10485760,'policy-migration',NOW()
 FROM sys_tenant t LEFT JOIN (SELECT tenant_id,COUNT(*) n FROM sys_user WHERE del_flag='0' GROUP BY tenant_id) u ON u.tenant_id=t.tenant_id
 LEFT JOIN (SELECT tenant_id,SUM(size_bytes) n FROM sys_file GROUP BY tenant_id) f ON f.tenant_id=t.tenant_id
 WHERE 1=1 ON DUPLICATE KEY UPDATE tenant_id=VALUES(tenant_id)`).Error
}

func provisionTenantPolicy(tx *gorm.DB, t *model.SysTenant) error {
	p := model.TenantPolicy{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, MaxUsers: 100, MaxStorageBytes: 1 << 30, MaxUploadBytes: 10 << 20, CreateBy: "bootstrap", CreateTime: types.Now()}
	if t.TenantKind == "00" {
		p.MaxUsers, p.MaxStorageBytes = 0, 0
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error
}

func lockedTenantPolicy(tx *gorm.DB, id int64) (*model.TenantPolicy, error) {
	var p model.TenantPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=?", id).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.New("企业配额未初始化，请联系平台")
		}
		return nil, err
	}
	if p.MaxUsers < 0 || p.MaxStorageBytes < 0 || p.MaxUploadBytes < 0 {
		return nil, errs.New("企业配额配置异常，请联系平台")
	}
	return &p, nil
}

func tenantUsage(tx *gorm.DB, id int64) (model.TenantUsage, error) {
	var usage model.TenantUsage
	if err := tx.Model(&model.SysUser{}).Where("tenant_id=? AND del_flag='0'", id).Count(&usage.UserCount).Error; err != nil {
		return usage, err
	}
	err := tx.Model(&model.SysFile{}).Where("tenant_id=?", id).Select("COALESCE(SUM(size_bytes),0)").Scan(&usage.StorageBytes).Error
	return usage, err
}

func checkAccountQuota(tx *gorm.DB) error {
	id := tenantctx.Get(tx.Statement.Context).ID
	if id < 1 {
		return tenantctx.ErrMissing
	}
	if err := lockQuotaTenantRoot(tx, id); err != nil {
		return err
	}
	p, err := lockedTenantPolicy(tx, id)
	if err != nil {
		return err
	}
	if p.MaxUsers == 0 {
		return nil
	}
	var count int64
	if err = tx.Model(&model.SysUser{}).Where("tenant_id=? AND del_flag='0'", id).Count(&count).Error; err != nil {
		return err
	}
	if count >= p.MaxUsers {
		return errs.New("已达到企业账号数上限，请联系平台调整配额")
	}
	return nil
}

func checkFileQuota(tx *gorm.DB, size int64) error {
	id := tenantctx.Get(tx.Statement.Context).ID
	if id < 1 {
		return tenantctx.ErrMissing
	}
	if err := lockQuotaTenantRoot(tx, id); err != nil {
		return err
	}
	p, err := lockedTenantPolicy(tx, id)
	if err != nil {
		return err
	}
	if size < 0 || (p.MaxUploadBytes > 0 && size > p.MaxUploadBytes) {
		return errs.New("文件超过企业单文件上传上限")
	}
	if p.MaxStorageBytes == 0 {
		return nil
	}
	var used int64
	if err = tx.Model(&model.SysFile{}).Where("tenant_id=?", id).Select("COALESCE(SUM(size_bytes),0)").Scan(&used).Error; err != nil {
		return err
	}
	if used > p.MaxStorageBytes || size > p.MaxStorageBytes-used {
		return errs.New("企业附件容量不足，请联系平台调整配额")
	}
	return nil
}

// Always lock parent tenant before policy, matching quota/lifecycle updates.
// Otherwise sys_file's FK parent lock can deadlock against a policy editor.
func lockQuotaTenantRoot(tx *gorm.DB, id int64) error {
	var t model.SysTenant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=?", id).Take(&t).Error; err != nil {
		return err
	}
	if t.Status != model.TenantActive {
		return errs.New("企业已暂停使用，不能新增账号或附件")
	}
	return nil
}

func CurrentTenantUploadLimit(ctx context.Context) (int64, error) {
	if _, err := tenantctx.Require(ctx); err != nil {
		return 0, err
	}
	var p model.TenantPolicy
	if err := DB(ctx).Take(&p).Error; err != nil {
		return 0, err
	}
	if p.MaxUploadBytes < 0 {
		return 0, errs.New("企业上传配额配置异常")
	}
	return p.MaxUploadBytes, nil
}

func CustomerQuotaSettings(ctx context.Context, id int64) (model.TenantQuotaSettings, error) {
	var result model.TenantQuotaSettings
	err := Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var t model.SysTenant
		if err := tx.Where("tenant_id=? AND tenant_kind='01'", id).Take(&t).Error; err != nil {
			return customerTenantError(err)
		}
		if err := tx.Where("tenant_id=?", id).Take(&result.Policy).Error; err != nil {
			return err
		}
		usage, err := tenantUsage(tx, id)
		if err != nil {
			return err
		}
		result.TenantID, result.TenantCode, result.TenantName, result.Status, result.Version, result.Usage = t.TenantID, t.TenantCode, t.TenantName, t.Status, t.Version, usage
		return nil
	})
	return result, err
}

func UpdateCustomerQuota(ctx context.Context, id, version int64, code, reason string, p model.TenantPolicy, actor model.PlatformActor) error {
	if !validPlatformActor(ctx, actor) {
		return errs.New("平台操作身份无效")
	}
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		t, err := lockCustomerAction(tx, id, version, code)
		if err != nil {
			return err
		}
		if err = customerMutable(t); err != nil {
			return err
		}
		if _, err = lockedTenantPolicy(tx, id); err != nil {
			return err
		}
		usage, err := tenantUsage(tx, id)
		if err != nil {
			return err
		}
		if (p.MaxUsers > 0 && p.MaxUsers < usage.UserCount) || (p.MaxStorageBytes > 0 && p.MaxStorageBytes < usage.StorageBytes) {
			return errs.New("新配额不能低于当前账号数或已登记附件用量")
		}
		if err = tx.Model(&model.TenantPolicy{}).Where("tenant_id=?", id).Updates(map[string]any{"max_users": p.MaxUsers, "max_storage_bytes": p.MaxStorageBytes, "max_upload_bytes": p.MaxUploadBytes, "update_by": actor.UserName, "update_time": types.Now()}).Error; err != nil {
			return err
		}
		if err = advanceTenantActionVersion(tx, id, actor.UserName); err != nil {
			return err
		}
		return insertTenantActionAudit(tx, t, actor, "企业配额调整", "quota", reason, map[string]any{"maxUsers": p.MaxUsers, "maxStorageBytes": p.MaxStorageBytes, "maxUploadBytes": p.MaxUploadBytes}, model.BusinessTypeUpdate)
	})
}
