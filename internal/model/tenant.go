package model

import "ruoyi-go/pkg/types"

type TenantRecord struct {
	TenantID int64 `gorm:"column:tenant_id" json:"-"`
}
type SysTenant struct {
	TenantID    int64      `gorm:"column:tenant_id;primaryKey;autoIncrement" json:"tenantId"`
	TenantCode  string     `gorm:"column:tenant_code" json:"tenantCode"`
	TenantName  string     `gorm:"column:tenant_name" json:"tenantName"`
	TenantKind  string     `gorm:"column:tenant_kind" json:"tenantKind"`
	OwnerUserID *int64     `gorm:"column:owner_user_id" json:"ownerUserId"`
	Status      string     `gorm:"column:status" json:"status"`
	Version     int64      `gorm:"column:version" json:"version"`
	ExpireTime  types.Time `gorm:"column:expire_time" json:"expireTime"`
	CreateBy    string     `gorm:"column:create_by" json:"createBy"`
	CreateTime  types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy    string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime  types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark      string     `gorm:"column:remark" json:"remark"`
}

func (SysTenant) TableName() string { return "sys_tenant" }

type SysTenantMenu struct {
	TenantRecord
	MenuID int64 `gorm:"column:menu_id;primaryKey" json:"menuId"`
}

func (SysTenantMenu) TableName() string { return "sys_tenant_menu" }
