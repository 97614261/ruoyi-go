package model

import "ruoyi-go/pkg/types"

// Extended from the real011 generator output; inputs never expose server-owned
// tenant/audit columns or generic deletion of package snapshots/actions.
type LogisticsCarrier struct {
	ERPRecord
	CarrierID   int64  `gorm:"column:carrier_id;primaryKey;autoIncrement" json:"carrierId"`
	CarrierCode string `gorm:"column:carrier_code" json:"carrierCode"`
	CarrierName string `gorm:"column:carrier_name" json:"carrierName"`
	Status      string `gorm:"column:status" json:"status"`
	Version     int64  `gorm:"column:version" json:"version"`
}

func (LogisticsCarrier) TableName() string { return "erp_carrier" }

type ShippingMethod struct {
	ERPRecord
	ShippingMethodID int64  `gorm:"column:shipping_method_id;primaryKey;autoIncrement" json:"shippingMethodId"`
	CarrierID        int64  `gorm:"column:carrier_id" json:"carrierId"`
	MethodCode       string `gorm:"column:method_code" json:"methodCode"`
	MethodName       string `gorm:"column:method_name" json:"methodName"`
	Status           string `gorm:"column:status" json:"status"`
	Version          int64  `gorm:"column:version" json:"version"`
}

func (ShippingMethod) TableName() string { return "erp_shipping_method" }

type PackageShipping struct {
	ERPRecord
	ShippingID       int64  `gorm:"column:shipping_id;primaryKey;autoIncrement" json:"-"`
	OrderID          int64  `gorm:"column:order_id" json:"orderId"`
	PackageID        int64  `gorm:"column:package_id" json:"packageId"`
	CarrierID        int64  `gorm:"column:carrier_id" json:"carrierId"`
	ShippingMethodID int64  `gorm:"column:shipping_method_id" json:"shippingMethodId"`
	CarrierCode      string `gorm:"column:carrier_code" json:"carrierCode"`
	CarrierName      string `gorm:"column:carrier_name" json:"carrierName"`
	MethodCode       string `gorm:"column:method_code" json:"methodCode"`
	MethodName       string `gorm:"column:method_name" json:"methodName"`
}

func (PackageShipping) TableName() string { return "erp_package_shipping" }

type LogisticsAction struct {
	ERPRecord
	ActionID    int64  `gorm:"column:action_id;primaryKey;autoIncrement" json:"-"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	ActorID     int64  `gorm:"column:actor_id" json:"-"`
	ActionKind  string `gorm:"column:action_kind" json:"-"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (LogisticsAction) TableName() string { return "erp_logistics_action" }

type LogisticsWrite struct {
	RequestKey string `json:"requestKey"`
	Version    int64  `json:"version"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	CarrierID  int64  `json:"carrierId"`
	Status     string `json:"status"`
	Remark     string `json:"remark"`
}
type LogisticsResult struct {
	ID       int64  `json:"id"`
	Version  int64  `json:"version"`
	Kind     string `json:"kind"`
	Existing bool   `json:"existing"`
}
type LogisticsMasterView struct {
	ID          int64      `json:"id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	CarrierID   int64      `json:"carrierId"`
	CarrierCode string     `json:"carrierCode"`
	CarrierName string     `json:"carrierName"`
	Status      string     `json:"status"`
	Version     int64      `json:"version"`
	Remark      string     `json:"remark"`
	CreateTime  types.Time `json:"createTime"`
}
type LogisticsOption struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	CarrierID   int64  `json:"carrierId"`
	CarrierCode string `json:"carrierCode"`
	CarrierName string `json:"carrierName"`
}
type LogisticsQuery struct {
	Code      string `form:"code"`
	Name      string `form:"name"`
	Status    string `form:"status"`
	CarrierID int64  `form:"carrierId" binding:"min=0"`
}
type PackageShippingInput struct {
	ERPActionToken
	ShippingMethodID int64  `json:"shippingMethodId"`
	TrackingNo       string `json:"trackingNo"`
}
type TrackingCheckInput struct {
	PackageID        int64  `json:"packageId"`
	ShippingMethodID int64  `json:"shippingMethodId"`
	TrackingNo       string `json:"trackingNo"`
}
type TrackingCheck struct {
	Duplicate bool `json:"duplicate"`
}

type PackageShippingView struct {
	PackageID      int64            `json:"packageId"`
	PackageVersion int64            `json:"packageVersion"`
	Available      bool             `json:"available"`
	Snapshot       *PackageShipping `json:"snapshot"`
}
type ShipmentQuery struct {
	ShopID           int64  `form:"shopId" binding:"min=0"`
	WarehouseID      int64  `form:"warehouseId" binding:"min=0"`
	ShippingMethodID int64  `form:"shippingMethodId" binding:"min=0"`
	CarrierCode      string `form:"carrierCode"`
	TrackingNo       string `form:"trackingNo"`
	Mode             string `form:"mode"`
	ShippedFrom      string `form:"shippedFrom"`
	ShippedTo        string `form:"shippedTo"`
}
type ShipmentView struct {
	PackageID        int64      `json:"packageId"`
	PackageNo        string     `json:"packageNo" excel:"name:包裹号"`
	OrderID          int64      `json:"orderId"`
	ShopID           int64      `json:"shopId"`
	WarehouseID      int64      `json:"warehouseId"`
	ShippingMethodID *int64     `json:"shippingMethodId"`
	Mode             string     `json:"mode" excel:"name:物流录入方式"`
	CarrierCode      string     `json:"carrierCode" excel:"name:物流代码"`
	CarrierName      string     `json:"carrierName" excel:"name:物流公司快照"`
	MethodCode       string     `json:"methodCode" excel:"name:配送方式编码"`
	MethodName       string     `json:"methodName" excel:"name:配送方式快照"`
	TrackingNo       string     `json:"trackingNo" excel:"name:运单号"`
	ShippedAt        types.Time `json:"shippedAt" excel:"name:本地发货时间"`
}
