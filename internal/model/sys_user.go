package model

import (
	"encoding/json"

	"ruoyi-go/pkg/types"
)

// AdminUserID 超级管理员用户 ID。
const AdminUserID int64 = 1

// 通用状态值。注意是字符串不是数字，与 RuoYi 保持一致。
const (
	StatusNormal  = "0" // 正常
	StatusDisable = "1" // 停用

	DelFlagExist   = "0" // 存在
	DelFlagDeleted = "2" // 已删除
)

// SysUser 用户表 sys_user。
type SysUser struct {
	UserID   int64  `gorm:"column:user_id;primaryKey;autoIncrement" json:"userId" excel:"name:用户序号;cell:numeric;type:export"`
	DeptID   *int64 `gorm:"column:dept_id" json:"deptId" excel:"name:部门编号;type:import"`
	UserName string `gorm:"column:user_name" json:"userName" binding:"notblank,xss,max=30" excel:"name:登录名称"`
	// NickName 没有 notblank —— Java 版 SysUser.nickName 只有 @Xss 和 @Size，昵称允许为空
	NickName string `gorm:"column:nick_name" json:"nickName" binding:"omitempty,xss,max=30" excel:"name:用户名称"`
	UserType string `gorm:"column:user_type" json:"userType"`
	Email    string `gorm:"column:email" json:"email" binding:"omitempty,email,max=50" excel:"name:用户邮箱"`
	// Phonenumber 字段名跟着 RuoYi 走，不要"修正"成 phoneNumber，前端读的就是这个。
	// cell:text 强制文本，否则长数字会变成科学计数法
	Phonenumber string `gorm:"column:phonenumber" json:"phonenumber" binding:"omitempty,max=11" excel:"name:手机号码;cell:text"`
	Sex         string `gorm:"column:sex" json:"sex" excel:"name:用户性别;converter:0=男,1=女,2=未知"`
	Avatar      string `gorm:"column:avatar" json:"avatar"`
	// Password 用 omitempty + MarshalJSON 置空实现"可输入、不可输出"：
	// 直接标 json:"-" 会连输入也一起挡掉，新增用户就收不到密码了。
	Password      string     `gorm:"column:password" json:"password,omitempty" binding:"omitempty,min=5,max=20"`
	Status        string     `gorm:"column:status" json:"status" binding:"omitempty,oneof=0 1" excel:"name:账号状态;converter:0=正常,1=停用"`
	DelFlag       string     `gorm:"column:del_flag" json:"delFlag"`
	LoginIP       string     `gorm:"column:login_ip" json:"loginIp" excel:"name:最后登录IP;type:export"`
	LoginDate     types.Time `gorm:"column:login_date" json:"loginDate" excel:"name:最后登录时间;type:export;width:30"`
	PwdUpdateDate types.Time `gorm:"column:pwd_update_date" json:"pwdUpdateDate"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark        *string    `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// 以下为非表字段，由 repository 关联查询后填充
	Dept  *SysDept  `gorm:"-" json:"dept,omitempty"`
	Roles []SysRole `gorm:"-" json:"roles"`
	// RoleIDs / PostIDs 前端提交的角色、岗位选中项
	RoleIDs []int64 `gorm:"-" json:"roleIds"`
	PostIDs []int64 `gorm:"-" json:"postIds"`

	// 导出专用的扁平字段。excelx 不支持 Java @Excel 的 targetAttr（嵌套取值），
	// 所以导出前由 service 从 Dept 里拷过来。json:"-" 保证不影响接口响应。
	ExportDeptName string `gorm:"-" json:"-" excel:"name:部门名称;type:export"`
	ExportLeader   string `gorm:"-" json:"-" excel:"name:部门负责人;type:export"`
}

func (SysUser) TableName() string { return "sys_user" }

// IsAdmin 是否超级管理员。
func (u SysUser) IsAdmin() bool { return u.UserID == AdminUserID }

// MarshalJSON 补出 admin 字段，并确保密码永不出现在响应里。
func (u SysUser) MarshalJSON() ([]byte, error) {
	type alias SysUser
	shadow := alias(u)
	// 配合 json:"password,omitempty"，置空后整个键都不会输出
	shadow.Password = ""
	return json.Marshal(struct {
		alias
		Admin bool `json:"admin"`
	}{shadow, u.IsAdmin()})
}

// UserQuery 用户列表的查询条件。
type UserQuery struct {
	UserID      int64  `form:"userId"`
	UserName    string `form:"userName"`
	Phonenumber string `form:"phonenumber"`
	Status      string `form:"status"`
	// DeptID 按部门筛选时包含其所有子部门
	DeptID int64 `form:"deptId"`
	// 时间范围，前端以 params[beginTime] / params[endTime] 提交
	BeginTime string `form:"params[beginTime]"`
	EndTime   string `form:"params[endTime]"`
}

// UserSortColumns 允许排序的字段白名单。
var UserSortColumns = map[string]string{
	"userId":     "u.user_id",
	"userName":   "u.user_name",
	"nickName":   "u.nick_name",
	"status":     "u.status",
	"createTime": "u.create_time",
}

// UserResetPwdBody /system/user/resetPwd 的请求体。
type UserResetPwdBody struct {
	UserID   int64  `json:"userId" binding:"required"`
	Password string `json:"password" binding:"required,min=5,max=20"`
}

// ProfileBody /system/user/profile 的请求体。
//
// 刻意只收这四个字段：部门、角色、状态如果允许自助修改，
// 用户就能给自己换部门绕过数据权限，或者把自己改成启用状态。
type ProfileBody struct {
	NickName    string `json:"nickName" binding:"omitempty,xss,max=30"`
	Email       string `json:"email" binding:"omitempty,email,max=50"`
	Phonenumber string `json:"phonenumber" binding:"omitempty,max=11"`
	Sex         string `json:"sex"`
}

// UnlockBody /unlockscreen 的请求体。
//
// 不加 binding:"required"：Java 版是在方法体里判空并返回"密码不能为空"，
// 用 binding 会变成"参数 Password 不合法"，文案对不上。
type UnlockBody struct {
	Password string `json:"password"`
}

// UpdatePwdBody /system/user/profile/updatePwd 的请求体。
//
// 【参数在 JSON body 里，不是查询串】
// Vue3 的 updateUserPwd 用的是 `data: data`，Java 那边是
// `updatePwd(@RequestBody Map<String, String> params)`。
//
// 这里曾经写成 c.Query 读查询串 —— 后果是 Go 永远拿到两个空串，
// 用真实前端改密码**从来没成功过**，报的还是"旧密码和新密码不能为空"，
// 用户完全猜不到是参数没接上。
//
// 刻意不加 binding 校验：空值和长度由 service 判，
// 这样错误文案能和 Java 保持一致（Java 也是在方法体里判的）。
type UpdatePwdBody struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// UserStatusBody /system/user/changeStatus 的请求体。
type UserStatusBody struct {
	UserID int64  `json:"userId" binding:"required"`
	Status string `json:"status" binding:"required,oneof=0 1"`
}
