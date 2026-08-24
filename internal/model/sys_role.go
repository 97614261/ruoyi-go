package model

import (
	"encoding/json"

	"ruoyi-go/pkg/types"
)

// 数据权限范围，对应 sys_role.data_scope。
const (
	DataScopeAll          = "1" // 全部数据权限
	DataScopeCustom       = "2" // 自定数据权限
	DataScopeDept         = "3" // 本部门数据权限
	DataScopeDeptAndChild = "4" // 本部门及以下数据权限
	DataScopeSelf         = "5" // 仅本人数据权限
)

// AdminRoleID 超级管理员角色 ID。
const AdminRoleID int64 = 1

// SysRole 角色表 sys_role。
type SysRole struct {
	RoleID   int64  `gorm:"column:role_id;primaryKey;autoIncrement" json:"roleId" excel:"name:角色序号;cell:numeric"`
	RoleName string `gorm:"column:role_name" json:"roleName" binding:"notblank,max=30" excel:"name:角色名称"`
	RoleKey  string `gorm:"column:role_key" json:"roleKey" binding:"notblank,max=100" excel:"name:角色权限"`
	// 指针 + required：对齐 Java 的 Integer + @NotNull，同时让 roleSort=0 通过。
	// 理由详见 SysPost.PostSort 的注释。
	RoleSort *int `gorm:"column:role_sort" json:"roleSort" binding:"required,min=0" excel:"name:角色排序"`
	// DataScope 由"分配数据权限"单独维护，普通新增/修改不校验
	DataScope         string     `gorm:"column:data_scope" json:"dataScope" excel:"name:数据范围;converter:1=所有数据权限,2=自定义数据权限,3=本部门数据权限,4=本部门及以下数据权限,5=仅本人数据权限"`
	MenuCheckStrictly bool       `gorm:"column:menu_check_strictly" json:"menuCheckStrictly"`
	DeptCheckStrictly bool       `gorm:"column:dept_check_strictly" json:"deptCheckStrictly"`
	Status            string     `gorm:"column:status" json:"status" binding:"required" excel:"name:角色状态;converter:0=正常,1=停用"`
	DelFlag           string     `gorm:"column:del_flag" json:"delFlag"`
	CreateBy          string     `gorm:"column:create_by" json:"createBy"`
	CreateTime        types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy          string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime        types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark            *string    `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// Flag 非表字段，角色分配界面用于标记是否选中
	Flag bool `gorm:"-" json:"flag"`
	// Permissions 该角色拥有的权限标识，登录时按角色维度填充。
	// 数据权限（@DataScope 的等价实现）要按角色匹配权限，依赖这个字段。
	Permissions []string `gorm:"-" json:"permissions"`
	// MenuIDs / DeptIDs 前端提交的菜单树、部门树选中项，非表字段
	MenuIDs []int64 `gorm:"-" json:"menuIds"`
	DeptIDs []int64 `gorm:"-" json:"deptIds"`
}

func (SysRole) TableName() string { return "sys_role" }

// IsAdmin 是否超级管理员角色。
func (r SysRole) IsAdmin() bool { return r.RoleID == AdminRoleID }

// MarshalJSON 补出 admin 字段。
//
// Java 版 SysRole 的 isAdmin() 是个 getter，Jackson 会把它序列化成
// "admin": true/false。前端确实读这个字段，所以必须补上，
// 不能只留 IsAdmin() 方法。
func (r SysRole) MarshalJSON() ([]byte, error) {
	type alias SysRole // 用别名避免递归调用 MarshalJSON
	return json.Marshal(struct {
		alias
		Admin bool `json:"admin"`
	}{alias(r), r.IsAdmin()})
}

// RoleStatusBody /system/role/changeStatus 的请求体。
//
// 前端只传 roleId 和 status，绑到 SysRole 上会被 roleName 的 notblank 拒掉，
// 所以必须用专用 DTO。changeStatus / dataScope 这类"局部更新"接口都要这么处理。
type RoleStatusBody struct {
	RoleID int64  `json:"roleId" binding:"required"`
	Status string `json:"status" binding:"required"`
}

// RoleDataScopeBody /system/role/dataScope 的请求体。
type RoleDataScopeBody struct {
	RoleID            int64   `json:"roleId" binding:"required"`
	DataScope         string  `json:"dataScope" binding:"required"`
	DeptIDs           []int64 `json:"deptIds"`
	DeptCheckStrictly bool    `json:"deptCheckStrictly"`
}

// RoleQuery 角色列表的查询条件。
type RoleQuery struct {
	RoleID   int64  `form:"roleId"`
	RoleName string `form:"roleName"`
	RoleKey  string `form:"roleKey"`
	Status   string `form:"status"`
}

// RoleSortColumns 允许排序的字段白名单。
var RoleSortColumns = map[string]string{
	"roleId":     "r.role_id",
	"roleName":   "r.role_name",
	"roleKey":    "r.role_key",
	"roleSort":   "r.role_sort",
	"status":     "r.status",
	"createTime": "r.create_time",
}
