package model

type CMSTransfer struct {
	Format string              `json:"format"`
	Kind   string              `json:"kind"`
	Rows   []CMSDraftInput     `json:"rows"`
	Source []CMSTransferSource `json:"source,omitempty"`
}
type CMSTransferSource struct {
	ContentId int64 `json:"contentId"`
	Version   int64 `json:"version"`
}
type CMSTransferRow struct {
	Row       int    `json:"row"`
	Slug      string `json:"slug"`
	Ready     bool   `json:"ready"`
	Success   bool   `json:"success"`
	ContentId int64  `json:"contentId"`
	Message   string `json:"message"`
}
type CMSTransferReport struct {
	CheckOnly    bool             `json:"checkOnly"`
	Rows         []CMSTransferRow `json:"rows"`
	SuccessCount int              `json:"successCount"`
	FailureCount int              `json:"failureCount"`
}
