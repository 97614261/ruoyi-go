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
	DocumentID       int64             `gorm:"column:document_id;primaryKey;autoIncrement" json:"documentId"`
	DocumentNo       string            `gorm:"column:document_no" json:"documentNo"`
	Kind             string            `gorm:"column:kind" json:"kind"`
	WarehouseID      int64             `gorm:"column:warehouse_id" json:"warehouseId"`
	WarehouseName    string            `gorm:"column:warehouse_name" json:"warehouseName"`
	Status           string            `gorm:"column:status" json:"status"`
	RequestKey       string            `gorm:"column:request_key" json:"-"`
	RequestHash      string            `gorm:"column:request_hash" json:"-"`
	Version          int64             `gorm:"column:version" json:"version"`
	SourceDocumentID *int64            `gorm:"column:source_document_id" json:"sourceDocumentId"`
	Reversal         bool              `gorm:"column:reversal" json:"reversal"`
	Lines            []ERPDocumentLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPDocument) TableName() string { return "erp_document" }

type ERPDocumentLine struct {
	ERPRecord
	LineID           int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	DocumentID       int64  `gorm:"column:document_id" json:"documentId"`
	SkuID            int64  `gorm:"column:sku_id" json:"skuId"`
	SkuCode          string `gorm:"column:sku_code" json:"skuCode"`
	ProductName      string `gorm:"column:product_name" json:"productName"`
	Specification    string `gorm:"column:specification" json:"specification"`
	Unit             string `gorm:"column:unit" json:"unit"`
	Quantity         int64  `gorm:"column:quantity" json:"quantity"`
	ExpectedOnHand   int64  `gorm:"column:expected_on_hand" json:"expectedOnHand"`
	ExpectedReserved int64  `gorm:"column:expected_reserved" json:"expectedReserved"`
	ExpectedVersion  int64  `gorm:"column:expected_version" json:"expectedVersion"`
}

func (ERPDocumentLine) TableName() string { return "erp_document_line" }

type ERPStock struct {
	ERPRecord
	StockID     int64 `gorm:"column:stock_id;primaryKey;autoIncrement" json:"stockId"`
	WarehouseID int64 `gorm:"column:warehouse_id" json:"warehouseId"`
	SkuID       int64 `gorm:"column:sku_id" json:"skuId"`
	OnHand      int64 `gorm:"column:on_hand" json:"onHand"`
	Reserved    int64 `gorm:"column:reserved" json:"reserved"`
	Version     int64 `gorm:"column:version" json:"version"`
}

func (ERPStock) TableName() string { return "erp_stock" }

type ERPStockView struct {
	ERPStock
	WarehouseName     string `json:"warehouseName"`
	SkuCode           string `json:"skuCode"`
	ProductName       string `json:"productName"`
	Unit              string `json:"unit"`
	Available         int64  `json:"available"`
	ThresholdQuantity *int64 `json:"thresholdQuantity"`
	AlertVersion      int64  `json:"alertVersion"`
	LowStock          bool   `json:"lowStock"`
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
	WarehouseName string `gorm:"->" json:"warehouseName"`
	SkuCode       string `gorm:"->" json:"skuCode"`
	ProductName   string `gorm:"->" json:"productName"`
	Unit          string `gorm:"->" json:"unit"`
	DocumentKind  string `gorm:"->" json:"documentKind"`
	TransferID    *int64 `gorm:"->" json:"transferId"`
}

func (ERPLedger) TableName() string { return "erp_stock_ledger" }

type ERPLineInput struct {
	SkuID           int64  `json:"skuId" binding:"required,min=1"`
	Quantity        int64  `json:"quantity" binding:"min=0,max=1000000000"`
	CountedQuantity *int64 `json:"countedQuantity,omitempty" binding:"omitempty,min=0,max=1000000000"`
}
type ERPDocumentInput struct {
	WarehouseID      int64          `json:"warehouseId" binding:"required,min=1"`
	RequestKey       string         `json:"requestKey" binding:"max=64"`
	Remark           string         `json:"remark" binding:"max=500"`
	Lines            []ERPLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
	SourceDocumentID *int64         `json:"sourceDocumentId" binding:"omitempty,min=1"`
	Reversal         bool           `json:"reversal"`
}

type ERPDocumentEdit struct {
	ERPDocumentInput
	Version int64 `json:"version" binding:"min=1"`
}

// Every employee defaults to no warehouses. Only the current tenant owner can
// replace the explicit set; missing/empty is never interpreted as all.
type ERPWarehouseAccess struct {
	ERPRecord
	AccessID    int64 `gorm:"column:access_id;primaryKey;autoIncrement" json:"accessId"`
	UserID      int64 `gorm:"column:user_id" json:"userId"`
	WarehouseID int64 `gorm:"column:warehouse_id" json:"warehouseId"`
}

func (ERPWarehouseAccess) TableName() string { return "erp_warehouse_access" }

type ERPAccessInput struct {
	WarehouseIDs []int64 `json:"warehouseIds" binding:"max=200,dive,min=1"`
	Version      int64   `json:"version" binding:"min=1"`
}
type ERPAccessRequest struct {
	WarehouseIDs *[]int64 `json:"warehouseIds" binding:"required,max=200,dive,min=1"`
	Version      int64    `json:"version" binding:"min=1"`
}
type ERPAccessPolicy struct {
	ERPRecord
	PolicyID int64 `gorm:"column:policy_id;primaryKey;autoIncrement" json:"policyId"`
	UserID   int64 `gorm:"column:user_id" json:"userId"`
	Version  int64 `gorm:"column:version" json:"version"`
}

func (ERPAccessPolicy) TableName() string { return "erp_warehouse_policy" }

type ERPAccessView struct {
	UserID       int64   `json:"userId"`
	WarehouseIDs []int64 `json:"warehouseIds"`
	Version      int64   `json:"version"`
}
type ERPQuery struct {
	Status      string `form:"status" binding:"max=16"`
	DocumentNo  string `form:"documentNo" binding:"max=64"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	SkuID       int64  `form:"skuId" binding:"min=0"`
	DocumentID  int64  `form:"documentId" binding:"min=0"`
	StockState  string `form:"stockState" binding:"omitempty,oneof=ALL IN_STOCK ZERO RESERVED_OUT LOW HAS_STOCK HAS_RESERVED"`
	Event       string `form:"event" binding:"omitempty,oneof=IN OUT RESERVE RELEASE SR PR COUNT TOUT TIN"`
	BeginTime   string `form:"beginTime" binding:"max=19"`
	EndTime     string `form:"endTime" binding:"max=19"`
}
