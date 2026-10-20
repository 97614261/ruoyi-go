package model

import "ruoyi-go/pkg/types"

func (ErpSku) TableName() string { return "erp_sku" }

// ErpSku 商品SKU。
type ErpSku struct {
	TenantID      int64      `gorm:"column:tenant_id" json:"-"`
	SkuId         int64      `gorm:"column:sku_id;primaryKey;autoIncrement" json:"skuId"` // SKU编号
	SkuCode       string     `gorm:"column:sku_code" json:"skuCode" binding:"notblank,max=64" excel:"name:SKU编码"`
	ProductName   string     `gorm:"column:product_name" json:"productName" binding:"notblank,max=120" excel:"name:商品名称"`
	Specification string     `gorm:"column:specification" json:"specification" binding:"max=120" excel:"name:规格"`
	Unit          string     `gorm:"column:unit" json:"unit" binding:"notblank,max=20" excel:"name:单位"`
	Status        string     `gorm:"column:status" json:"status" binding:"oneof=0 1" excel:"name:状态"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`     // 字段
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"` // 字段
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`     // 字段
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"` // 字段
	Remark        string     `gorm:"column:remark" json:"remark" binding:"max=500"`
}

// ErpSkuQuery 查询条件。
type ErpSkuQuery struct {
	SkuCode       string `form:"skuCode"`
	ProductName   string `form:"productName"`
	Specification string `form:"specification"`
	Unit          string `form:"unit"`
}

var ErpSkuSortColumns = map[string]string{
	"skuId":         "sku_id",
	"skuCode":       "sku_code",
	"productName":   "product_name",
	"specification": "specification",
	"unit":          "unit",
	"status":        "status",
}
