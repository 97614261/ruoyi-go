package model

import "ruoyi-go/pkg/types"

type OverviewStamp struct {
	Available bool       `json:"available"`
	CheckedAt types.Time `json:"checkedAt"`
}
type OverviewOrders struct {
	OverviewStamp
	Counts *ERPOrderWorkbench `json:"counts"`
}
type OverviewShipmentCounts struct {
	Today     int64 `json:"today"`
	SevenDays int64 `json:"sevenDays"`
}
type OverviewShipments struct {
	OverviewStamp
	Counts        *OverviewShipmentCounts `json:"counts"`
	TodayFrom     string                  `json:"todayFrom"`
	SevenDaysFrom string                  `json:"sevenDaysFrom"`
	Through       string                  `json:"through"`
}
type OverviewStockCounts struct {
	WithStock    int64  `json:"withStock"`
	WithReserved int64  `json:"withReserved"`
	Low          *int64 `json:"low"`
}
type OverviewStocks struct {
	OverviewStamp
	Counts *OverviewStockCounts `json:"counts"`
}
type OverviewPurchases struct {
	OverviewStamp
	AwaitingReceipt *int64 `json:"awaitingReceipt"`
}
type OverviewMoney struct {
	Currency     string `json:"currency"`
	Documents    int64  `json:"documents"`
	DueAmount    string `json:"dueAmount"`
	ExcessAmount string `json:"excessAmount"`
}
type OverviewMoneyPage struct {
	OverviewStamp
	Rows  []OverviewMoney `json:"rows"`
	Total int64           `json:"total"`
}
type OverviewMoneyQuery struct {
	Currency string `form:"currency"`
}
