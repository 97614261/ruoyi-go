package model

// Extensions never change the immutable 001/002 schemas.
type ERPWarehouseProfile struct {
	ERPRecord
	ProfileID   int64  `gorm:"column:profile_id;primaryKey;autoIncrement" json:"-"`
	WarehouseID int64  `json:"warehouseId"`
	Address     string `json:"address"`
	Manager     string `json:"manager"`
	Version     int64  `json:"version"`
}

func (ERPWarehouseProfile) TableName() string { return "erp_warehouse_profile" }

type ERPStockAlert struct {
	ERPRecord
	AlertID           int64  `gorm:"column:alert_id;primaryKey;autoIncrement" json:"-"`
	WarehouseID       int64  `json:"warehouseId" binding:"required,min=1"`
	SkuID             int64  `json:"skuId" binding:"required,min=1"`
	ThresholdQuantity *int64 `json:"thresholdQuantity" binding:"omitempty,min=0,max=1000000000000"`
	Version           int64  `json:"version" binding:"min=0"`
}

func (ERPStockAlert) TableName() string { return "erp_stock_alert" }

type ERPTransfer struct {
	ERPRecord
	TransferID          int64             `gorm:"column:transfer_id;primaryKey;autoIncrement" json:"transferId"`
	TransferNo          string            `json:"transferNo"`
	SourceWarehouseID   int64             `json:"sourceWarehouseId"`
	TargetWarehouseID   int64             `json:"targetWarehouseId"`
	SourceWarehouseName string            `json:"sourceWarehouseName"`
	TargetWarehouseName string            `json:"targetWarehouseName"`
	Status              string            `json:"status"`
	Version             int64             `json:"version"`
	ActorUserID         int64             `json:"-"`
	RequestKey          string            `json:"-"`
	RequestHash         string            `json:"-"`
	ResultJSON          string            `gorm:"column:result_json" json:"-"`
	OutDocumentID       *int64            `json:"outDocumentId"`
	InDocumentID        *int64            `json:"inDocumentId"`
	Lines               []ERPTransferLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPTransfer) TableName() string { return "erp_transfer" }

type ERPTransferLine struct {
	ERPRecord
	LineID        int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	TransferID    int64  `json:"transferId"`
	SkuID         int64  `json:"skuId"`
	Quantity      int64  `json:"quantity"`
	SkuCode       string `json:"skuCode"`
	ProductName   string `json:"productName"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
}

func (ERPTransferLine) TableName() string { return "erp_transfer_line" }

type ERPTransferAction struct {
	ERPRecord
	ActionID    int64  `gorm:"column:action_id;primaryKey;autoIncrement" json:"-"`
	TransferID  int64  `json:"transferId"`
	ActorUserID int64  `json:"-"`
	Action      string `json:"action"`
	RequestKey  string `json:"-"`
	RequestHash string `json:"-"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (ERPTransferAction) TableName() string { return "erp_transfer_action" }

type ERPTransferInput struct {
	SourceWarehouseID int64          `json:"sourceWarehouseId" binding:"required,min=1"`
	TargetWarehouseID int64          `json:"targetWarehouseId" binding:"required,min=1"`
	Lines             []ERPLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
	Remark            string         `json:"remark" binding:"max=500"`
	Version           int64          `json:"version" binding:"min=0"`
	RequestKey        string         `json:"requestKey" binding:"required,max=64"`
}
type ERPTransferOperation struct {
	Version    int64  `json:"version" binding:"min=1"`
	RequestKey string `json:"requestKey" binding:"required,max=64"`
}
type ERPTransferResult struct {
	TransferID int64  `json:"transferId"`
	Status     string `json:"status"`
	Version    int64  `json:"version"`
}
type ERPTransferQuery struct {
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	TransferNo  string `form:"transferNo" binding:"max=64"`
	Status      string `form:"status" binding:"omitempty,oneof=DRAFT IN_TRANSIT COMPLETED CANCELLED"`
}
type ERPStockReconciliation struct {
	WarehouseID        int64 `json:"warehouseId"`
	SkuID              int64 `json:"skuId"`
	OnHand             int64 `json:"onHand"`
	Reserved           int64 `json:"reserved"`
	LedgerOnHand       int64 `json:"ledgerOnHand"`
	LedgerReserved     int64 `json:"ledgerReserved"`
	OnHandDifference   int64 `json:"onHandDifference"`
	ReservedDifference int64 `json:"reservedDifference"`
	Consistent         bool  `json:"consistent"`
	BalanceMissing     bool  `json:"balanceMissing"`
}
