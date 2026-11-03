package model

// Recommendations are live views of 004/007, never independent stock facts.
type ERPReplenishmentQuery struct {
	WarehouseID int64 `form:"warehouseId" binding:"required,min=1"`
	SkuID       int64 `form:"skuId" binding:"min=0"`
}
type ERPReplenishment struct {
	AlertID           int64  `json:"-"`
	WarehouseID       int64  `json:"warehouseId"`
	WarehouseName     string `json:"warehouseName"`
	SkuID             int64  `json:"skuId"`
	SkuCode           string `json:"skuCode"`
	ProductName       string `json:"productName"`
	Unit              string `json:"unit"`
	OnHand            int64  `json:"onHand"`
	Reserved          int64  `json:"reserved"`
	Available         int64  `gorm:"-" json:"available"`
	ThresholdQuantity int64  `json:"thresholdQuantity"`
	AlertVersion      int64  `json:"-"`
	TargetQuantity    int64  `gorm:"-" json:"targetQuantity"`
	DraftQuantity     int64  `gorm:"-" json:"draftQuantity"`
	IncomingQuantity  int64  `gorm:"-" json:"incomingQuantity"`
	NeededQuantity    int64  `gorm:"-" json:"neededQuantity"`
	SuggestedQuantity int64  `gorm:"-" json:"suggestedQuantity"`
	Basis             string `gorm:"-" json:"basis"`
}
type ERPReplenishmentBasis struct {
	SkuID int64  `json:"skuId" binding:"min=1"`
	Basis string `json:"basis" binding:"required,len=64"`
}
type ERPReplenishmentWrite struct {
	ERPPurchaseWrite
	Basis []ERPReplenishmentBasis `json:"basis" binding:"required,min=1,max=100,dive"`
}
