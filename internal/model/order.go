package model

import "ruoyi-go/pkg/types"

type ERPOrderFields struct {
	ReceiverName     string     `gorm:"column:receiver_name" json:"receiverName"`
	ReceiverPhone    string     `gorm:"column:receiver_phone" json:"receiverPhone"`
	ReceiverCountry  string     `gorm:"column:receiver_country" json:"receiverCountry"`
	ReceiverRegion   string     `gorm:"column:receiver_region" json:"receiverRegion"`
	ReceiverCity     string     `gorm:"column:receiver_city" json:"receiverCity"`
	ReceiverPostcode string     `gorm:"column:receiver_postcode" json:"receiverPostcode"`
	ReceiverAddress  string     `gorm:"column:receiver_address" json:"receiverAddress"`
	Currency         string     `gorm:"column:currency" json:"currency"`
	OriginalAmount   *string    `gorm:"column:original_amount" json:"originalAmount"`
	OrderedAt        types.Time `gorm:"column:ordered_at" json:"orderedAt"`
}
type ERPOrder struct {
	ERPRecord
	ERPOrderFields
	OrderID           int64                `gorm:"column:order_id;primaryKey;autoIncrement" json:"orderId"`
	ShopID            int64                `gorm:"column:shop_id" json:"shopId"`
	OrderNo           string               `gorm:"column:order_no" json:"orderNo"`
	SourceType        string               `gorm:"column:source_type" json:"sourceType"`
	SourceOrderNo     string               `gorm:"column:source_order_no" json:"sourceOrderNo"`
	CreateRequestKey  string               `gorm:"column:create_request_key" json:"-"`
	CreateRequestHash string               `gorm:"column:create_request_hash" json:"-"`
	Status            string               `gorm:"column:status" json:"status"`
	Version           int64                `gorm:"column:version" json:"version"`
	WarehouseID       *int64               `gorm:"column:warehouse_id" json:"warehouseId"`
	StockDocumentID   *int64               `gorm:"column:stock_document_id" json:"stockDocumentId"`
	ReviewedAt        types.Time           `gorm:"column:reviewed_at" json:"reviewedAt"`
	ShippedAt         types.Time           `gorm:"column:shipped_at" json:"shippedAt"`
	CancelledAt       types.Time           `gorm:"column:cancelled_at" json:"cancelledAt"`
	CancelReason      string               `gorm:"column:cancel_reason" json:"cancelReason"`
	Package           *ERPOrderPackageLink `gorm:"-" json:"package"`
	Lines             []ERPOrderLine       `gorm:"-" json:"lines"`
}

func (ERPOrder) TableName() string { return "erp_order" }

type ERPOrderLine struct {
	ERPRecord
	LineID         int64   `gorm:"column:line_id;primaryKey;autoIncrement" json:"lineId"`
	ShopID         int64   `gorm:"column:shop_id" json:"shopId"`
	OrderID        int64   `gorm:"column:order_id" json:"orderId"`
	SourceLineNo   string  `gorm:"column:source_line_no" json:"sourceLineNo"`
	ShopItemCode   string  `gorm:"column:shop_item_code" json:"shopItemCode"`
	SourceTitle    string  `gorm:"column:source_title" json:"sourceTitle"`
	Quantity       int64   `gorm:"column:quantity" json:"quantity"`
	ShopItemID     *int64  `gorm:"column:shop_item_id" json:"shopItemId"`
	MappingID      *int64  `gorm:"column:mapping_id" json:"mappingId"`
	MappingVersion *int64  `gorm:"column:mapping_version" json:"mappingVersion"`
	SkuID          *int64  `gorm:"column:sku_id" json:"skuId"`
	SkuCode        *string `gorm:"column:sku_code" json:"skuCode"`
	ProductName    *string `gorm:"column:product_name" json:"productName"`
	Specification  *string `gorm:"column:specification" json:"specification"`
	Unit           *string `gorm:"column:unit" json:"unit"`
}

func (ERPOrderLine) TableName() string { return "erp_order_line" }

type ERPOrderLineInput struct {
	SourceLineNo string `json:"sourceLineNo"`
	ShopItemCode string `json:"shopItemCode"`
	SourceTitle  string `json:"sourceTitle"`
	Quantity     int64  `json:"quantity"`
}
type ERPOrderWrite struct {
	ERPOrderFields
	ShopID        int64               `json:"shopId"`
	SourceOrderNo string              `json:"sourceOrderNo"`
	RequestKey    string              `json:"requestKey"`
	Version       int64               `json:"version"`
	Lines         []ERPOrderLineInput `json:"lines"`
}
type ERPOrderQuery struct {
	ShopID        int64  `form:"shopId" binding:"min=0"`
	Status        string `form:"status"`
	SourceOrderNo string `form:"sourceOrderNo"`
}
type ERPOrderSummary struct {
	OrderID        int64      `json:"orderId" excel:"name:内部ID"`
	ShopID         int64      `json:"shopId" excel:"name:店铺ID"`
	OrderNo        string     `json:"orderNo" excel:"name:订单号"`
	SourceOrderNo  string     `json:"sourceOrderNo" excel:"name:来源订单号"`
	SourceType     string     `json:"sourceType" excel:"name:来源"`
	Status         string     `json:"status" excel:"name:状态"`
	Version        int64      `json:"version"`
	Currency       string     `json:"currency" excel:"name:币种"`
	OriginalAmount *string    `json:"originalAmount" excel:"name:原始金额"`
	OrderedAt      types.Time `json:"orderedAt" excel:"name:下单时间"`
}
type ERPOrderResult struct {
	OrderID  int64 `json:"orderId"`
	Version  int64 `json:"version"`
	Existing bool  `json:"existing"`
}
type ERPOrderEvent struct {
	ERPRecord
	EventID     int64  `gorm:"column:event_id;primaryKey;autoIncrement" json:"eventId"`
	OrderID     int64  `gorm:"column:order_id" json:"orderId"`
	ActorUserID int64  `gorm:"column:actor_user_id" json:"actorUserId"`
	Action      string `gorm:"column:action" json:"action"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	FromStatus  string `gorm:"column:from_status" json:"fromStatus"`
	ToStatus    string `gorm:"column:to_status" json:"toStatus"`
	FromVersion int64  `gorm:"column:from_version" json:"fromVersion"`
	ToVersion   int64  `gorm:"column:to_version" json:"toVersion"`
}

func (ERPOrderEvent) TableName() string { return "erp_order_event" }
