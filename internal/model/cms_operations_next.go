package model

import "ruoyi-go/pkg/types"

type CMSVersionRef struct {
	Source  string `json:"source" binding:"required,oneof=DRAFT REVISION RELEASE REVIEW"`
	Id      int64  `json:"id" binding:"min=0"`
	Version int64  `json:"version" binding:"min=0"`
}
type CMSCompareInput struct {
	Kind     string        `json:"kind" binding:"required,oneof=ARTICLE PRODUCT PAGE SITE"`
	ObjectId int64         `json:"objectId" binding:"required,gt=0"`
	Left     CMSVersionRef `json:"left" binding:"required"`
	Right    CMSVersionRef `json:"right" binding:"required"`
}
type CMSCompareSide struct {
	Label    string            `json:"label"`
	Key      string            `json:"key"`
	Snapshot CMSReviewSnapshot `json:"snapshot"`
}
type CMSDiffPart struct {
	Text string `json:"text"`
	Op   string `json:"op"`
}
type CMSDiffField struct {
	Path    string        `json:"path"`
	Label   string        `json:"label"`
	Left    string        `json:"left"`
	Right   string        `json:"right"`
	Changed bool          `json:"changed"`
	Coarse  bool          `json:"coarse"`
	Parts   []CMSDiffPart `json:"parts"`
}
type CMSComparison struct {
	Notices      []string       `json:"notices"`
	LeftLabel    string         `json:"leftLabel"`
	RightLabel   string         `json:"rightLabel"`
	ChangedCount int            `json:"changedCount"`
	Fields       []CMSDiffField `json:"fields"`
}
type CMSInquiryFilter struct {
	CMSInquiryTaskQuery
	From        string `form:"from" json:"from" binding:"omitempty,len=10"`
	To          string `form:"to" json:"to" binding:"omitempty,len=10"`
	OwnerUserId *int64 `form:"ownerUserId" json:"ownerUserId" binding:"omitempty,min=0"`
}
type CMSInquiryExport struct {
	CMSInquiryFilter
	IncludePrivate bool `json:"includePrivate"`
}
type CMSInquiryExportRow struct {
	InquiryId    int64      `json:"inquiryId" excel:"name:编号;cell:text;sort:1"`
	Name         string     `json:"name" excel:"name:称呼;cell:text;sort:2"`
	Contact      string     `json:"contact" excel:"name:联系方式;cell:text;sort:3"`
	Status       string     `json:"status" excel:"name:状态;sort:4;converter:NEW=待处理,DONE=已处理,SPAM=垃圾留言"`
	OwnerName    string     `json:"ownerName" excel:"name:负责人;cell:text;sort:5"`
	CreateTime   types.Time `json:"createTime" excel:"name:提交时间;sort:6"`
	NextFollowAt types.Time `json:"nextFollowAt" excel:"name:下次跟进;sort:7"`
	Message      string     `json:"message" excel:"name:留言正文（敏感授权后填写）;cell:text;sort:8"`
	Note         string     `json:"note" excel:"name:处理备注（敏感授权后填写）;cell:text;sort:9"`
}
type CMSDataPolicy struct {
	PolicyId      int64    `gorm:"primaryKey" json:"-"`
	RetentionDays int      `json:"retentionDays"`
	Statuses      []string `gorm:"serializer:json" json:"statuses"`
	Version       int64    `json:"version"`
	CMSWorkflowAudit
}

func (CMSDataPolicy) TableName() string { return "cms_data_policy" }

type CMSDataPolicyInput struct {
	Version       int64    `json:"version" binding:"required,gt=0"`
	RetentionDays int      `json:"retentionDays" binding:"min=0,max=3650"`
	Statuses      []string `json:"statuses" binding:"required,min=1,max=3,dive,oneof=NEW DONE SPAM"`
	Reason        string   `json:"reason" binding:"notblank,max=300"`
}
type CMSCleanupInput struct {
	Reason     string  `json:"reason" binding:"notblank,max=300"`
	InquiryIds []int64 `json:"inquiryIds" binding:"max=100,dive,gt=0"`
}
type CMSCleanupExecute struct {
	Version      int64  `json:"version" binding:"required,gt=0"`
	Confirmation string `json:"confirmation" binding:"required,eq=确认清除"`
}
type CMSCleanupBatch struct {
	BatchId       int64      `gorm:"primaryKey" json:"batchId"`
	ActorUserId   int64      `json:"actorUserId"`
	PolicyVersion int64      `json:"policyVersion"`
	Cutoff        types.Time `json:"cutoff"`
	Statuses      []string   `gorm:"serializer:json" json:"statuses"`
	TotalEligible int64      `json:"totalEligible"`
	PreviewCount  int        `json:"previewCount"`
	ErasedCount   int        `json:"erasedCount"`
	ExpiresAt     types.Time `json:"expiresAt"`
	Status        string     `json:"status"`
	Version       int64      `json:"version"`
	Reason        string     `json:"reason"`
	CMSWorkflowAudit
}

