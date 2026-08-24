package model

// 关联表实体。这些表只有联合主键、没有业务字段，也没有 create_time 之类的审计列。

// SysRoleMenu 角色和菜单关联表 sys_role_menu。
type SysRoleMenu struct {
	RoleID int64 `gorm:"column:role_id;primaryKey" json:"roleId"`
	MenuID int64 `gorm:"column:menu_id;primaryKey" json:"menuId"`
}

func (SysRoleMenu) TableName() string { return "sys_role_menu" }

// SysRoleDept 角色和部门关联表 sys_role_dept（数据权限用）。
type SysRoleDept struct {
	RoleID int64 `gorm:"column:role_id;primaryKey" json:"roleId"`
	DeptID int64 `gorm:"column:dept_id;primaryKey" json:"deptId"`
}

func (SysRoleDept) TableName() string { return "sys_role_dept" }

// SysUserRole 用户和角色关联表 sys_user_role。
type SysUserRole struct {
	UserID int64 `gorm:"column:user_id;primaryKey" json:"userId"`
	RoleID int64 `gorm:"column:role_id;primaryKey" json:"roleId"`
}

func (SysUserRole) TableName() string { return "sys_user_role" }

// SysUserPost 用户和岗位关联表 sys_user_post。
type SysUserPost struct {
	UserID int64 `gorm:"column:user_id;primaryKey" json:"userId"`
	PostID int64 `gorm:"column:post_id;primaryKey" json:"postId"`
}

func (SysUserPost) TableName() string { return "sys_user_post" }
