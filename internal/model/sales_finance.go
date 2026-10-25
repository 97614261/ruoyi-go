package model

// Extended from the actual009 generator output. Dedicated input DTOs protect
// server-owned columns; money is decimal strings, never generic float64.
type SalesFinReceivable struct {
	ERPRecord
	ReceivableID       int64                    `gorm:"column:receivable_id;primaryKey;autoIncrement" json:"receivableId"`
	ReceivableNo       string                   `gorm:"column:receivable_no" json:"receivableNo"`
	OrderID            int64                    `gorm:"column:order_id" json:"orderId"`
	PackageID          int64                    `gorm:"column:package_id" json:"packageId"`
	ShopID             int64                    `gorm:"column:shop_id" json:"shopId"`
	WarehouseID        int64                    `gorm:"column:warehouse_id" json:"warehouseId"`
	ShopName           string                   `gorm:"column:shop_name" json:"shopName"`
	OrderNo            string                   `gorm:"column:order_no" json:"orderNo"`
	Currency           string                   `gorm:"column:currency" json:"currency"`
	GrossAmount        string                   `gorm:"column:gross_amount" json:"grossAmount"`
	CreditAmount       string                   `gorm:"column:credit_amount" json:"creditAmount"`
	ReceivedAmount     string                   `gorm:"column:received_amount" json:"receivedAmount"`
	Version            int64                    `gorm:"column:version" json:"version"`
	NetAmount          string                   `gorm:"-" json:"netAmount"`
	DueAmount          string                   `gorm:"-" json:"dueAmount"`
	OverreceivedAmount string                   `gorm:"-" json:"overreceivedAmount"`
	PendingReturnCount int64                    `gorm:"-" json:"pendingReturnCount"`
	Lines              []SalesFinReceivableLine `gorm:"-" json:"lines,omitempty"`
}

func (SalesFinReceivable) TableName() string { return "erp_sf_receivable" }

type SalesFinReceivableLine struct {
	ERPRecord
	LineID       int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	ReceivableID int64  `gorm:"column:receivable_id" json:"receivableId"`
	OrderLineID  int64  `gorm:"column:order_line_id" json:"orderLineId"`
	OrderID      int64  `gorm:"column:order_id" json:"orderId"`
	SkuCode      string `gorm:"column:sku_code" json:"skuCode"`
	ProductName  string `gorm:"column:product_name" json:"productName"`
	Unit         string `gorm:"column:unit" json:"unit"`
	Quantity     int64  `gorm:"column:quantity" json:"quantity"`
}

func (SalesFinReceivableLine) TableName() string { return "erp_sf_receivable_line" }

type SalesFinCredit struct {
	ERPRecord
	CreditID     int64  `gorm:"column:credit_id;primaryKey;autoIncrement" json:"creditId"`
	ReceivableID int64  `gorm:"column:receivable_id" json:"receivableId"`
	ReturnID     int64  `gorm:"column:return_id" json:"returnId"`
	PackageID    int64  `gorm:"column:package_id" json:"packageId"`
	OrderID      int64  `gorm:"column:order_id" json:"orderId"`
	ReturnNo     string `gorm:"column:return_no" json:"returnNo"`
	Amount       string `gorm:"column:amount" json:"amount"`
	Registered   bool   `gorm:"-" json:"registered"`
}

func (SalesFinCredit) TableName() string { return "erp_sf_credit" }

type SalesFinCollection struct {
	ERPRecord
	CollectionID  int64                `gorm:"column:collection_id;primaryKey;autoIncrement" json:"collectionId"`
	CollectionNo  string               `gorm:"column:collection_no" json:"collectionNo"`
	AccountID     int64                `gorm:"column:account_id" json:"accountId"`
	AccountName   string               `gorm:"column:account_name" json:"accountName"`
	ShopID        int64                `gorm:"column:shop_id" json:"shopId"`
	ShopName      string               `gorm:"column:shop_name" json:"shopName"`
	Currency      string               `gorm:"column:currency" json:"currency"`
	Amount        string               `gorm:"column:amount" json:"amount"`
	Status        string               `gorm:"column:status" json:"status"`
	Version       int64                `gorm:"column:version" json:"version"`
	ReverseReason string               `gorm:"column:reverse_reason" json:"reverseReason,omitempty"`
	Allocations   []SalesFinAllocation `gorm:"-" json:"allocations,omitempty"`
}

