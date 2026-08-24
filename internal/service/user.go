package service

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ruoyi-go/internal/datascope"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// PermUserList 用户列表的权限标识。
const PermUserList = "system:user:list"

// userScope 用户查询的数据权限过滤器。
//
// 有 userAlias，所以"仅本人"能正确表达成 u.user_id = 当前用户。
//
// 【DeptAlias 用 u 而不是 Java 的 d，过滤结果完全相同】
// Java 是 @DataScope(deptAlias = "d")，条件落在 LEFT JOIN 出来的 d.dept_id 上。
// 但那个 JOIN 是按 sys_dept 主键做的，所以对每一行只有两种可能：
// d.dept_id 等于 u.dept_id，或者 dept 行不存在、d.dept_id 为 NULL。
//
// 而所有数据范围的取值都来自 sys_dept 或 sys_role_dept —— 一个在 sys_dept 里
// 不存在的 dept_id 永远不可能出现在允许列表中。所以
// "d.dept_id IN (...)" 和 "u.dept_id IN (...)" 对每一行的判定都一致。
// （唯一有差别的是兜底条件 dept_id = 0，已改成与别名无关的 1 = 0，见 datascope.denyAll）
//
// 换掉之后 LEFT JOIN sys_dept 就没有任何条件依赖它了，可以整个去掉。
// 实测：10 万用户下那次 JOIN 让分页的 COUNT 从几十毫秒涨到 432ms，
// 压测里 5214 条慢 SQL 全是它。
func userScope(user *model.SysUser) func(*gorm.DB) *gorm.DB {
	return datascope.Scope(datascope.Params{
		User:       user,
		DeptAlias:  repository.UserTableAlias,
		UserAlias:  repository.UserTableAlias,
		Permission: PermUserList,
	})
}

// ListUserPage 分页查询用户。
func ListUserPage(ctx context.Context, operator *model.SysUser, query model.UserQuery, pg page.Query) ([]model.SysUser, int64, error) {
	return repository.SelectUserPage(ctx, query, pg, userScope(operator))
}

// ListUserExport 导出用的全量查询，顺便把部门信息摊平到导出字段。
func ListUserExport(ctx context.Context, operator *model.SysUser, query model.UserQuery) ([]model.SysUser, error) {
	list, err := repository.SelectUserList(ctx, query, userScope(operator))
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Dept == nil {
			continue
		}
		list[i].ExportDeptName = list[i].Dept.DeptName
		if list[i].Dept.Leader != nil {
			list[i].ExportLeader = *list[i].Dept.Leader
		}
	}
	return list, nil
}

// GetUser 查用户详情（含角色、岗位），带数据权限校验。
func GetUser(ctx context.Context, operator *model.SysUser, userID int64) (*model.SysUser, []int64, error) {
	if err := CheckUserDataScope(ctx, operator, userID); err != nil {
		return nil, nil, err
	}
	user, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, errs.New("用户不存在")
	}
	postIDs, err := repository.SelectPostIDsByUserID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return user, postIDs, nil
}

// CheckUserAllowed 超级管理员不允许被修改、停用或删除。
func CheckUserAllowed(userID int64) error {
	if userID == model.AdminUserID {
		return errs.New("不允许操作超级管理员用户")
	}
	return nil
}

// CheckUserDataScope 校验当前用户是否有权操作目标用户。
func CheckUserDataScope(ctx context.Context, operator *model.SysUser, userID int64) error {
	if operator == nil || operator.IsAdmin() || userID == 0 {
		return nil
	}
	list, err := repository.SelectUserList(ctx, model.UserQuery{UserID: userID}, userScope(operator))
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errs.New("没有权限访问用户数据！")
	}
	return nil
}

// CreateUser 新增用户。
func CreateUser(ctx context.Context, operator *model.SysUser, user *model.SysUser, operatorName string) error {
	if err := checkUserRelatedScope(ctx, operator, user); err != nil {
		return err
	}
	if err := checkUserUnique(ctx, user, "新增"); err != nil {
		return err
	}
	if user.Password == "" {
		return errs.New("密码不能为空")
	}

	hashed, err := HashPassword(user.Password)
	if err != nil {
		return err
	}

	user.UserID = 0
	user.Password = hashed
	user.DelFlag = model.DelFlagExist
	if user.UserType == "" {
		user.UserType = "00"
	}
	if user.Status == "" {
		user.Status = model.StatusNormal
	}
	user.CreateBy = operatorName
	user.CreateTime = types.Now()
	// 【不要】给 PwdUpdateDate 赋值。Java 的 insertUser 只在调用方显式设置时才写这一列，
	// 而 controller 的 add 不设置，所以新用户的 pwd_update_date 是 NULL —— 这正是
	// getInfo 里 isDefaultModifyPwd 判定"请修改初始密码"的依据。赋了值提示就永远不出现。
	return repository.InsertUser(ctx, user)
}

