package model

import "ruoyi-go/pkg/types"

type PickingItem struct {
	SkuID         int64  `json:"skuId"`
	SkuCode       string `json:"skuCode"`
	ProductName   string `json:"productName"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
	Quantity      int64  `json:"quantity"`
	ScanCode      string `json:"scanCode"`
}
type PickingSheet struct {
	PackageID     int64         `json:"packageId"`
	PackageNo     string        `json:"packageNo"`
	OrderID       int64         `json:"orderId"`
	WarehouseID   int64         `json:"warehouseId"`
	WarehouseName string        `json:"warehouseName"`
	Version       int64         `json:"version"`
	Status        string        `json:"status"`
	HasLogistics  bool          `json:"hasLogistics"`
	Scannable     bool          `json:"scannable"`
	BlockReason   string        `json:"blockReason"`
	Checked       bool          `json:"checked"`
	CheckedAt     types.Time    `json:"checkedAt"`
	Items         []PickingItem `json:"items"`
}
type PickingScan struct {
	ScanCode string `json:"scanCode"`
	Quantity int64  `json:"quantity"`
}
type PickingCheckInput struct {
	ERPActionToken
	Scans []PickingScan `json:"scans"`
}
