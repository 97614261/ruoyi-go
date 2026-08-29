package model

import "ruoyi-go/pkg/types"

// 菜单类型，对应 sys_menu.menu_type。
const (
	MenuTypeDir    = "M" // 目录
	MenuTypeMenu   = "C" // 菜单
	MenuTypeButton = "F" // 按钮
)

// is_frame 取值。注意 0 才是"是外链"，容易反。
//
// 【类型是字符串不是数字】库里是 int(1)，但 Java 版 SysMenu 建模成 String，
// 前端也按字符串处理：表单 <el-radio value="0"> 提交的是 "0"，
// 列表回显用 `scope.row.isFrame === '0'` 严格相等判断。
// 建模成 int 会导致两头都断：提交时反序列化失败，回显时判断不成立。
const (
	MenuIsFrameYes = "0" // 是外链
	MenuIsFrameNo  = "1" // 不是外链
)

// is_cache 取值，类型同上，也是字符串。
const (
	MenuCacheYes = "0" // 缓存
	MenuCacheNo  = "1" // 不缓存
)

// visible 取值。
const (
	MenuVisible = "0" // 显示
	MenuHidden  = "1" // 隐藏
)

// SysMenu 菜单表 sys_menu。
type SysMenu struct {
	MenuID   int64  `gorm:"column:menu_id;primaryKey;autoIncrement" json:"menuId"`
	MenuName string `gorm:"column:menu_name" json:"menuName" binding:"notblank,xss,max=50"`
	ParentID int64  `gorm:"column:parent_id" json:"parentId"`
	// 指针 + required：对齐 Java 的 Integer + @NotNull，同时让 orderNum=0 通过。
	// 理由详见 SysPost.PostSort 的注释。
	OrderNum *int `gorm:"column:order_num" json:"orderNum" binding:"required,min=0"`
	// max=200 跟 Java 的 @Size 走（列长其实是 200）
	Path string `gorm:"column:path" json:"path" binding:"omitempty,xss,max=200"`
	// Component 可为 NULL（目录通常没有组件）。
	// Java 的 @Size 写的是 200（提示语里的 255 是它自己写错了），这里跟注解走
	Component *string `gorm:"column:component" json:"component" binding:"omitempty,max=200"`
	Query     *string `gorm:"column:query" json:"query" binding:"omitempty,max=255"`
	RouteName string  `gorm:"column:route_name" json:"routeName" binding:"omitempty,max=50"`
	// IsFrame / IsCache 是**字符串**，理由见上方常量注释
	IsFrame    string     `gorm:"column:is_frame" json:"isFrame"`
	IsCache    string     `gorm:"column:is_cache" json:"isCache"`
	MenuType   string     `gorm:"column:menu_type" json:"menuType" binding:"required"`
	Visible    string     `gorm:"column:visible" json:"visible"`
	Status     string     `gorm:"column:status" json:"status"`
	Perms      *string    `gorm:"column:perms" json:"perms" binding:"omitempty,max=100"`
	Icon       string     `gorm:"column:icon" json:"icon" binding:"omitempty,max=100"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark     string     `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// 非表字段
	ParentName string     `gorm:"-" json:"parentName,omitempty"`
	Children   []*SysMenu `gorm:"-" json:"children,omitempty"`
}

func (SysMenu) TableName() string { return "sys_menu" }

// MenuQuery 菜单列表的查询条件。
type MenuQuery struct {
	MenuName string `form:"menuName"`
	Visible  string `form:"visible"`
	Status   string `form:"status"`
}

// MenuSortBody /system/menu/updateSort 的请求体，格式同部门排序。
type MenuSortBody struct {
	MenuIDs   string `json:"menuIds"`
	OrderNums string `json:"orderNums"`
}

// RouterVo 前端路由对象，/getRouters 的返回元素。
//
// 字段顺序和 omitempty 规则照抄 Java 版 RouterVo —— 该省的必须省，
// Vue 的路由生成逻辑会根据键是否存在走不同分支。
type RouterVo struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Hidden     bool       `json:"hidden"`
	Redirect   string     `json:"redirect,omitempty"`
	Component  string     `json:"component,omitempty"`
	Query      string     `json:"query,omitempty"`
	AlwaysShow bool       `json:"alwaysShow,omitempty"`
	Meta       *MetaVo    `json:"meta,omitempty"`
	Children   []RouterVo `json:"children,omitempty"`
}

// MetaVo 路由元信息。
type MetaVo struct {
	Title   string `json:"title"`
	Icon    string `json:"icon"`
	NoCache bool   `json:"noCache"`
	// Link 外链地址，非外链时为 null
	Link *string `json:"link"`
}
