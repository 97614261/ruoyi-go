package model

import "ruoyi-go/pkg/types"

// 部门状态。
const (
	DeptNormal  = "0"
	DeptDisable = "1"
)

// SysDept 部门表 sys_dept。
type SysDept struct {
	DeptID   int64 `gorm:"column:dept_id;primaryKey;autoIncrement" json:"deptId"`
	ParentID int64 `gorm:"column:parent_id" json:"parentId"`
	// Ancestors 祖级路径，形如 "0,100,101"，由服务端计算，不接受前端提交
	Ancestors string `gorm:"column:ancestors" json:"ancestors"`
	DeptName  string `gorm:"column:dept_name" json:"deptName" binding:"notblank,max=30"`
	// OrderNum 指针 + required：对齐 Java 的 Integer + @NotNull（不传拒绝），
	// 同时让 orderNum=0 这个合法值通过。理由详见 SysPost.PostSort 的注释。
	OrderNum *int    `gorm:"column:order_num" json:"orderNum" binding:"required,min=0"`
	Leader   *string `gorm:"column:leader" json:"leader" binding:"omitempty,max=20"`
	Phone    *string `gorm:"column:phone" json:"phone" binding:"omitempty,max=11"`
	// 指针字段的两个失败点都要挡住：
	//   omitempty 处理"字段没传"（nil 指针，validator 会直接判失败）
	//   覆盖后的 email 处理"传了空串"（非 nil 指针，omitempty 不生效）
	Email      *string    `gorm:"column:email" json:"email" binding:"omitempty,email,max=50"`
	Status     string     `gorm:"column:status" json:"status"`
	DelFlag    string     `gorm:"column:del_flag" json:"delFlag"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`

	// 以下为非表字段，供树形展示使用
	ParentName string     `gorm:"-" json:"parentName,omitempty"`
	Children   []*SysDept `gorm:"-" json:"children,omitempty"`
}

// DeptQuery 部门列表的查询条件。
type DeptQuery struct {
	DeptID   int64  `form:"deptId"`
	ParentID int64  `form:"parentId"`
	DeptName string `form:"deptName"`
	Status   string `form:"status"`
}

// DeptSortBody /system/dept/updateSort 的请求体。
//
// Java 版收的是 Map<String,String>，两个字段都是逗号分隔的字符串，
// 这里保持同样的传输格式。
type DeptSortBody struct {
	DeptIDs   string `json:"deptIds"`
	OrderNums string `json:"orderNums"`
}

func (SysDept) TableName() string { return "sys_dept" }

// TreeSelect 树选择结构，对应 Java 版 TreeSelect。
//
// 用于 /system/role/deptTree/{roleId} 等接口。注意 Children 为空时
// 该键不输出（omitempty），与 Java 版行为一致 —— 前端据此判断叶子节点。
type TreeSelect struct {
	ID       int64        `json:"id"`
	Label    string       `json:"label"`
	Disabled bool         `json:"disabled"`
	Children []TreeSelect `json:"children,omitempty"`
}
