package model

import "ruoyi-go/pkg/types"

type CMSRecovery struct {
	RecoveryId  int64      `gorm:"primaryKey" json:"-"`
	UserId      int64      `json:"-"`
	Kind        string     `json:"-"`
	ContentId   int64      `json:"-"`
	BaseVersion int64      `json:"-"`
	Payload     string     `json:"-"`
	Version     int64      `json:"-"`
	ExpiresAt   types.Time `json:"-"`
	CreateBy    string     `json:"-"`
	CreateTime  types.Time `json:"-"`
	UpdateBy    string     `json:"-"`
	UpdateTime  types.Time `json:"-"`
	Remark      string     `json:"-"`
}

func (CMSRecovery) TableName() string { return "cms_content_recovery" }

// Partial input deliberately omits full-draft required fields. Full validation runs on commit.
type CMSRecoveryDraft struct {
	ProductCode    string  `json:"productCode" binding:"xss,max=80"`
	ProductParams  string  `json:"productParams" binding:"xss,max=2000"`
	GalleryIds     []int64 `json:"galleryIds" binding:"max=16,dive,gt=0"`
	Slug           string  `json:"slug" binding:"xss,max=80"`
	ChannelId      int64   `json:"channelId" binding:"min=0"`
	Title          string  `json:"title" binding:"xss,max=120"`
	Summary        string  `json:"summary" binding:"xss,max=500"`
	BodyHTML       string  `json:"bodyHtml"`
	Author         string  `json:"author" binding:"xss,max=60"`
	CoverAssetId   int64   `json:"coverAssetId" binding:"min=0"`
	CoverAltText   string  `json:"coverAltText" binding:"xss,max=200"`
	SeoTitle       string  `json:"seoTitle" binding:"xss,max=150"`
	SeoDescription string  `json:"seoDescription" binding:"xss,max=300"`
	Remark         string  `json:"remark" binding:"xss,max=500"`
}
type CMSRecoveryToken struct {
	SlotId  int64 `json:"slotId" binding:"min=0"`
	Version int64 `json:"version" binding:"min=0"`
}
type CMSRecoveryInput struct {
	CMSRecoveryToken
	BaseVersion int64            `json:"baseVersion" binding:"min=0"`
	Draft       CMSRecoveryDraft `json:"draft"`
}
type CMSRecoveryCommit struct {
	CMSRecoveryToken
	BaseVersion int64         `json:"baseVersion" binding:"min=0"`
	Draft       CMSDraftInput `json:"draft"`
}
type CMSRecoveryView struct {
	CMSRecoveryToken
	Found          bool              `json:"found"`
	BaseVersion    int64             `json:"baseVersion"`
	ContentVersion int64             `json:"contentVersion"`
	BaseMatches    bool              `json:"baseMatches"`
	SavedAt        types.Time        `json:"savedAt"`
	ExpiresAt      types.Time        `json:"expiresAt"`
	Draft          *CMSRecoveryDraft `json:"draft,omitempty"`
}
type CMSRecoveryCommitted struct {
	ContentId int64 `json:"contentId"`
	Version   int64 `json:"version"`
}
