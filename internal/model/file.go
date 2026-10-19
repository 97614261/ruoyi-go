package model

import "ruoyi-go/pkg/types"

type SysFile struct {
	TenantRecord
	FileID         int64      `gorm:"column:file_id;primaryKey;autoIncrement" json:"fileId"`
	StorageKey     string     `gorm:"column:storage_key" json:"storageKey"`
	OriginalName   string     `gorm:"column:original_name" json:"originalName"`
	SizeBytes      int64      `gorm:"column:size_bytes" json:"sizeBytes"`
	ContentType    string     `gorm:"column:content_type" json:"contentType"`
	UploaderUserID int64      `gorm:"column:uploader_user_id" json:"uploaderUserId"`
	CreateBy       string     `gorm:"column:create_by" json:"createBy"`
	CreateTime     types.Time `gorm:"column:create_time" json:"createTime"`
}

func (SysFile) TableName() string { return "sys_file" }
