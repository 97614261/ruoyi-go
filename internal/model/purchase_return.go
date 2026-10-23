package model

import "ruoyi-go/pkg/types"

// Domain extension of the two generated 005 entities. Immutable after posting.
type ERPPurchaseReturn struct {
	ERPRecord
	ReturnID        int64                   `gorm:"column:return_id;primaryKey;autoIncrement" json:"returnId"`
	ReturnNo        string                  `gorm:"column:return_no" json:"returnNo"`
	PurchaseID      int64                   `gorm:"column:purchase_id" json:"purchaseId"`
	ReceiptID       int64                   `gorm:"column:receipt_id" json:"receiptId"`
	WarehouseID     int64                   `gorm:"column:warehouse_id" json:"warehouseId"`
	StockDocumentID int64                   `gorm:"column:stock_document_id" json:"stockDocumentId"`
	ActorUserID     int64                   `gorm:"column:actor_user_id" json:"actorUserId"`
	RequestKey      string                  `gorm:"column:request_key" json:"-"`
	RequestHash     string                  `gorm:"column:request_hash" json:"-"`
	FromVersion     int64                   `gorm:"column:from_version" json:"fromVersion"`
	ToVersion       int64                   `gorm:"column:to_version" json:"toVersion"`
	ResultJSON      string                  `gorm:"column:result_json" json:"-"`
	Reason          string                  `gorm:"column:reason" json:"reason"`
	PostedAt        types.Time              `gorm:"column:posted_at" json:"postedAt"`
	Lines           []ERPPurchaseReturnLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPPurchaseReturn) TableName() string { return "erp_purchase_return_order" }

type ERPPurchaseReturnLine struct {
	ERPRecord
	LineID         int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	ReturnID       int64  `gorm:"column:return_id" json:"returnId"`
	ReceiptID      int64  `gorm:"column:receipt_id" json:"receiptId"`
	ReceiptLineID  int64  `gorm:"column:receipt_line_id" json:"receiptLineId"`
	PurchaseID     int64  `gorm:"column:purchase_id" json:"purchaseId"`
	PurchaseLineID int64  `gorm:"column:purchase_line_id" json:"purchaseLineId"`
	SkuID          int64  `gorm:"column:sku_id" json:"skuId"`
	Quantity       int64  `gorm:"column:quantity" json:"quantity"`
	SkuCode        string `gorm:"column:sku_code" json:"skuCode"`
	ProductName    string `gorm:"column:product_name" json:"productName"`
	Specification  string `gorm:"column:specification" json:"specification"`
	Unit           string `gorm:"column:unit" json:"unit"`
}

func (ERPPurchaseReturnLine) TableName() string { return "erp_purchase_return_line" }

type ERPPurchaseReturnLineInput struct {
	ReceiptLineID int64 `json:"receiptLineId" binding:"min=1"`
	Quantity      int64 `json:"quantity" binding:"min=1,max=1000000000"`
}
type ERPPurchaseReturnInput struct {
	ERPActionToken
	Reason string                       `json:"reason" binding:"required,max=500"`
	Lines  []ERPPurchaseReturnLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
}
type ERPPurchaseReturnResult struct {
	ReturnID        int64 `json:"returnId"`
	ReceiptID       int64 `json:"receiptId"`
	PurchaseID      int64 `json:"purchaseId"`
	StockDocumentID int64 `json:"stockDocumentId"`
	Version         int64 `json:"version"`
	Existing        bool  `json:"existing"`
}
type ERPPurchaseReturnQuery struct {
	ReceiptID   int64  `form:"receiptId" binding:"min=0"`
	PurchaseID  int64  `form:"purchaseId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	ReturnNo    string `form:"returnNo" binding:"max=64"`
}
type ERPPurchaseReturnable struct {
	ReceiptID           int64                       `json:"receiptId"`
	ReceiptNo           string                      `json:"receiptNo"`
	PurchaseID          int64                       `json:"purchaseId"`
	WarehouseID         int64                       `json:"warehouseId"`
	StockDocumentID     int64                       `json:"stockDocumentId"`
	PostedAt            types.Time                  `json:"postedAt"`
	Version             int64                       `json:"version"`
	ReceivedQuantity    int64                       `json:"receivedQuantity"`
	ReturnedQuantity    int64                       `json:"returnedQuantity"`
	NetReceivedQuantity int64                       `json:"netReceivedQuantity"`
	Lines               []ERPPurchaseReturnableLine `json:"lines,omitempty"`
}
type ERPPurchaseReturnableLine struct {
	ERPPurchaseReceiptLine
	ReturnedQuantity    int64 `json:"returnedQuantity"`
	NetReceivedQuantity int64 `json:"netReceivedQuantity"`
}
