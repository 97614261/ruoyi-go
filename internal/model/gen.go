package model

import (
	"encoding/json"
	"strings"

	"ruoyi-go/pkg/types"
)

// GenTable 对应代码生成业务表 gen_table。
//
// 这些字段名和类型保持 Java/Vue3 的既有契约；生成出的后端代码则使用
// 当前项目的 Go 分层，而不是照搬 Java 包结构。
type GenTable struct {
	TableID        int64            `gorm:"column:table_id;primaryKey;autoIncrement" json:"tableId"`
	TableName      string           `gorm:"column:table_name" json:"tableName" binding:"notblank,max=200"`
	TableComment   string           `gorm:"column:table_comment" json:"tableComment" binding:"notblank,max=500"`
	SubTableName   *string          `gorm:"column:sub_table_name" json:"subTableName"`
	SubTableFKName *string          `gorm:"column:sub_table_fk_name" json:"subTableFkName"`
	ClassName      string           `gorm:"column:class_name" json:"className" binding:"notblank,max=100"`
	TplCategory    string           `gorm:"column:tpl_category" json:"tplCategory"`
	TplWebType     string           `gorm:"column:tpl_web_type" json:"tplWebType"`
	PackageName    string           `gorm:"column:package_name" json:"packageName" binding:"notblank,max=100"`
	ModuleName     string           `gorm:"column:module_name" json:"moduleName" binding:"notblank,max=30"`
	BusinessName   string           `gorm:"column:business_name" json:"businessName" binding:"notblank,max=30"`
	FunctionName   string           `gorm:"column:function_name" json:"functionName" binding:"notblank,max=50"`
	FunctionAuthor string           `gorm:"column:function_author" json:"functionAuthor" binding:"notblank,max=50"`
	FormColNum     *int             `gorm:"column:form_col_num" json:"formColNum"`
	GenType        string           `gorm:"column:gen_type" json:"genType"`
	GenPath        *string          `gorm:"column:gen_path" json:"genPath"`
	Options        *string          `gorm:"column:options" json:"options"`
	CreateBy       string           `gorm:"column:create_by" json:"createBy"`
	CreateTime     types.Time       `gorm:"column:create_time" json:"createTime"`
	UpdateBy       string           `gorm:"column:update_by" json:"updateBy"`
	UpdateTime     types.Time       `gorm:"column:update_time" json:"updateTime"`
	Remark         *string          `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`
	Params         map[string]any   `gorm:"-" json:"params,omitempty"`
	Columns        []GenTableColumn `gorm:"-" json:"columns"`
	PKColumn       *GenTableColumn  `gorm:"-" json:"pkColumn"`
	SubTable       *GenTable        `gorm:"-" json:"subTable"`
	TreeCode       string           `gorm:"-" json:"treeCode"`
	TreeParentCode string           `gorm:"-" json:"treeParentCode"`
	TreeName       string           `gorm:"-" json:"treeName"`
	ParentMenuID   int64            `gorm:"-" json:"parentMenuId"`
	ParentMenuName string           `gorm:"-" json:"parentMenuName"`
	View           bool             `gorm:"-" json:"view"`
}

// MarshalJSON 补齐 Java 的 isSub/isTree/isCrud 计算属性。
func (t GenTable) MarshalJSON() ([]byte, error) {
	type alias GenTable
	return json.Marshal(struct {
		alias
		Sub  bool `json:"sub"`
		Tree bool `json:"tree"`
		Crud bool `json:"crud"`
	}{alias: alias(t), Sub: t.TplCategory == "sub", Tree: t.TplCategory == "tree", Crud: t.TplCategory == "crud"})
}

// GenTableColumn 对应代码生成字段表 gen_table_column。
type GenTableColumn struct {
	ColumnID      int64      `gorm:"column:column_id;primaryKey;autoIncrement" json:"columnId"`
	TableID       int64      `gorm:"column:table_id" json:"tableId"`
	ColumnName    string     `gorm:"column:column_name" json:"columnName"`
	ColumnComment string     `gorm:"column:column_comment" json:"columnComment"`
	ColumnType    string     `gorm:"column:column_type" json:"columnType"`
	JavaType      string     `gorm:"column:java_type" json:"javaType"`
	JavaField     string     `gorm:"column:java_field" json:"javaField" binding:"notblank,max=200"`
	IsPK          string     `gorm:"column:is_pk" json:"isPk"`
	IsIncrement   string     `gorm:"column:is_increment" json:"isIncrement"`
	IsRequired    string     `gorm:"column:is_required" json:"isRequired"`
	IsInsert      string     `gorm:"column:is_insert" json:"isInsert"`
	IsEdit        string     `gorm:"column:is_edit" json:"isEdit"`
	IsList        string     `gorm:"column:is_list" json:"isList"`
	IsQuery       string     `gorm:"column:is_query" json:"isQuery"`
	QueryType     string     `gorm:"column:query_type" json:"queryType"`
	HTMLType      string     `gorm:"column:html_type" json:"htmlType"`
	DictType      *string    `gorm:"column:dict_type" json:"dictType"`
	Sort          *int       `gorm:"column:sort" json:"sort"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"`
}

// MarshalJSON 补齐 Java bean 暴露的计算属性，避免编辑页/第三方消费者字段漂移。
func (c GenTableColumn) MarshalJSON() ([]byte, error) {
	type alias GenTableColumn
	field := c.JavaField
	capField := field
	if field != "" {
		capField = strings.ToUpper(field[:1]) + field[1:]
	}
	super := false
	for _, value := range []string{"createBy", "createTime", "updateBy", "updateTime", "remark", "parentName", "parentId", "orderNum", "ancestors"} {
		if strings.EqualFold(field, value) {
			super = true
			break
		}
	}
	usable := strings.EqualFold(field, "parentId") || strings.EqualFold(field, "orderNum") || strings.EqualFold(field, "remark")
	return json.Marshal(struct {
		alias
		CapJavaField string `json:"capJavaField"`
		PK           bool   `json:"pk"`
		Increment    bool   `json:"increment"`
		Required     bool   `json:"required"`
		Insert       bool   `json:"insert"`
		Edit         bool   `json:"edit"`
		List         bool   `json:"list"`
		Query        bool   `json:"query"`
		SuperColumn  bool   `json:"superColumn"`
		UsableColumn bool   `json:"usableColumn"`
	}{
		alias: alias(c), CapJavaField: capField,
		PK: c.IsPK == "1", Increment: c.IsIncrement == "1", Required: c.IsRequired == "1",
		Insert: c.IsInsert == "1", Edit: c.IsEdit == "1", List: c.IsList == "1", Query: c.IsQuery == "1",
		SuperColumn: super, UsableColumn: usable,
	})
}

// GenTableQuery 代码生成列表和数据库表列表共用的查询条件。
type GenTableQuery struct {
	TableName    string `form:"tableName"`
	TableComment string `form:"tableComment"`
	BeginTime    string `form:"params[beginTime]"`
	EndTime      string `form:"params[endTime]"`
}

var GenTableSortColumns = map[string]string{
	"tableId":    "table_id",
	"tableName":  "table_name",
	"className":  "class_name",
	"createTime": "create_time",
	"updateTime": "update_time",
}

var GenDBTableSortColumns = map[string]string{
	"tableName":  "table_name",
	"createTime": "create_time",
	"updateTime": "update_time",
}
