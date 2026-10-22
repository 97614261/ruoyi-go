package model

import "ruoyi-go/pkg/types"

type ERPActionToken struct {
	Version    int64  `json:"version"`
	RequestKey string `json:"requestKey"`
}
type ERPReviewInput struct {
	ERPActionToken
	WarehouseID int64 `json:"warehouseId"`
}
type ERPCancelInput struct {
	ERPActionToken
	Reason string `json:"reason"`
}
type ERPLogisticsInput struct {
	ERPActionToken
	CarrierCode string `json:"carrierCode"`
	TrackingNo  string `json:"trackingNo"`
}
type ERPFulfillmentResult struct {
	OrderID        int64  `json:"orderId"`
	Version        int64  `json:"version"`
	Status         string `json:"status"`
	PackageID      *int64 `json:"packageId"`
	PackageVersion *int64 `json:"packageVersion"`
	Existing       bool   `json:"existing"`
}

// An order may show its own non-PII package link. Full package/logistics data
// is available only through the separately authorized package endpoint.
type ERPOrderPackageLink struct {
	PackageID int64  `json:"packageId"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
}
type ERPPackage struct {
	ERPRecord
	PackageID   int64            `gorm:"column:package_id;primaryKey;autoIncrement" json:"packageId"`
	OrderID     int64            `gorm:"column:order_id" json:"orderId"`
	WarehouseID int64            `gorm:"column:warehouse_id" json:"warehouseId"`
	PackageNo   string           `gorm:"column:package_no" json:"packageNo"`
	Status      string           `gorm:"column:status" json:"status"`
	Version     int64            `gorm:"column:version" json:"version"`
	CarrierCode string           `gorm:"column:carrier_code" json:"carrierCode"`
	TrackingNo  string           `gorm:"column:tracking_no" json:"trackingNo"`
	ShippedAt   types.Time       `gorm:"column:shipped_at" json:"shippedAt"`
	ShopID      int64            `gorm:"column:shop_id;->" json:"shopId"`
	Lines       []ERPPackageLine `gorm:"-" json:"lines"`
}

func (ERPPackage) TableName() string { return "erp_package" }

type ERPPackageLine struct {
	ERPRecord
	LineID      int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	OrderID     int64  `gorm:"column:order_id" json:"orderId"`
	PackageID   int64  `gorm:"column:package_id" json:"packageId"`
	OrderLineID int64  `gorm:"column:order_line_id" json:"orderLineId"`
	SkuID       int64  `gorm:"column:sku_id" json:"skuId"`
	Quantity    int64  `gorm:"column:quantity" json:"quantity"`
	SkuCode     string `gorm:"column:sku_code" json:"skuCode"`
	ProductName string `gorm:"column:product_name" json:"productName"`
	Unit        string `gorm:"column:unit" json:"unit"`
}

func (ERPPackageLine) TableName() string { return "erp_package_line" }

type ERPPackageQuery struct {
	ShopID      int64  `form:"shopId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	OrderID     int64  `form:"orderId" binding:"min=0"`
	Status      string `form:"status"`
}
type ERPPackageSummary struct {
	PackageID   int64      `json:"packageId"`
	PackageNo   string     `json:"packageNo"`
	OrderID     int64      `json:"orderId"`
	ShopID      int64      `json:"shopId"`
	WarehouseID int64      `json:"warehouseId"`
	Status      string     `json:"status"`
	Version     int64      `json:"version"`
	ShippedAt   types.Time `json:"shippedAt"`
}
