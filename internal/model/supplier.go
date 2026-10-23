package model

import "ruoyi-go/pkg/types"

// ErpSupplier generated from erp_supplier, then extended with separate DTOs,
// immutable code, versioned writes and private contact fields.
type ErpSupplier struct {
	TenantID     int64      `gorm:"column:tenant_id" json:"-"`
	SupplierId   int64      `gorm:"column:supplier_id;primaryKey;autoIncrement" json:"supplierId"`
	SupplierCode string     `gorm:"column:supplier_code" json:"supplierCode"`
	SupplierName string     `gorm:"column:supplier_name" json:"supplierName"`
	ContactName  string     `gorm:"column:contact_name" json:"contactName"`
	Phone        string     `gorm:"column:phone" json:"phone"`
	Email        string     `gorm:"column:email" json:"email"`
	Address      string     `gorm:"column:address" json:"address"`
	Status       string     `gorm:"column:status" json:"status"`
	Version      int64      `gorm:"column:version" json:"version"`
	CreateBy     string     `gorm:"column:create_by" json:"createBy"`
	CreateTime   types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy     string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime   types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark       string     `gorm:"column:remark" json:"remark"`
}

func (ErpSupplier) TableName() string { return "erp_supplier" }

// Explicit projection: never add contacts or remark to the list to support a
// UI convenience. List permission must not imply private query permission.
type ErpSupplierListRow struct {
	SupplierId   int64      `gorm:"column:supplier_id" json:"supplierId"`
	SupplierCode string     `gorm:"column:supplier_code" json:"supplierCode"`
	SupplierName string     `gorm:"column:supplier_name" json:"supplierName"`
	Status       string     `gorm:"column:status" json:"status"`
	Version      int64      `gorm:"column:version" json:"version"`
	CreateTime   types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime   types.Time `gorm:"column:update_time" json:"updateTime"`
}
type ErpSupplierQuery struct {
	SupplierCode string `form:"supplierCode" binding:"max=64"`
	SupplierName string `form:"supplierName" binding:"max=120"`
	Status       string `form:"status" binding:"max=1"`
}
type ERPSupplierWrite struct {
	SupplierCode string `json:"supplierCode" binding:"required,max=64"`
	SupplierName string `json:"supplierName" binding:"required,max=120"`
	ContactName  string `json:"contactName" binding:"max=64"`
	Phone        string `json:"phone" binding:"max=64"`
	Email        string `json:"email" binding:"max=128"`
	Address      string `json:"address" binding:"max=500"`
	Status       string `json:"status" binding:"required"`
	Remark       string `json:"remark" binding:"max=500"`
}
type ERPSupplierEdit struct {
	ERPSupplierWrite
	Version int64 `json:"version" binding:"min=1"`
}
type ERPSupplierResult struct {
	SupplierId int64 `json:"supplierId"`
	Version    int64 `json:"version"`
}

var ErpSupplierSortColumns = map[string]string{
	"supplierId": "supplier_id", "supplierCode": "supplier_code", "supplierName": "supplier_name", "status": "status", "version": "version",
}
