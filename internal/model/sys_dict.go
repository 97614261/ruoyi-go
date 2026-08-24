package model

import "ruoyi-go/pkg/types"

// SysDictType 字典类型表 sys_dict_type。
type SysDictType struct {
	DictID   int64  `gorm:"column:dict_id;primaryKey;autoIncrement" json:"dictId" excel:"name:字典主键;cell:numeric"`
	DictName string `gorm:"column:dict_name" json:"dictName" binding:"notblank,max=100" excel:"name:字典名称"`
	// dicttype 规则对应 Java 的 @Pattern(regexp = "^[a-z][a-z0-9_]*$")
	DictType   string     `gorm:"column:dict_type" json:"dictType" binding:"notblank,max=100,dicttype" excel:"name:字典类型"`
	Status     string     `gorm:"column:status" json:"status" excel:"name:状态;converter:0=正常,1=停用"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark     *string    `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`
}

func (SysDictType) TableName() string { return "sys_dict_type" }

// SysDictData 字典数据表 sys_dict_data。
type SysDictData struct {
	DictCode int64 `gorm:"column:dict_code;primaryKey;autoIncrement" json:"dictCode" excel:"name:字典编码;cell:numeric"`
	// 指针 + required，理由详见 SysPost.PostSort 的注释。
	// 【注意 Java 这里没有 @NotNull】SysDictData.dictSort 是裸 Integer，
	// 不传时 Java 会存 NULL。我们要求必填属于**更严**，已在 CONVENTIONS 登记：
	// 前端表单本就是 :min="0" 且总会带值，而 NULL 排序会让列表顺序变得不可预期。
	DictSort  *int   `gorm:"column:dict_sort" json:"dictSort" binding:"required,min=0" excel:"name:字典排序;cell:numeric"`
	DictLabel string `gorm:"column:dict_label" json:"dictLabel" binding:"notblank,max=100" excel:"name:字典标签"`
	DictValue string `gorm:"column:dict_value" json:"dictValue" binding:"notblank,max=100" excel:"name:字典键值"`
	DictType  string `gorm:"column:dict_type" json:"dictType" binding:"notblank,max=100" excel:"name:字典类型"`
	// CssClass / ListClass 是可选的样式字段
	CssClass   *string    `gorm:"column:css_class" json:"cssClass" binding:"omitempty,max=100"`
	ListClass  *string    `gorm:"column:list_class" json:"listClass" binding:"omitempty,max=100"`
	IsDefault  string     `gorm:"column:is_default" json:"isDefault" excel:"name:是否默认;converter:Y=是,N=否"`
	Status     string     `gorm:"column:status" json:"status" excel:"name:状态;converter:0=正常,1=停用"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark     *string    `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// Default 非表字段。Java 版 SysDictData.getDefault() 是 getter，
	// Jackson 会序列化成 "default"，这里手工补出来。
	Default bool `gorm:"-" json:"default"`
}

func (SysDictData) TableName() string { return "sys_dict_data" }

// IsDefaultDict is_default 是否为 Y。
func (d SysDictData) IsDefaultDict() bool { return d.IsDefault == "Y" }

// DictTypeQuery 字典类型列表的查询条件。
type DictTypeQuery struct {
	DictName  string `form:"dictName"`
	DictType  string `form:"dictType"`
	Status    string `form:"status"`
	BeginTime string `form:"params[beginTime]"`
	EndTime   string `form:"params[endTime]"`
}

// DictDataQuery 字典数据列表的查询条件。
type DictDataQuery struct {
	DictType  string `form:"dictType"`
	DictLabel string `form:"dictLabel"`
	Status    string `form:"status"`
}

// DictTypeSortColumns 字典类型允许排序的字段白名单。
var DictTypeSortColumns = map[string]string{
	"dictId":     "dict_id",
	"dictName":   "dict_name",
	"dictType":   "dict_type",
	"status":     "status",
	"createTime": "create_time",
}

// DictDataSortColumns 字典数据允许排序的字段白名单。
var DictDataSortColumns = map[string]string{
	"dictCode":   "dict_code",
	"dictSort":   "dict_sort",
	"dictLabel":  "dict_label",
	"dictValue":  "dict_value",
	"status":     "status",
	"createTime": "create_time",
}
