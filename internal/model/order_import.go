package model

import "ruoyi-go/pkg/types"

const ERPImportFilePrefix = "erp-import:"

type ERPImportJob struct {
	ERPRecord
	JobID          int64      `gorm:"column:job_id;primaryKey;autoIncrement" json:"jobId"`
	ShopID         int64      `gorm:"column:shop_id" json:"shopId"`
	ActorUserID    int64      `gorm:"column:actor_user_id" json:"actorUserId"`
	RequestKey     string     `gorm:"column:request_key" json:"-"`
	RequestHash    string     `gorm:"column:request_hash" json:"-"`
	Status         string     `gorm:"column:status" json:"status"`
	Version        int64      `gorm:"column:version" json:"version"`
	Attempt        int64      `gorm:"column:attempt" json:"attempt"`
	FileKey        string     `gorm:"column:file_key" json:"-"`
	FileSha256     string     `gorm:"column:file_sha256" json:"-"`
	FileBytes      int64      `gorm:"column:file_bytes" json:"fileBytes"`
	FileState      string     `gorm:"column:file_state" json:"fileState"`
	RetainUntil    types.Time `gorm:"column:retain_until" json:"retainUntil"`
	TotalRows      int64      `gorm:"column:total_rows" json:"totalRows"`
	ProcessedRows  int64      `gorm:"-" json:"processedRows"`
	CreatedOrders  int64      `gorm:"column:created_orders" json:"createdOrders"`
	ExistingOrders int64      `gorm:"column:existing_orders" json:"existingOrders"`
	FailedOrders   int64      `gorm:"column:failed_orders" json:"failedOrders"`
	StartedAt      types.Time `gorm:"column:started_at" json:"startedAt"`
	FinishedAt     types.Time `gorm:"column:finished_at" json:"finishedAt"`
}

func (ERPImportJob) TableName() string { return "erp_import_job" }

type ERPImportResult struct {
	ERPRecord
	ResultID      int64  `gorm:"column:result_id;primaryKey;autoIncrement" json:"resultId"`
	ShopID        int64  `gorm:"column:shop_id" json:"shopId"`
	JobID         int64  `gorm:"column:job_id" json:"jobId"`
	SourceRow     int64  `gorm:"column:source_row" json:"sourceRow"`
	SourceOrderNo string `gorm:"column:source_order_no" json:"sourceOrderNo"`
	InputHash     string `gorm:"column:input_hash" json:"-"`
	ResultCode    string `gorm:"column:result_code" json:"resultCode"`
	ErrorSummary  string `gorm:"column:error_summary" json:"errorSummary"`
	OrderID       *int64 `gorm:"column:order_id" json:"orderId"`
	Attempt       int64  `gorm:"column:attempt" json:"attempt"`
}

func (ERPImportResult) TableName() string { return "erp_import_result" }

type ERPImportQuery struct {
	ShopID int64  `form:"shopId" binding:"min=0"`
	Status string `form:"status"`
}
type ERPImportRetry struct {
	Version    int64  `json:"version"`
	RequestKey string `json:"requestKey"`
}