// UpdateUser 修改用户。
//
// 不处理密码：编辑表单不含密码字段，若一并更新会把账号密码清成空串的哈希。
func UpdateUser(ctx context.Context, operator *model.SysUser, user *model.SysUser, operatorName string) error {
	if user.UserID == 0 {
		return errs.New("用户ID不能为空")
	}
	if err := CheckUserAllowed(user.UserID); err != nil {
		return err
	}
	if err := CheckUserDataScope(ctx, operator, user.UserID); err != nil {
		return err
	}
	if err := checkUserRelatedScope(ctx, operator, user); err != nil {
		return err
	}
	if err := checkUserUnique(ctx, user, "修改"); err != nil {
		return err
	}

	existing, err := repository.SelectUserByID(ctx, user.UserID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("用户不存在")
	}

	user.UpdateBy = operatorName
	user.UpdateTime = types.Now()
	return repository.UpdateUser(ctx, user)
}

// ChangeUserStatus 启用/停用用户。
func ChangeUserStatus(ctx context.Context, operator *model.SysUser, userID int64, status, operatorName string) error {
	if err := CheckUserAllowed(userID); err != nil {
		return err
	}
	if err := CheckUserDataScope(ctx, operator, userID); err != nil {
		return err
	}
	return repository.UpdateUserStatus(ctx, userID, status, operatorName)
}

// ResetUserPwd 重置密码。
func ResetUserPwd(ctx context.Context, operator *model.SysUser, userID int64, password, operatorName string) error {
	if err := CheckUserAllowed(userID); err != nil {
		return err
	}
	if err := CheckUserDataScope(ctx, operator, userID); err != nil {
		return err
	}
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}
	return repository.ResetUserPwd(ctx, userID, hashed, operatorName, time.Now())
}

// DeleteUsers 批量删除用户。
func DeleteUsers(ctx context.Context, operator *model.SysUser, userIDs []int64) error {
	if len(userIDs) == 0 {
		return errs.New("请选择要删除的用户")
	}
	for _, userID := range userIDs {
		if operator != nil && userID == operator.UserID {
			return errs.New("当前用户不能删除")
		}
		if err := CheckUserAllowed(userID); err != nil {
			return err
		}
		if err := CheckUserDataScope(ctx, operator, userID); err != nil {
			return err
		}
	}
	return repository.DeleteUserByIDs(ctx, userIDs)
}

// AuthRoleOfUser 查用户的授权角色页面数据。
//
// 非超级管理员的用户，候选角色里要剔除 admin 角色 —— 否则可以给
// 任意账号赋予超级管理员，是提权漏洞。
func AuthRoleOfUser(ctx context.Context, userID int64) (*model.SysUser, []model.SysRole, error) {
	user, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, errs.New("用户不存在")
	}

	roles, err := repository.SelectRoleAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	assigned := make(map[int64]bool, len(user.Roles))
	for _, r := range user.Roles {
		assigned[r.RoleID] = true
	}

	result := make([]model.SysRole, 0, len(roles))
	for _, role := range roles {
		if role.IsAdmin() && userID != model.AdminUserID {
			continue
		}
		role.Flag = assigned[role.RoleID]
		result = append(result, role)
	}
	return user, result, nil
}

// AssignUserRoles 保存用户的授权角色。
func AssignUserRoles(ctx context.Context, operator *model.SysUser, userID int64, roleIDs []int64) error {
	if err := CheckUserDataScope(ctx, operator, userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := CheckRoleDataScope(ctx, operator, roleID); err != nil {
			return err
		}
	}
	if err := repository.ReplaceUserRoles(ctx, userID, roleIDs); err != nil {
		return err
	}
	// 角色变了，该用户的在线会话权限要立刻刷新
	return RefreshOnlineUserByID(ctx, userID)
}

// HashPassword 生成 bcrypt 哈希。
func HashPassword(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", errs.Wrap(err, "密码加密失败")
	}
	return string(hashed), nil
}

// checkUserRelatedScope 校验提交的部门和角色是否在当前用户的数据权限内。
func checkUserRelatedScope(ctx context.Context, operator *model.SysUser, user *model.SysUser) error {
	if user.DeptID != nil {
		if err := CheckDeptDataScope(ctx, operator, *user.DeptID); err != nil {
			return err
		}
	}
	for _, roleID := range user.RoleIDs {
		if err := CheckRoleDataScope(ctx, operator, roleID); err != nil {
			return err
		}
	}
	return nil
}

// checkUserUnique 校验账号、手机号、邮箱唯一。
//
// 手机号和邮箱为空时跳过检查，与 Java 一致 —— 它们本来就是选填的，
// 多个用户留空是正常情况。
func checkUserUnique(ctx context.Context, user *model.SysUser, action string) error {
	nameCount, err := repository.CountUserByName(ctx, user.UserName, user.UserID)
	if err != nil {
		return err
	}
	if nameCount > 0 {
		return errs.Newf("%s用户'%s'失败，登录账号已存在", action, user.UserName)
	}

	if user.Phonenumber != "" {
		phoneCount, err := repository.CountUserByPhone(ctx, user.Phonenumber, user.UserID)
		if err != nil {
			return err
		}
		if phoneCount > 0 {
			return errs.Newf("%s用户'%s'失败，手机号码已存在", action, user.UserName)
		}
	}

	if user.Email != "" {
		emailCount, err := repository.CountUserByEmail(ctx, user.Email, user.UserID)
		if err != nil {
			return err
		}
		if emailCount > 0 {
			return errs.Newf("%s用户'%s'失败，邮箱账号已存在", action, user.UserName)
		}
	}
	return nil
}
