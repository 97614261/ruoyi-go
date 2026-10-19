package model

import "ruoyi-go/pkg/types"

const (
	TenantActive              = "0"
	TenantDisabled            = "1"
	TenantCancellationPending = "3"
	TenantArchived            = "4"
)

type TenantPolicy struct {
	TenantRecord
	MaxUsers        int64      `gorm:"column:max_users" json:"maxUsers"`
	MaxStorageBytes int64      `gorm:"column:max_storage_bytes" json:"maxStorageBytes"`
	MaxUploadBytes  int64      `gorm:"column:max_upload_bytes" json:"maxUploadBytes"`
	CreateBy        string     `gorm:"column:create_by" json:"createBy"`
	CreateTime      types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy        string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime      types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark          string     `gorm:"column:remark" json:"remark"`
}

func (TenantPolicy) TableName() string { return "sys_tenant_policy" }

type TenantUsage struct {
	UserCount    int64 `json:"userCount"`
	StorageBytes int64 `json:"storageBytes"`
}
type TenantQuotaSettings struct {
	TenantID   int64        `json:"tenantId"`
	TenantCode string       `json:"tenantCode"`
	TenantName string       `json:"tenantName"`
	Status     string       `json:"status"`
	Version    int64        `json:"version"`
	Policy     TenantPolicy `json:"policy"`
	Usage      TenantUsage  `json:"usage"`
}
