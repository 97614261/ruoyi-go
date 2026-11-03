package model

import "ruoyi-go/pkg/types"

type CMSWorkflowAudit struct {
	CreateBy   string     `json:"createBy"`
	CreateTime types.Time `json:"createTime"`
	UpdateBy   string     `json:"updateBy"`
	UpdateTime types.Time `json:"updateTime"`
	Remark     string     `json:"remark"`
}
type CMSCollaborationSetting struct {
	SettingId        int64   `gorm:"primaryKey" json:"-"`
	InquiryIsolation bool    `json:"inquiryIsolation"`
	ManagerIds       []int64 `gorm:"serializer:json" json:"managerIds"`
	Version          int64   `json:"version"`
	CMSWorkflowAudit
}

func (CMSCollaborationSetting) TableName() string { return "cms_collaboration_setting" }

type CMSReviewPolicy struct {
	Kind     string `gorm:"primaryKey" json:"kind"`
	Required bool   `json:"required"`
	Version  int64  `json:"version"`
	CMSWorkflowAudit
}

func (CMSReviewPolicy) TableName() string { return "cms_review_policy" }

type CMSWorkflowSettings struct {
	CMSCollaborationSetting
	Policies []CMSReviewPolicy `gorm:"-" json:"policies"`
}
type CMSWorkflowInput struct {
	Version          int64           `json:"version" binding:"required,gt=0"`
	InquiryIsolation bool            `json:"inquiryIsolation"`
	ManagerIds       []int64         `json:"managerIds" binding:"max=20,dive,gt=0"`
	Required         map[string]bool `json:"required" binding:"required"`
	Reason           string          `json:"reason" binding:"notblank,max=500"`
}
type CMSDraftActor struct {
	Kind         string `gorm:"primaryKey" json:"kind"`
	SnapshotKey  string `gorm:"primaryKey" json:"snapshotKey"`
	ObjectId     int64  `json:"objectId"`
	AuthorUserId int64  `json:"authorUserId"`
	CMSWorkflowAudit
}

func (CMSDraftActor) TableName() string { return "cms_draft_actor" }

type CMSReviewRequest struct {
	ReviewId        int64  `gorm:"primaryKey" json:"reviewId"`
	Kind            string `json:"kind"`
	ObjectId        int64  `json:"objectId"`
	SnapshotKey     string `json:"snapshotKey"`
	ContentVersion  int64  `json:"contentVersion"`
	SnapshotHash    string `json:"snapshotHash"`
	SnapshotJSON    string `gorm:"column:snapshot_json" json:"-"`
	AuthorUserId    int64  `json:"authorUserId"`
	SubmitterUserId int64  `json:"submitterUserId"`
	ReviewerUserId  int64  `json:"reviewerUserId"`
	PolicyVersion   int64  `json:"policyVersion"`
	Status          string `json:"status"`
	Opinion         string `json:"opinion"`
	Version         int64  `json:"version"`
	CMSWorkflowAudit
}

func (CMSReviewRequest) TableName() string { return "cms_review_request" }

type CMSReviewSnapshot struct {
	Content    *CMSContentDetail `json:"content,omitempty"`
	Operations *CMSContentOps    `json:"operations,omitempty"`
	Site       *SiteConfig       `json:"site,omitempty"`
}
type CMSReviewDetail struct {
	CMSReviewRequest
	Snapshot  CMSReviewSnapshot `json:"snapshot"`
	IsCurrent bool              `json:"isCurrent"`
	Events    []CMSReviewEvent  `json:"events"`
}
type CMSReviewEvent struct {
	EventId     int64  `gorm:"primaryKey" json:"eventId"`
	ReviewId    int64  `json:"reviewId"`
	Kind        string `json:"kind"`
	ObjectId    int64  `json:"objectId"`
	Action      string `json:"action"`
	ActorUserId int64  `json:"actorUserId"`
	Reason      string `json:"reason"`
	Details     string `json:"details"`
	CMSWorkflowAudit
}

func (CMSReviewEvent) TableName() string { return "cms_review_event" }

type CMSReviewInput struct {
	Kind           string `json:"kind" binding:"required,oneof=ARTICLE PRODUCT PAGE SITE"`
	ObjectId       int64  `json:"objectId" binding:"required,gt=0"`
	ContentVersion int64  `json:"contentVersion" binding:"required,gt=0"`
}
type CMSReviewDecision struct {
	Version int64  `json:"version" binding:"required,gt=0"`
	Action  string `json:"action" binding:"required,oneof=approve reject withdraw"`
	Opinion string `json:"opinion" binding:"max=1000"`
}
type CMSReviewQuery struct {
	Kind     string `form:"kind" binding:"omitempty,oneof=ARTICLE PRODUCT PAGE SITE"`
	Status   string `form:"status" binding:"omitempty,oneof=PENDING APPROVED REJECTED WITHDRAWN STALE PUBLISHED"`
	ObjectId int64  `form:"objectId" binding:"min=0"`
	Mine     bool   `form:"mine" json:"mine"`
}
type CMSWorkflowPerson struct {
	UserId   int64  `json:"userId"`
	UserName string `json:"userName"`
	NickName string `json:"nickName"`
}
type CMSPeopleQuery struct {
	Purpose string `form:"purpose" binding:"required,oneof=review inquiry"`
	Kind    string `form:"kind" binding:"omitempty,oneof=ARTICLE PRODUCT PAGE SITE"`
	Name    string `form:"name" binding:"max=60"`
}
type CMSInquiryTeam struct {
	InquiryId           int64      `gorm:"primaryKey" json:"inquiryId"`
	OwnerUserId         int64      `json:"ownerUserId"`
	NextFollowAt        types.Time `json:"nextFollowAt"`
	ReminderSentVersion int64      `json:"-"`
	Version             int64      `json:"version"`
	CMSWorkflowAudit
}

