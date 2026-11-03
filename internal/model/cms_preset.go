package model

type CMSPresetPage struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	BodyHTML string `json:"bodyHtml"`
	Slug     string `json:"slug"`
}
type CMSPreset struct {
	Id          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     int             `json:"version"`
	Theme       string          `json:"theme"`
	Palette     string          `json:"palette"`
	Blocks      []SiteBlock     `json:"blocks"`
	Pages       []CMSPresetPage `json:"pages"`
}
type CMSPresetInput struct {
	PresetId    string   `json:"presetId" binding:"required,max=32"`
	SiteVersion int64    `json:"siteVersion" binding:"required,gt=0"`
	SlugPrefix  string   `json:"slugPrefix" binding:"required,max=40"`
	PageKeys    []string `json:"pageKeys" binding:"max=3,dive,oneof=about services contact"`
}
type CMSPresetApply struct {
	CMSPresetInput
	PlanSHA256 string `json:"planSha256" binding:"required,len=64"`
	RequestId  string `json:"requestId" binding:"required,uuid4"`
	Reason     string `json:"reason" binding:"notblank,max=300"`
}
type CMSPresetPlan struct {
	CMSPresetInput
	PresetVersion int             `json:"presetVersion"`
	Config        SiteConfig      `json:"config"`
	Pages         []CMSPresetPage `json:"pages"`
	Conflicts     []string        `json:"conflicts"`
	Warnings      []string        `json:"warnings"`
	PlanSHA256    string          `json:"planSha256"`
}
type CMSPresetApplication struct {
	ApplicationId     int64   `gorm:"primaryKey" json:"applicationId"`
	ActorUserId       int64   `json:"actorUserId"`
	RequestId         string  `json:"requestId"`
	RequestSHA256     string  `gorm:"column:request_sha256" json:"-"`
	PresetId          string  `json:"presetId"`
	PresetVersion     int     `json:"presetVersion"`
	SiteVersionBefore int64   `json:"siteVersionBefore"`
	SiteVersionAfter  int64   `json:"siteVersionAfter"`
	PageIds           []int64 `gorm:"serializer:json" json:"pageIds"`
	BeforeConfigJSON  string  `gorm:"column:before_config_json" json:"-"`
	CMSWorkflowAudit
}

func (CMSPresetApplication) TableName() string { return "cms_preset_application" }
