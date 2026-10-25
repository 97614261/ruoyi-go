package model

import "ruoyi-go/pkg/types"

// Domain extension of the two actual-table generated 006 entities. No mutable CRUD.
type ERPSalesReturn struct {
	ERPRecord
	ReturnID        int64                `gorm:"column:return_id;primaryKey;autoIncrement" json:"returnId"`
	ReturnNo        string               `gorm:"column:return_no" json:"returnNo"`
	OrderID         int64                `gorm:"column:order_id" json:"orderId"`
	ShopID          int64                `gorm:"column:shop_id" json:"shopId"`
	PackageID       int64                `gorm:"column:package_id" json:"packageId"`
	WarehouseID     int64                `gorm:"column:warehouse_id" json:"warehouseId"`
	StockDocumentID int64                `gorm:"column:stock_document_id" json:"stockDocumentId"`
	ActorUserID     int64                `gorm:"column:actor_user_id" json:"actorUserId"`
	RequestKey      string               `gorm:"column:request_key" json:"-"`
	RequestHash     string               `gorm:"column:request_hash" json:"-"`
	FromVersion     int64                `gorm:"column:from_version" json:"fromVersion"`
	ToVersion       int64                `gorm:"column:to_version" json:"toVersion"`
	ResultJSON      string               `gorm:"column:result_json" json:"-"`
	Reason          string               `gorm:"column:reason" json:"reason"`
	PostedAt        types.Time           `gorm:"column:posted_at" json:"postedAt"`
	Lines           []ERPSalesReturnLine `gorm:"-" json:"lines,omitempty"`
}

func (ERPSalesReturn) TableName() string { return "erp_sales_return_order" }

type ERPSalesReturnLine struct {
	ERPRecord
	LineID        int64  `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	ReturnID      int64  `gorm:"column:return_id" json:"returnId"`
	PackageID     int64  `gorm:"column:package_id" json:"packageId"`
	PackageLineID int64  `gorm:"column:package_line_id" json:"packageLineId"`
	OrderID       int64  `gorm:"column:order_id" json:"orderId"`
	OrderLineID   int64  `gorm:"column:order_line_id" json:"orderLineId"`
	SkuID         int64  `gorm:"column:sku_id" json:"skuId"`
	Quantity      int64  `gorm:"column:quantity" json:"quantity"`
	SkuCode       string `gorm:"column:sku_code" json:"skuCode"`
	ProductName   string `gorm:"column:product_name" json:"productName"`
	Unit          string `gorm:"column:unit" json:"unit"`
}

func (ERPSalesReturnLine) TableName() string { return "erp_sales_return_line" }

type ERPSalesReturnLineInput struct {
	PackageLineID int64 `json:"packageLineId" binding:"min=1"`
	Quantity      int64 `json:"quantity" binding:"min=1,max=1000000000"`
}
type ERPSalesReturnInput struct {
	ERPActionToken
	Reason string                    `json:"reason" binding:"required,max=500"`
	Lines  []ERPSalesReturnLineInput `json:"lines" binding:"required,min=1,max=100,dive"`
}
type ERPSalesReturnResult struct {
	ReturnID        int64 `json:"returnId"`
	PackageID       int64 `json:"packageId"`
	OrderID         int64 `json:"orderId"`
	StockDocumentID int64 `json:"stockDocumentId"`
	Version         int64 `json:"version"`
	Existing        bool  `json:"existing"`
}
type ERPSalesReturnQuery struct {
	ShopID      int64  `form:"shopId" binding:"min=0"`
	PackageNo   string `form:"packageNo" binding:"max=64"`
	PackageID   int64  `form:"packageId" binding:"min=0"`
	OrderID     int64  `form:"orderId" binding:"min=0"`
	WarehouseID int64  `form:"warehouseId" binding:"min=0"`
	ReturnNo    string `form:"returnNo" binding:"max=64"`
}
type ERPSalesReturnable struct {
	ShopID             int64                    `json:"shopId"`
	LegacyStockReturn  bool                     `json:"legacyStockReturn"`
	PackageID          int64                    `json:"packageId"`
	PackageNo          string                   `json:"packageNo"`
	OrderID            int64                    `json:"orderId"`
	WarehouseID        int64                    `json:"warehouseId"`
	StockDocumentID    int64                    `json:"stockDocumentId"`
	ShippedAt          types.Time               `json:"shippedAt"`
	Version            int64                    `json:"version"`
	ShippedQuantity    int64                    `json:"shippedQuantity"`
	ReturnedQuantity   int64                    `json:"returnedQuantity"`
	NetShippedQuantity *int64                   `gorm:"-" json:"netShippedQuantity"`
	Lines              []ERPSalesReturnableLine `gorm:"-" json:"lines,omitempty"`
}
type ERPSalesReturnableLine struct {
	ERPPackageLine
	ReturnedQuantity   int64 `json:"returnedQuantity"`
	NetShippedQuantity int64 `json:"netShippedQuantity"`
}
