package model

import "ruoyi-go/pkg/types"

type ERPOrderWorkbench struct {
	Total         int64 `json:"total"`
	PendingReview int64 `json:"pendingReview"`
	ReadyToShip   int64 `json:"readyToShip"`
	Shipped       int64 `json:"shipped"`
	Cancelled     int64 `json:"cancelled"`
}
type ERPReviewIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ERPReviewStock struct {
	SkuID       int64  `json:"skuId"`
	SkuCode     string `json:"skuCode"`
	ProductName string `json:"productName"`
	Unit        string `json:"unit"`
	Required    int64  `json:"required"`
	Available   int64  `json:"available"`
	Shortage    int64  `json:"shortage"`
}

// A diagnostic, not a reservation or a promise that a later review succeeds.
// Never embed ERPOrder: review permission does not grant recipient query access.
type ERPOrderReadiness struct {
	OrderID     int64            `json:"orderId"`
	Version     int64            `json:"version"`
	Status      string           `json:"status"`
	WarehouseID int64            `json:"warehouseId"`
	CheckedAt   types.Time       `json:"checkedAt"`
	Ready       bool             `json:"ready"`
	Issues      []ERPReviewIssue `json:"issues"`
	Stocks      []ERPReviewStock `json:"stocks"`
}
