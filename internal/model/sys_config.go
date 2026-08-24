package model

import "ruoyi-go/pkg/types"

// 参数是否系统内置。
const (
	ConfigTypeBuiltin = "Y"
	ConfigTypeCustom  = "N"
)

// SysConfig 参数配置表 sys_config。
type SysConfig struct {
	ConfigID    int64  `gorm:"column:config_id;primaryKey;autoIncrement" json:"configId" excel:"name:参数主键;cell:numeric"`
	ConfigName  string `gorm:"column:config_name" json:"configName" binding:"notblank,max=100" excel:"name:参数名称"`
	ConfigKey   string `gorm:"column:config_key" json:"configKey" binding:"notblank,max=100" excel:"name:参数键名"`
	ConfigValue string `gorm:"column:config_value" json:"configValue" binding:"notblank,max=500" excel:"name:参数键值"`
	// ConfigType 内置参数不允许删除
	ConfigType string     `gorm:"column:config_type" json:"configType" excel:"name:系统内置;converter:Y=是,N=否"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark     *string    `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`
}

func (SysConfig) TableName() string { return "sys_config" }

// ConfigQuery 参数列表的查询条件。
type ConfigQuery struct {
	ConfigName string `form:"configName"`
	ConfigKey  string `form:"configKey"`
	ConfigType string `form:"configType"`
	BeginTime  string `form:"params[beginTime]"`
	EndTime    string `form:"params[endTime]"`
}

// ConfigSortColumns 允许排序的字段白名单。
var ConfigSortColumns = map[string]string{
	"configId":   "config_id",
	"configName": "config_name",
	"configKey":  "config_key",
	"configType": "config_type",
	"createTime": "create_time",
}
