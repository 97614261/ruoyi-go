package model

import "ruoyi-go/pkg/types"

// Extended from the generated erp_purchase_order and order_line models. Money
// deliberately uses strings instead of the generic generator's float64 fields.
type ERPPurchase struct {
	ERPRecord
	PurchaseID        int64             `gorm:"column:purchase_id;primaryKey;autoIncrement" json:"purchaseId"`
	PurchaseNo        string            `gorm:"column:purchase_no" json:"purchaseNo"`
	SupplierID        int64             `gorm:"column:supplier_id" json:"supplierId"`
	WarehouseID       int64             `gorm:"column:warehouse_id" json:"warehouseId"`
	SupplierCode      string            `gorm:"column:supplier_code" json:"supplierCode"`
	SupplierName      string            `gorm:"column:supplier_name" json:"supplierName"`
	WarehouseName     string            `gorm:"column:warehouse_name" json:"warehouseName"`
	Currency          string            `gorm:"column:currency" json:"currency"`
	Status            string            `gorm:"column:status" json:"status"`
	Version           int64             `gorm:"column:version" json:"version"`
	CreateRequestKey  string            `gorm:"column:create_request_key" json:"-"`
	CreateRequestHash string            `gorm:"column:create_request_hash" json:"-"`
	OrderedAmount     string            `gorm:"column:ordered_amount" json:"orderedAmount"`
	ConfirmedAt       types.Time        `gorm:"column:confirmed_at" json:"confirmedAt"`
	ClosedAt          types.Time        `gorm:"column:closed_at" json:"closedAt"`
	CancelledAt       types.Time        `gorm:"column:cancelled_at" json:"cancelledAt"`
	CloseReason       string            `gorm:"column:close_reason" json:"closeReason"`
	Lines             []ERPPurchaseLine `gorm:"-" json:"lines"`
}

func (ERPPurchase) TableName() string { return "erp_purchase_order" }

type ERPPurchaseLine struct {
	ERPRecord
	LineID           int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	PurchaseID       int64  `gorm:"column:purchase_id" json:"purchaseId"`
	LineNo           int64  `gorm:"column:line_no" json:"lineNo"`
	SkuID            int64  `gorm:"column:sku_id" json:"skuId"`
	SkuCode          string `gorm:"column:sku_code" json:"skuCode"`
	ProductName      string `gorm:"column:product_name" json:"productName"`
	Specification    string `gorm:"column:specification" json:"specification"`
	Unit             string `gorm:"column:unit" json:"unit"`
	OrderedQuantity  int64  `gorm:"column:ordered_quantity" json:"orderedQuantity"`
	ReceivedQuantity int64  `gorm:"column:received_quantity" json:"receivedQuantity"`
	UnitPrice        string `gorm:"column:unit_price" json:"unitPrice"`
	LineAmount       string `gorm:"column:line_amount" json:"lineAmount"`
}

func (ERPPurchaseLine) TableName() string { return "erp_purchase_order_line" }

type ERPPurchaseEvent struct {
	ERPRecord
	EventID     int64  `gorm:"column:event_id;primaryKey;autoIncrement" json:"eventId"`
	PurchaseID  int64  `gorm:"column:purchase_id" json:"purchaseId"`
	ReceiptID   *int64 `gorm:"column:receipt_id" json:"receiptId"`
	ActorUserID int64  `gorm:"column:actor_user_id" json:"actorUserId"`
	Action      string `gorm:"column:action" json:"action"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	FromStatus  string `gorm:"column:from_status" json:"fromStatus"`
	ToStatus    string `gorm:"column:to_status" json:"toStatus"`
	FromVersion int64  `gorm:"column:from_version" json:"fromVersion"`
	ToVersion   int64  `gorm:"column:to_version" json:"toVersion"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (ERPPurchaseEvent) TableName() string { return "erp_purchase_event" }

type ERPPurchaseLineInput struct {
	SkuID      int64  `json:"skuId" binding:"min=1"`
	Quantity   int64  `json:"quantity" binding:"min=1,max=1000000000"`
	UnitPrice  string `json:"unitPrice" binding:"required,max=21"`
	LineAmount string `json:"-"`
}
type ERPPurchaseWrite struct {
	SupplierID    int64                  `json:"supplierId" binding:"min=1"`
	WarehouseID   int64                  `json:"warehouseId" binding:"min=1"`
	Currency      string                 `json:"currency" binding:"required,len=3"`
	RequestKey    string                 `json:"requestKey" binding:"required,max=64"`
	Remark        string                 `json:"remark" binding:"max=500"`
	Lines         []ERPPurchaseLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
	OrderedAmount string                 `json:"-"`
}
type ERPPurchaseEdit struct {
	ERPPurchaseWrite
	Version int64 `json:"version" binding:"min=1"`
}
type ERPPurchaseAction struct {
	ERPActionToken
	Reason string `json:"reason" binding:"max=500"`
}
type ERPPurchaseResult struct {
	PurchaseID int64  `json:"purchaseId"`
	Version    int64  `json:"version"`
	Status     string `json:"status"`
	Existing   bool   `json:"existing"`
}
type ERPPurchaseQuery struct {
	AwaitingReceipt bool   `form:"awaitingReceipt"`
	PurchaseNo      string `form:"purchaseNo" binding:"max=64"`
	SupplierID      int64  `form:"supplierId" binding:"min=0"`
	WarehouseID     int64  `form:"warehouseId" binding:"min=0"`
	Status          string `form:"status" binding:"max=24"`
}
type ERPPurchaseSummary struct {
	OrderedQuantity  int64      `gorm:"-" json:"orderedQuantity"`
	ReceivedQuantity int64      `gorm:"-" json:"receivedQuantity"`
	PurchaseID       int64      `gorm:"column:purchase_id" json:"purchaseId"`
	PurchaseNo       string     `gorm:"column:purchase_no" json:"purchaseNo"`
	SupplierID       int64      `gorm:"column:supplier_id" json:"supplierId"`
	WarehouseID      int64      `gorm:"column:warehouse_id" json:"warehouseId"`
	SupplierName     string     `gorm:"column:supplier_name" json:"supplierName"`
	WarehouseName    string     `gorm:"column:warehouse_name" json:"warehouseName"`
	Currency         string     `gorm:"column:currency" json:"currency"`
	Status           string     `gorm:"column:status" json:"status"`
	Version          int64      `gorm:"column:version" json:"version"`
	CreateTime       types.Time `gorm:"column:create_time" json:"createTime"`
}

var ERPPurchaseSortColumns = map[string]string{"purchaseId": "purchase_id", "status": "status", "purchaseNo": "purchase_no"}
