package model

// ErpShopItem originated from the erp_shop_item generator output; domain
// writes use an explicit DTO and persistent item version, not generic deletion.
type ErpShopItem struct {
	ERPRecord
	ItemID            int64   `gorm:"column:item_id;primaryKey;autoIncrement" json:"itemId"`
	ShopID            int64   `gorm:"column:shop_id" json:"shopId"`
	ShopItemCode      string  `gorm:"column:shop_item_code" json:"shopItemCode"`
	ExternalProductID *string `gorm:"column:external_product_id" json:"externalProductId"`
	ExternalVariantID *string `gorm:"column:external_variant_id" json:"externalVariantId"`
	Title             string  `gorm:"column:title" json:"title"`
	Status            string  `gorm:"column:status" json:"status"`
	Version           int64   `gorm:"column:version" json:"version"`
}

func (ErpShopItem) TableName() string { return "erp_shop_item" }

type ERPShopSkuMapping struct {
	ERPRecord
	MappingID  int64  `gorm:"column:mapping_id;primaryKey;autoIncrement" json:"mappingId"`
	ShopID     int64  `gorm:"column:shop_id" json:"shopId"`
	ShopItemID int64  `gorm:"column:shop_item_id" json:"shopItemId"`
	SkuID      int64  `gorm:"column:sku_id" json:"skuId"`
	Status     string `gorm:"column:status" json:"status"`
	Version    int64  `gorm:"column:version" json:"version"`
}

func (ERPShopSkuMapping) TableName() string { return "erp_shop_sku_mapping" }

type ERPShopItemView struct {
	ErpShopItem
	ShopCode       string  `json:"shopCode"`
	ShopName       string  `json:"shopName"`
	ShopStatus     string  `json:"shopStatus"`
	MappingID      *int64  `json:"mappingId"`
	MappingVersion *int64  `json:"mappingVersion"`
	MappingStatus  *string `json:"mappingStatus"`
	SkuID          *int64  `json:"skuId"`
	SkuCode        string  `json:"skuCode"`
	ProductName    string  `json:"productName"`
	Unit           string  `json:"unit"`
	SkuStatus      *string `json:"skuStatus"`
}
type ERPShopItemWrite struct {
	ShopID            int64   `json:"shopId" binding:"required,min=1"`
	ShopItemCode      string  `json:"shopItemCode" binding:"required,max=128"`
	ExternalProductID *string `json:"externalProductId" binding:"omitempty,max=128"`
	ExternalVariantID *string `json:"externalVariantId" binding:"omitempty,max=128"`
	Title             string  `json:"title" binding:"required,max=200"`
	Status            string  `json:"status" binding:"required,oneof=0 1"`
	Version           int64   `json:"version"`
	Remark            string  `json:"remark" binding:"max=500"`
}
type ERPShopItemQuery struct {
	ShopID       int64  `form:"shopId" binding:"min=0"`
	ShopItemCode string `form:"shopItemCode" binding:"max=128"`
	Title        string `form:"title" binding:"max=200"`
	Status       string `form:"status" binding:"omitempty,oneof=0 1"`
	MappingState string `form:"mappingState" binding:"omitempty,oneof=MAPPED UNMAPPED"`
}
type ERPMappingWrite struct {
	Version int64  `json:"version" binding:"required,min=1"` // item version, not mapping version
	SkuID   int64  `json:"skuId" binding:"required,min=1"`
	Remark  string `json:"remark" binding:"max=500"`
}
type ERPMappingResult struct {
	ItemID         int64  `json:"itemId"`
	Version        int64  `json:"version"`
	MappingID      int64  `json:"mappingId"`
	MappingVersion int64  `json:"mappingVersion"`
	MappingStatus  string `json:"mappingStatus"`
}

var ERPShopItemSortColumns = map[string]string{"itemId": "erp_shop_item.item_id", "shopItemCode": "erp_shop_item.shop_item_code", "title": "erp_shop_item.title", "status": "erp_shop_item.status"}
