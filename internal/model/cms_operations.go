package model

type CMSIssue struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	ContentId int64  `json:"contentId,omitempty"`
	AssetId   int64  `json:"assetId,omitempty"`
	URL       string `json:"url,omitempty"`
}
type CMSCheck struct {
	ContentId int64      `json:"contentId"`
	Version   int64      `json:"version"`
	Ready     bool       `json:"ready"`
	Issues    []CMSIssue `json:"issues"`
}
type CMSBatchItem struct {
	ContentId int64 `json:"contentId" binding:"required,gt=0"`
	Version   int64 `json:"version" binding:"required,gt=0"`
}
type CMSBatchInput struct {
	Action string         `json:"action" binding:"required,oneof=publish unpublish"`
	Items  []CMSBatchItem `json:"items" binding:"required,min=1,max=20,dive"`
}
type CMSBatchResult struct {
	ContentId int64  `json:"contentId"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
}
type CMSInspectionRow struct {
	Kind        string
	Slug        string
	Version     int64
	CMSRevision `gorm:"embedded"`
}
type CMSInspection struct {
	Rows                 []CMSCheck `json:"rows"`
	Total                int64      `json:"total"`
	Issues               []CMSIssue `json:"issues"`
	ExternalLinksSkipped int        `json:"externalLinksSkipped"`
	Scope                string     `json:"scope"`
}