func (CMSCleanupBatch) TableName() string { return "cms_data_cleanup_batch" }

type CMSCleanupItem struct {
	BatchId       int64 `gorm:"primaryKey" json:"-"`
	InquiryId     int64 `gorm:"primaryKey" json:"inquiryId"`
	SourceVersion int64 `json:"sourceVersion"`
	CMSWorkflowAudit
}

func (CMSCleanupItem) TableName() string { return "cms_data_cleanup_item" }

type CMSCleanupItemView struct {
	InquiryId      int64      `json:"inquiryId"`
	SourceVersion  int64      `json:"sourceVersion"`
	CurrentVersion int64      `json:"currentVersion"`
	Name           string     `json:"name"`
	Contact        string     `json:"contact"`
	Status         string     `json:"status"`
	ActiveAt       types.Time `json:"activeAt"`
}
type CMSDataEvent struct {
	EventId     int64  `gorm:"primaryKey" json:"eventId"`
	ActorUserId int64  `json:"actorUserId"`
	Action      string `json:"action"`
	ResourceId  int64  `json:"resourceId"`
	RowCount    int64  `json:"rowCount"`
	Details     string `json:"details"`
	FileSHA256  string `gorm:"column:file_sha256" json:"fileSha256"`
	CMSWorkflowAudit
}

func (CMSDataEvent) TableName() string { return "cms_data_event" }

type CMSInquiryClock struct {
	InquiryId     int64      `gorm:"primaryKey" json:"inquiryId"`
	SubmittedAt   types.Time `json:"submittedAt"`
	ActiveAt      types.Time `json:"activeAt"`
	Status        string     `json:"status"`
	IsErased      bool       `json:"-"`
	SourceVersion int64      `json:"sourceVersion"`
	CMSWorkflowAudit
}

func (CMSInquiryClock) TableName() string { return "cms_inquiry_clock" }

type CMSSearchDocument struct {
	ContentId   int64  `gorm:"primaryKey" json:"contentId"`
	RevisionId  int64  `json:"revisionId"`
	TitleText   string `json:"-"`
	SummaryText string `json:"-"`
	BodyText    string `json:"-"`
	SearchText  string `json:"-"`
	CMSWorkflowAudit
}

func (CMSSearchDocument) TableName() string { return "cms_search_document" }

type CMSSearchPosting struct {
	TokenHash []byte `gorm:"primaryKey" json:"-"`
	ContentId int64  `gorm:"primaryKey" json:"-"`
	Weight    int    `json:"-"`
	CMSWorkflowAudit
}

func (CMSSearchPosting) TableName() string { return "cms_search_posting" }

type CMSSearchTask struct {
	ContentId         int64  `gorm:"primaryKey" json:"contentId"`
	DesiredRevisionId *int64 `json:"desiredRevisionId"`
	Status            string `json:"status"`
	Attempts          int    `json:"attempts"`
	ErrorMessage      string `json:"errorMessage"`
	Version           int64  `json:"version"`
	CMSWorkflowAudit
}

func (CMSSearchTask) TableName() string { return "cms_search_task" }

type CMSSearchTaskView struct {
	CMSSearchTask
	Kind  string `json:"kind"`
	Title string `json:"title"`
}
type CMSSearchJob struct {
	CMSSearchTask
	IsPublished         int
	IsDeleted           int
	PublishedRevisionId *int64
	Title               string
	Summary             string
	BodyHTML            string `gorm:"column:body_html"`
	ProductParams       string
	ProductCode         string
}
type CMSSearchBuilt struct {
	Job      CMSSearchJob
	Doc      CMSSearchDocument
	Postings []CMSSearchPosting
	Error    string
}
type CMSSearchOverview struct {
	Ready       int64 `json:"ready"`
	Pending     int64 `json:"pending"`
	Failed      int64 `json:"failed"`
	MaxAttempts int   `json:"maxAttempts"`
}
type CMSSearchQuery struct {
	Status string `form:"status" binding:"omitempty,oneof=PENDING READY FAILED"`
}
type PortalSearchRow struct {
	PortalCard
	BodyText string
	Score    int64
}
type PortalTextPart struct {
	Text  string
	Match bool
}
