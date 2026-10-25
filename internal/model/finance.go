package model

// Extended from the actual008 generator output. Dedicated input DTOs protect
// server-owned columns; money is decimal strings, never generic float64.
type FinAccount struct {
	ERPRecord
	AccountID   int64  `gorm:"column:account_id;primaryKey;autoIncrement" json:"accountId"`
	AccountCode string `gorm:"column:account_code" json:"accountCode"`
	AccountName string `gorm:"column:account_name" json:"accountName"`
	AccountType string `gorm:"column:account_type" json:"accountType"`
	Currency    string `gorm:"column:currency" json:"currency"`
	Status      string `gorm:"column:status" json:"status"`
	Version     int64  `gorm:"column:version" json:"version"`
}

func (FinAccount) TableName() string { return "erp_fin_account" }

type FinPayable struct {
	ERPRecord
	PayableID          int64            `gorm:"column:payable_id;primaryKey;autoIncrement" json:"payableId"`
	PayableNo          string           `gorm:"column:payable_no" json:"payableNo"`
	ReceiptID          int64            `gorm:"column:receipt_id" json:"receiptId"`
	PurchaseID         int64            `gorm:"column:purchase_id" json:"purchaseId"`
	SupplierID         int64            `gorm:"column:supplier_id" json:"supplierId"`
	WarehouseID        int64            `gorm:"column:warehouse_id" json:"warehouseId"`
	SupplierName       string           `gorm:"column:supplier_name" json:"supplierName"`
	ReceiptNo          string           `gorm:"column:receipt_no" json:"receiptNo"`
	Currency           string           `gorm:"column:currency" json:"currency"`
	GrossAmount        string           `gorm:"column:gross_amount" json:"grossAmount"`
	CreditAmount       string           `gorm:"column:credit_amount" json:"creditAmount"`
	PaidAmount         string           `gorm:"column:paid_amount" json:"paidAmount"`
	Version            int64            `gorm:"column:version" json:"version"`
	NetAmount          string           `gorm:"-" json:"netAmount"`
	DueAmount          string           `gorm:"-" json:"dueAmount"`
	OverpaidAmount     string           `gorm:"-" json:"overpaidAmount"`
	PendingReturnCount int64            `gorm:"-" json:"pendingReturnCount"`
	Lines              []FinPayableLine `gorm:"-" json:"lines,omitempty"`
}

func (FinPayable) TableName() string { return "erp_fin_payable" }

type FinPayableLine struct {
	ERPRecord
	LineID         int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	PayableID      int64  `gorm:"column:payable_id" json:"payableId"`
	ReceiptLineID  int64  `gorm:"column:receipt_line_id" json:"receiptLineId"`
	ReceiptID      int64  `gorm:"column:receipt_id" json:"receiptId"`
	PurchaseLineID int64  `gorm:"column:purchase_line_id" json:"purchaseLineId"`
	SkuCode        string `gorm:"column:sku_code" json:"skuCode"`
	ProductName    string `gorm:"column:product_name" json:"productName"`
	Unit           string `gorm:"column:unit" json:"unit"`
	Quantity       int64  `gorm:"column:quantity" json:"quantity"`
	UnitPrice      string `gorm:"column:unit_price" json:"unitPrice"`
	LineAmount     string `gorm:"column:line_amount" json:"lineAmount"`
}

func (FinPayableLine) TableName() string { return "erp_fin_payable_line" }

type FinCredit struct {
	ERPRecord
	CreditID   int64  `gorm:"column:credit_id;primaryKey;autoIncrement" json:"creditId"`
	PayableID  int64  `gorm:"column:payable_id" json:"payableId"`
	ReturnID   int64  `gorm:"column:return_id" json:"returnId"`
	PurchaseID int64  `gorm:"column:purchase_id" json:"purchaseId"`
	ReceiptID  int64  `gorm:"column:receipt_id" json:"receiptId"`
	ReturnNo   string `gorm:"column:return_no" json:"returnNo"`
	Amount     string `gorm:"column:amount" json:"amount"`
	Registered bool   `gorm:"-" json:"registered"`
}

func (FinCredit) TableName() string { return "erp_fin_credit" }

type FinPayment struct {
	ERPRecord
	PaymentID     int64           `gorm:"column:payment_id;primaryKey;autoIncrement" json:"paymentId"`
	PaymentNo     string          `gorm:"column:payment_no" json:"paymentNo"`
	AccountID     int64           `gorm:"column:account_id" json:"accountId"`
	AccountName   string          `gorm:"column:account_name" json:"accountName"`
	SupplierID    int64           `gorm:"column:supplier_id" json:"supplierId"`
	SupplierName  string          `gorm:"column:supplier_name" json:"supplierName"`
	Currency      string          `gorm:"column:currency" json:"currency"`
	Amount        string          `gorm:"column:amount" json:"amount"`
	Status        string          `gorm:"column:status" json:"status"`
	Version       int64           `gorm:"column:version" json:"version"`
	ReverseReason string          `gorm:"column:reverse_reason" json:"reverseReason,omitempty"`
	Allocations   []FinAllocation `gorm:"-" json:"allocations,omitempty"`
}

