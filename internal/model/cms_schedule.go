package model

import "ruoyi-go/pkg/types"

type CMSSchedule struct {
	ScheduleId     int64      `gorm:"primaryKey" json:"scheduleId"`
	ContentId      int64      `json:"contentId"`
	Kind           string     `json:"kind"`
	Action         string     `json:"action"`
	ContentVersion int64      `json:"contentVersion"`
	OwnerUserId    int64      `json:"ownerUserId"`
	DueAt          types.Time `json:"dueAt"`
	Status         string     `json:"status"`
	ResultMessage  string     `json:"resultMessage"`
	FinishedAt     types.Time `json:"finishedAt"`
	Version        int64      `json:"version"`
	CreateBy       string     `json:"createBy"`
	CreateTime     types.Time `json:"createTime"`
	UpdateBy       string     `json:"updateBy"`
	UpdateTime     types.Time `json:"updateTime"`
	Remark         string     `json:"remark"`
}

func (CMSSchedule) TableName() string { return "cms_schedule" }

type CMSScheduleInput struct {
	ContentId      int64  `json:"contentId" binding:"required,gt=0"`
	ContentVersion int64  `json:"contentVersion" binding:"required,gt=0"`
	Action         string `json:"action" binding:"required,oneof=publish unpublish"`
	DueAt          string `json:"dueAt" binding:"required,max=19"`
}
type CMSScheduleQuery struct {
	ContentId int64  `form:"contentId" binding:"min=0"`
	Status    string `form:"status" binding:"omitempty,oneof=PENDING SUCCESS FAILED CANCELED"`
}
