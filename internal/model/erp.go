package model

import "ruoyi-go/pkg/types"

const ERPMaxStock int64 = 1000000000000

type ERPRecord struct {
	TenantRecord
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark     string     `gorm:"column:remark" json:"remark"`
}
type ERPDocument struct {
	ERPRecord
	DocumentID    int64             `gorm:"column:document_id;primaryKey;autoIncrement" json:"documentId"`
	DocumentNo    string            `gorm:"column:document_no" json:"documentNo"`
	Kind          string            `gorm:"column:kind" json:"kind"`
	WarehouseID   int64             `gorm:"column:warehouse_id" json:"warehouseId"`
	WarehouseName string            `gorm:"column:warehouse_name" json:"warehouseName"`
	Status        string            `gorm:"column:status" json:"status"`
	RequestKey    string            `gorm:"column:request_key" json:"-"`
	RequestHash   string            `gorm:"column:request_hash" json:"-"`
	Lines         []ERPDocumentLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPDocument) TableName() string { return "erp_document" }

type ERPDocumentLine struct {
	ERPRecord
	LineID        int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	DocumentID    int64  `gorm:"column:document_id" json:"documentId"`
	SkuID         int64  `gorm:"column:sku_id" json:"skuId"`
	SkuCode       string `gorm:"column:sku_code" json:"skuCode"`
	ProductName   string `gorm:"column:product_name" json:"productName"`
	Specification string `gorm:"column:specification" json:"specification"`
	Unit          string `gorm:"column:unit" json:"unit"`
	Quantity      int64  `gorm:"column:quantity" json:"quantity"`
}

func (ERPDocumentLine) TableName() string { return "erp_document_line" }

type ERPStock struct {
	ERPRecord
	StockID     int64 `gorm:"column:stock_id;primaryKey;autoIncrement" json:"stockId"`
	WarehouseID int64 `gorm:"column:warehouse_id" json:"warehouseId"`
	SkuID       int64 `gorm:"column:sku_id" json:"skuId"`
	OnHand      int64 `gorm:"column:on_hand" json:"onHand"`
	Reserved    int64 `gorm:"column:reserved" json:"reserved"`
}

func (ERPStock) TableName() string { return "erp_stock" }

type ERPStockView struct {
	ERPStock
	WarehouseName string `json:"warehouseName"`
	SkuCode       string `json:"skuCode"`
	ProductName   string `json:"productName"`
	Unit          string `json:"unit"`
	Available     int64  `json:"available"`
}
type ERPLedger struct {
	ERPRecord
	LedgerID      int64  `gorm:"column:ledger_id;primaryKey;autoIncrement" json:"ledgerId"`
	DocumentID    int64  `gorm:"column:document_id" json:"documentId"`
	DocumentNo    string `gorm:"column:document_no" json:"documentNo"`
	WarehouseID   int64  `gorm:"column:warehouse_id" json:"warehouseId"`
	SkuID         int64  `gorm:"column:sku_id" json:"skuId"`
	Event         string `gorm:"column:event" json:"event"`
	OnHandDelta   int64  `gorm:"column:on_hand_delta" json:"onHandDelta"`
	ReservedDelta int64  `gorm:"column:reserved_delta" json:"reservedDelta"`
	OnHandAfter   int64  `gorm:"column:on_hand_after" json:"onHandAfter"`
	ReservedAfter int64  `gorm:"column:reserved_after" json:"reservedAfter"`
}

func (ERPLedger) TableName() string { return "erp_stock_ledger" }

type ERPLineInput struct {
	SkuID    int64 `json:"skuId" binding:"required,min=1"`
	Quantity int64 `json:"quantity" binding:"required,min=1,max=1000000000"`
}
type ERPDocumentInput struct {
	WarehouseID int64          `json:"warehouseId" binding:"required,min=1"`
	RequestKey  string         `json:"requestKey" binding:"notblank,max=64"`
	Remark      string         `json:"remark" binding:"max=500"`
	Lines       []ERPLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
}
type ERPQuery struct {
	Status      string `form:"status" binding:"max=16"`
	DocumentNo  string `form:"documentNo" binding:"max=64"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	SkuID       int64  `form:"skuId" binding:"min=0"`
	DocumentID  int64  `form:"documentId" binding:"min=0"`
}