func (SalesFinCollection) TableName() string { return "erp_sf_collection" }

type SalesFinAllocation struct {
	ERPRecord
	AllocationID int64  `gorm:"column:allocation_id;primaryKey;autoIncrement" json:"allocationId"`
	CollectionID int64  `gorm:"column:collection_id" json:"collectionId"`
	ReceivableID int64  `gorm:"column:receivable_id" json:"receivableId"`
	Amount       string `gorm:"column:amount" json:"amount"`
}

func (SalesFinAllocation) TableName() string { return "erp_sf_allocation" }

type SalesFinAction struct {
	ERPRecord
	ActionID    int64  `gorm:"column:action_id;primaryKey;autoIncrement" json:"-"`
	ActorUserID int64  `gorm:"column:actor_user_id" json:"-"`
	Action      string `gorm:"column:action" json:"-"`
	TargetID    int64  `gorm:"column:target_id" json:"-"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (SalesFinAction) TableName() string { return "erp_sf_action" }

type SalesFinReceivableCreate struct {
	OrderID    int64  `json:"orderId" binding:"min=1"`
	RequestKey string `json:"requestKey" binding:"required,max=64"`
}
type SalesFinAdjust struct {
	FinToken
	ReturnID int64  `json:"returnId" binding:"min=1"`
	Amount   string `json:"amount" binding:"required,max=21"`
	Reason   string `json:"reason" binding:"required,max=500"`
}
type SalesFinAllocationInput struct {
	ReceivableID int64  `json:"receivableId" binding:"min=1"`
	Version      int64  `json:"version" binding:"min=1"`
	Amount       string `json:"amount" binding:"required,max=21"`
}
type SalesFinCollectionWrite struct {
	RequestKey  string                    `json:"requestKey" binding:"required,max=64"`
	AccountID   int64                     `json:"accountId" binding:"min=1"`
	ShopID      int64                     `json:"shopId" binding:"min=1"`
	Currency    string                    `json:"currency" binding:"required,len=3"`
	Amount      string                    `json:"amount" binding:"required,max=21"`
	Remark      string                    `json:"remark" binding:"max=500"`
	Allocations []SalesFinAllocationInput `json:"allocations" binding:"required,min=1,max=100,dive"`
}
type SalesFinQuery struct {
	ShopID      int64  `form:"shopId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	AccountID   int64  `form:"accountId" binding:"min=0"`
	OrderID     int64  `form:"orderId" binding:"min=0"`
	Currency    string `form:"currency" binding:"omitempty,len=3"`
	Status      string `form:"status" binding:"max=16"`
	Code        string `form:"code" binding:"max=64"`
}
type SalesFinSource struct {
	OrderID     int64                    `gorm:"column:order_id" json:"orderId"`
	OrderNo     string                   `gorm:"column:order_no" json:"orderNo"`
	PackageID   int64                    `gorm:"column:package_id" json:"packageId"`
	ShopID      int64                    `gorm:"column:shop_id" json:"shopId"`
	ShopName    string                   `gorm:"column:shop_name" json:"shopName"`
	WarehouseID int64                    `gorm:"column:warehouse_id" json:"warehouseId"`
	Currency    string                   `gorm:"column:currency" json:"currency"`
	GrossAmount string                   `gorm:"-" json:"grossAmount,omitempty"`
	Lines       []SalesFinReceivableLine `gorm:"-" json:"lines,omitempty"`
}
type SalesFinStatement struct {
	ShopID             int64  `gorm:"column:shop_id" json:"shopId"`
	ShopName           string `gorm:"column:shop_name" json:"shopName"`
	Currency           string `gorm:"column:currency" json:"currency"`
	GrossAmount        string `gorm:"column:gross_amount" json:"grossAmount"`
	CreditAmount       string `gorm:"column:credit_amount" json:"creditAmount"`
	ReceivedAmount     string `gorm:"column:received_amount" json:"receivedAmount"`
	DueAmount          string `gorm:"column:due_amount" json:"dueAmount"`
	OverreceivedAmount string `gorm:"column:overreceived_amount" json:"overreceivedAmount"`
}
