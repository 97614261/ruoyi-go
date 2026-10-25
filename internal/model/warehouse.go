package model

import "ruoyi-go/pkg/types"

func (ErpWarehouse) TableName() string { return "erp_warehouse" }

// ErpWarehouse 仓库。
type ErpWarehouse struct {
	TenantID      int64      `gorm:"column:tenant_id" json:"-"`
	WarehouseId   int64      `gorm:"column:warehouse_id;primaryKey;autoIncrement" json:"warehouseId"` // 仓库编号
	WarehouseCode string     `gorm:"column:warehouse_code" json:"warehouseCode" binding:"notblank,max=64" excel:"name:仓库编码"`
	WarehouseName string     `gorm:"column:warehouse_name" json:"warehouseName" binding:"notblank,max=120" excel:"name:仓库名称"`
	Status        string     `gorm:"column:status" json:"status" binding:"oneof=0 1" excel:"name:状态"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`     // 字段
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"` // 字段
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`     // 字段
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"` // 字段
	Remark        string     `gorm:"column:remark" json:"remark" binding:"max=500"`
	Address       string     `gorm:"-" json:"address,omitempty" binding:"max=300"`
	Manager       string     `gorm:"-" json:"manager,omitempty" binding:"max=64"`
	Version       int64      `gorm:"-" json:"version" binding:"min=0"`
}

// ErpWarehouseQuery 查询条件。
type ErpWarehouseQuery struct {
	WarehouseCode string `form:"warehouseCode"`
	WarehouseName string `form:"warehouseName"`
	Status        string `form:"status" binding:"omitempty,oneof=0 1"`
}

var ErpWarehouseSortColumns = map[string]string{
	"warehouseId":   "warehouse_id",
	"warehouseCode": "warehouse_code",
	"warehouseName": "warehouse_name",
	"status":        "status",
}