func (FinPayment) TableName() string { return "erp_fin_payment" }

type FinAllocation struct {
	ERPRecord
	AllocationID int64  `gorm:"column:allocation_id;primaryKey;autoIncrement" json:"allocationId"`
	PaymentID    int64  `gorm:"column:payment_id" json:"paymentId"`
	PayableID    int64  `gorm:"column:payable_id" json:"payableId"`
	Amount       string `gorm:"column:amount" json:"amount"`
}

func (FinAllocation) TableName() string { return "erp_fin_allocation" }

type FinAction struct {
	ERPRecord
	ActionID    int64  `gorm:"column:action_id;primaryKey;autoIncrement" json:"-"`
	ActorUserID int64  `gorm:"column:actor_user_id" json:"-"`
	Action      string `gorm:"column:action" json:"-"`
	TargetID    int64  `gorm:"column:target_id" json:"-"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (FinAction) TableName() string { return "erp_fin_action" }

type FinToken struct {
	Version    int64  `json:"version" binding:"min=1"`
	RequestKey string `json:"requestKey" binding:"required,max=64"`
}
type FinAccountWrite struct {
	RequestKey  string `json:"requestKey" binding:"required,max=64"`
	AccountCode string `json:"accountCode" binding:"required,max=64"`
	AccountName string `json:"accountName" binding:"required,max=120"`
	AccountType string `json:"accountType" binding:"required,oneof=BANK CASH OTHER"`
	Currency    string `json:"currency" binding:"required,len=3"`
	Status      string `json:"status" binding:"required,oneof=0 1"`
	Remark      string `json:"remark" binding:"max=500"`
	Version     int64  `json:"version"`
}
type FinPayableCreate struct {
	ReceiptID  int64  `json:"receiptId" binding:"min=1"`
	RequestKey string `json:"requestKey" binding:"required,max=64"`
}
type FinAdjust struct {
	FinToken
	ReturnIDs []int64 `json:"returnIds" binding:"required,min=1,max=100,dive,min=1"`
}
type FinAllocationInput struct {
	PayableID int64  `json:"payableId" binding:"min=1"`
	Version   int64  `json:"version" binding:"min=1"`
	Amount    string `json:"amount" binding:"required,max=21"`
}
type FinPaymentWrite struct {
	RequestKey  string               `json:"requestKey" binding:"required,max=64"`
	AccountID   int64                `json:"accountId" binding:"min=1"`
	SupplierID  int64                `json:"supplierId" binding:"min=1"`
	Currency    string               `json:"currency" binding:"required,len=3"`
	Amount      string               `json:"amount" binding:"required,max=21"`
	Remark      string               `json:"remark" binding:"max=500"`
	Allocations []FinAllocationInput `json:"allocations" binding:"required,min=1,max=100,dive"`
}
type FinReverse struct {
	FinToken
	Reason string `json:"reason" binding:"required,max=500"`
}
type FinResult struct {
	ID       int64  `json:"id"`
	Version  int64  `json:"version"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Existing bool   `json:"existing"`
}
type FinQuery struct {
	SupplierID  int64  `form:"supplierId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	AccountID   int64  `form:"accountId" binding:"min=0"`
	ReceiptID   int64  `form:"receiptId" binding:"min=0"`
	Currency    string `form:"currency" binding:"omitempty,len=3"`
	Status      string `form:"status" binding:"max=16"`
	Code        string `form:"code" binding:"max=64"`
}
type FinSource struct {
	ReceiptID    int64            `gorm:"column:receipt_id" json:"receiptId"`
	ReceiptNo    string           `gorm:"column:receipt_no" json:"receiptNo"`
	PurchaseID   int64            `gorm:"column:purchase_id" json:"purchaseId"`
	SupplierID   int64            `gorm:"column:supplier_id" json:"supplierId"`
	SupplierName string           `gorm:"column:supplier_name" json:"supplierName"`
	WarehouseID  int64            `gorm:"column:warehouse_id" json:"warehouseId"`
	Currency     string           `gorm:"column:currency" json:"currency"`
	GrossAmount  string           `gorm:"-" json:"grossAmount,omitempty"`
	Lines        []FinPayableLine `gorm:"-" json:"lines,omitempty"`
}
type FinStatement struct {
	SupplierID     int64  `gorm:"column:supplier_id" json:"supplierId"`
	SupplierName   string `gorm:"column:supplier_name" json:"supplierName"`
	Currency       string `gorm:"column:currency" json:"currency"`
	GrossAmount    string `gorm:"column:gross_amount" json:"grossAmount"`
	CreditAmount   string `gorm:"column:credit_amount" json:"creditAmount"`
	PaidAmount     string `gorm:"column:paid_amount" json:"paidAmount"`
	DueAmount      string `gorm:"column:due_amount" json:"dueAmount"`
	OverpaidAmount string `gorm:"column:overpaid_amount" json:"overpaidAmount"`
}