func (CMSInquiryTeam) TableName() string { return "cms_inquiry_team" }

type CMSInquiryTask struct {
	CMSInquirySummary
	OwnerUserId  int64      `json:"ownerUserId"`
	OwnerName    string     `json:"ownerName"`
	NextFollowAt types.Time `json:"nextFollowAt"`
}
type CMSInquiryFollowup struct {
	FollowupId  int64  `gorm:"primaryKey" json:"followupId"`
	InquiryId   int64  `json:"inquiryId"`
	ActorUserId int64  `json:"actorUserId"`
	EventType   string `json:"eventType"`
	Text        string `json:"text"`
	Result      string `json:"result"`
	IsErased    int    `json:"-"`
	CMSWorkflowAudit
}

func (CMSInquiryFollowup) TableName() string { return "cms_inquiry_followup" }

type CMSInquiryAssign struct {
	Version     int64 `json:"version" binding:"required,gt=0"`
	OwnerUserId int64 `json:"ownerUserId" binding:"min=0"`
}
type CMSInquiryFollowInput struct {
	Version      int64      `json:"version" binding:"required,gt=0"`
	Text         string     `json:"text" binding:"notblank,max=2000"`
	Result       string     `json:"result" binding:"max=200"`
	NextFollowAt string     `json:"nextFollowAt" binding:"max=19"`
	Due          types.Time `json:"-"`
}
type CMSInquiryTaskQuery struct {
	CMSInquiryQuery
	Due  string `form:"due" json:"due" binding:"omitempty,oneof=today overdue scheduled"`
	Mine bool   `form:"mine" json:"mine"`
}
type CMSNotificationOutbox struct {
	OutboxId        int64  `gorm:"primaryKey" json:"-"`
	EventKey        string `json:"-"`
	RecipientUserId int64  `json:"-"`
	ResourceType    string `json:"resourceType"`
	ResourceId      int64  `json:"resourceId"`
	Kind            string `json:"kind"`
	EventType       string `json:"eventType"`
	BusinessVersion int64  `json:"businessVersion"`
	Subject         string `json:"subject"`
	Status          string `json:"-"`
	Attempts        int    `json:"-"`
	CMSWorkflowAudit
}

func (CMSNotificationOutbox) TableName() string { return "cms_notification_outbox" }

type CMSNotification struct {
	TargetContentId int64      `gorm:"column:target_content_id;->;-:migration" json:"targetContentId,omitempty"`
	NotificationId  int64      `gorm:"primaryKey" json:"notificationId"`
	OutboxId        int64      `json:"-"`
	RecipientUserId int64      `json:"-"`
	ResourceType    string     `json:"resourceType"`
	ResourceId      int64      `json:"resourceId"`
	Kind            string     `json:"kind"`
	EventType       string     `json:"eventType"`
	BusinessVersion int64      `json:"businessVersion"`
	Subject         string     `json:"subject"`
	ReadAt          types.Time `json:"readAt"`
	ExpiresAt       types.Time `json:"expiresAt"`
	Version         int64      `json:"version"`
	CMSWorkflowAudit
}

func (CMSNotification) TableName() string { return "cms_notification" }

type CMSWorkbench struct {
	OwnDrafts        int64         `json:"ownDrafts"`
	PendingReview    int64         `json:"pendingReview"`
	Submitted        int64         `json:"submitted"`
	Approved         int64         `json:"approved"`
	Rejected         int64         `json:"rejected"`
	InquiryVisible   bool          `json:"inquiryVisible"`
	InquiryNew       int64         `json:"inquiryNew"`
	InquiryToday     int64         `json:"inquiryToday"`
	InquiryOverdue   int64         `json:"inquiryOverdue"`
	FailedSchedules  int64         `json:"failedSchedules"`
	AnalyticsVisible bool          `json:"analyticsVisible"`
	AnalyticsEnabled bool          `json:"analyticsEnabled"`
	Visits           []CMSVisitRow `json:"visits"`
}

type CMSWorkflowImpact struct {
	Kind      string `json:"kind"`
	Pending   int64  `json:"pending"`
	Approved  int64  `json:"approved"`
	Scheduled int64  `json:"scheduled"`
}
