package model

import "ruoyi-go/pkg/types"

// Extended from the generated receipt/line models. No price or contact fields.
type ERPPurchaseReceipt struct {
	ERPRecord
	ReceiptID       int64                    `gorm:"column:receipt_id;primaryKey;autoIncrement" json:"receiptId"`
	ReceiptNo       string                   `gorm:"column:receipt_no" json:"receiptNo"`
	PurchaseID      int64                    `gorm:"column:purchase_id" json:"purchaseId"`
	WarehouseID     int64                    `gorm:"column:warehouse_id" json:"warehouseId"`
	StockDocumentID int64                    `gorm:"column:stock_document_id" json:"stockDocumentId"`
	ActorUserID     int64                    `gorm:"column:actor_user_id" json:"actorUserId"`
	RequestKey      string                   `gorm:"column:request_key" json:"-"`
	RequestHash     string                   `gorm:"column:request_hash" json:"-"`
	PostedAt        types.Time               `gorm:"column:posted_at" json:"postedAt"`
	Lines           []ERPPurchaseReceiptLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPPurchaseReceipt) TableName() string { return "erp_purchase_receipt" }

type ERPPurchaseReceiptLine struct {
	ERPRecord
	LineID         int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	ReceiptID      int64  `gorm:"column:receipt_id" json:"receiptId"`
	PurchaseID     int64  `gorm:"column:purchase_id" json:"purchaseId"`
	PurchaseLineID int64  `gorm:"column:purchase_line_id" json:"purchaseLineId"`
	SkuID          int64  `gorm:"column:sku_id" json:"skuId"`
	Quantity       int64  `gorm:"column:quantity" json:"quantity"`
	SkuCode        string `gorm:"column:sku_code" json:"skuCode"`
	ProductName    string `gorm:"column:product_name" json:"productName"`
	Specification  string `gorm:"column:specification" json:"specification"`
	Unit           string `gorm:"column:unit" json:"unit"`
}

func (ERPPurchaseReceiptLine) TableName() string { return "erp_purchase_receipt_line" }

type ERPPurchaseReceiveLineInput struct {
	PurchaseLineID int64 `json:"purchaseLineId" binding:"min=1"`
	Quantity       int64 `json:"quantity" binding:"min=1,max=1000000000"`
}
type ERPPurchaseReceiveInput struct {
	ERPActionToken
	Lines []ERPPurchaseReceiveLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
}
type ERPPurchaseReceiveResult struct {
	PurchaseID      int64  `json:"purchaseId"`
	ReceiptID       int64  `json:"receiptId"`
	StockDocumentID int64  `json:"stockDocumentId"`
	Status          string `json:"status"`
	Version         int64  `json:"version"`
	Existing        bool   `json:"existing"`
}
type ERPPurchaseReceivable struct {
	ERPPurchaseSummary
	Lines []ERPPurchaseReceivableLine `json:"lines"`
}
type ERPPurchaseReceivableLine struct {
	LineID           int64  `gorm:"column:line_id" json:"lineId"`
	SkuID            int64  `gorm:"column:sku_id" json:"skuId"`
	SkuCode          string `gorm:"column:sku_code" json:"skuCode"`
	ProductName      string `gorm:"column:product_name" json:"productName"`
	Specification    string `gorm:"column:specification" json:"specification"`
	Unit             string `gorm:"column:unit" json:"unit"`
	OrderedQuantity  int64  `gorm:"column:ordered_quantity" json:"orderedQuantity"`
	ReceivedQuantity int64  `gorm:"column:received_quantity" json:"receivedQuantity"`
}
type ERPPurchaseReceiptQuery struct {
	PurchaseID  int64  `form:"purchaseId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	ReceiptNo   string `form:"receiptNo" binding:"max=64"`
}
