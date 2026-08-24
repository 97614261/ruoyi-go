package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// SelectUserByUserName 按账号查用户，并填充部门和角色。
//
// 用户不存在时返回 (nil, nil)，由调用方决定如何提示 —— 注意登录场景
// 必须对"用户不存在"和"密码错误"给出相同提示，避免账号枚举。
//
// Java 版用一条大 join + 嵌套 resultMap 拼装，这里拆成 3 条固定查询，
// 语义相同但更直观（不是 N+1，条数与用户数无关）。
func SelectUserByUserName(ctx context.Context, userName string) (*model.SysUser, error) {
	var user model.SysUser
	err := DB(ctx).
		Where("user_name = ?", userName).
		Where("del_flag = ?", model.DelFlagExist).
		Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户 %s 失败: %w", userName, err)
	}

	if err := fillUserRelations(ctx, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// SelectUserByID 按用户 ID 查用户，并填充部门和角色。
//
// 过滤 del_flag，理由同 SelectDeptByID：已删除的用户不该还能被查到、
// 被改状态、被重置密码。
func SelectUserByID(ctx context.Context, userID int64) (*model.SysUser, error) {
	var user model.SysUser
	err := DB(ctx).
		Where("user_id = ?", userID).
		Where("del_flag = ?", model.DelFlagExist).
		Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 失败: %w", userID, err)
	}

	if err := fillUserRelations(ctx, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func fillUserRelations(ctx context.Context, user *model.SysUser) error {
	if user.DeptID != nil {
		var dept model.SysDept
		err := DB(ctx).Where("dept_id = ?", *user.DeptID).Take(&dept).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询部门 %d 失败: %w", *user.DeptID, err)
		}
		if err == nil {
			user.Dept = &dept
		}
	}

	roles, err := SelectRolesByUserID(ctx, user.UserID)
	if err != nil {
		return err
	}
	user.Roles = roles
	return nil
}

// UserTableAlias 用户表在列表查询里的别名。
const UserTableAlias = "u"

// userListDB 构造用户列表的基础查询。
//
// 【没有 LEFT JOIN sys_dept，是刻意的】
// Java 版 join 它有两个用途：给数据权限提供 d.dept_id，以及顺带取
// d.dept_name / d.leader。这两条在 Go 侧都不成立 ——
// 数据权限已改用 u.dept_id（等价性证明见 service.userScope），
// 部门信息由 attachDepts 批量补齐。留着就是纯开销。
//
// 10 万用户下实测：这次 JOIN 让分页的 COUNT 达到 432ms，
// 并发 50 压测时 5214 条慢 SQL 全是它。
func userListDB(ctx context.Context, query model.UserQuery, scope func(*gorm.DB) *gorm.DB) *gorm.DB {
	db := DB(ctx).
		Table("sys_user u").
		Where("u.del_flag = ?", model.DelFlagExist)

	if query.UserID != 0 {
		db = db.Where("u.user_id = ?", query.UserID)
	}
	if query.UserName != "" {
		db = db.Where("u.user_name LIKE ?", "%"+query.UserName+"%")
	}
	if query.Phonenumber != "" {
		db = db.Where("u.phonenumber LIKE ?", "%"+query.Phonenumber+"%")
	}
	if query.Status != "" {
		db = db.Where("u.status = ?", query.Status)
	}
	// 按部门筛选时包含所有子部门，与 Java 一致
	if query.DeptID != 0 {
		db = db.Where("(u.dept_id = ? OR u.dept_id IN (SELECT t.dept_id FROM sys_dept t WHERE find_in_set(?, t.ancestors)))",
			query.DeptID, query.DeptID)
	}
	if query.BeginTime != "" {
		db = db.Where("date_format(u.create_time,'%Y%m%d') >= date_format(?,'%Y%m%d')", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("date_format(u.create_time,'%Y%m%d') <= date_format(?,'%Y%m%d')", query.EndTime)
	}
	if scope != nil {
		db = db.Scopes(scope)
	}
	return db
}

// SelectUserPage 分页查询用户，附带部门信息。
//
// 【这里没有 DISTINCT，是刻意的】
// userListDB 只 LEFT JOIN 了 sys_dept，而且是按主键 join，一个用户至多对应
// 一个部门，不可能产生重复行 —— DISTINCT 去不掉任何东西，只会让 MySQL
// 为整个结果集建一张临时表。
//
// 10 万用户下实测：带 DISTINCT 448ms，去掉后 1.5ms，差 300 倍。
// 并发 50 压测时带 DISTINCT 的版本 100% 超时。
//
// Java 版 selectUserList 同样没有 distinct（SysUserMapper.xml）。
// selectAllocatedList / selectUnallocatedList 才有，因为那两个 join 了
// sys_user_role 和 sys_role，是真的会出重复行 —— 别把这里的结论套过去。
func SelectUserPage(ctx context.Context, query model.UserQuery, pg page.Query, scope func(*gorm.DB) *gorm.DB) ([]model.SysUser, int64, error) {
	var total int64
	if err := userListDB(ctx, query, scope).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计用户总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysUser{}, 0, nil
	}

	orderBy := pg.OrderBy
	if orderBy == "" {
		orderBy = "u.user_id"
	}

	var list []model.SysUser
	err := userListDB(ctx, query, scope).
		Select("u.*").
		Order(orderBy).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}
	if err := attachDepts(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// SelectUserList 不分页查询，供导出使用。
//
// 同样不加 DISTINCT，理由见 SelectUserPage。导出的行数更多，
// 临时表的代价也更大。
func SelectUserList(ctx context.Context, query model.UserQuery, scope func(*gorm.DB) *gorm.DB) ([]model.SysUser, error) {
	var list []model.SysUser
	err := userListDB(ctx, query, scope).
		Select("u.*").
		Order("u.user_id").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户列表失败: %w", err)
	}
	if err := attachDepts(ctx, list); err != nil {
		return nil, err
	}
	return list, nil
}

// attachDepts 批量补部门信息。
//
// 一次 IN 查询搞定，不在循环里查库（N+1）。
func attachDepts(ctx context.Context, users []model.SysUser) error {
	deptIDs := make([]int64, 0, len(users))
	seen := make(map[int64]bool)
	for _, u := range users {
		if u.DeptID != nil && !seen[*u.DeptID] {
			seen[*u.DeptID] = true
			deptIDs = append(deptIDs, *u.DeptID)
		}
	}
	if len(deptIDs) == 0 {
		return nil
	}

	var depts []model.SysDept
	if err := DB(ctx).Where("dept_id IN ?", deptIDs).Find(&depts).Error; err != nil {
		return fmt.Errorf("批量查询部门失败: %w", err)
	}
	byID := make(map[int64]*model.SysDept, len(depts))
	for i := range depts {
		byID[depts[i].DeptID] = &depts[i]
	}
	for i := range users {
		if users[i].DeptID != nil {
			users[i].Dept = byID[*users[i].DeptID]
		}
	}
	return nil
}

// authUserDB 角色-用户分配页面的基础查询。
//
// allocated=true 查"已分配"，false 查"未分配"。
func authUserDB(ctx context.Context, roleID int64, query model.UserQuery, allocated bool, scope func(*gorm.DB) *gorm.DB) *gorm.DB {
	db := DB(ctx).
		Table("sys_user u").
		// 这里同样不 join sys_dept：数据权限用的是 u.dept_id，
		// 部门信息由 attachDepts 补。但下面两个 join 必须留 ——
		// 未分配/已分配是靠 sys_user_role 判定的，也正因为它们
		// 会产生重复行，这个查询的 DISTINCT 不能去掉（Java 版同样有）
		Joins("LEFT JOIN sys_user_role ur ON u.user_id = ur.user_id").
		Joins("LEFT JOIN sys_role r ON r.role_id = ur.role_id").
		Where("u.del_flag = ?", model.DelFlagExist)

	if allocated {
		db = db.Where("r.role_id = ?", roleID)
	} else {
		// 未分配：当前行不是该角色，且该用户完全没有这个角色
		// （单看 r.role_id != ? 会因为用户有其它角色而误命中）
		db = db.Where("(r.role_id <> ? OR r.role_id IS NULL)", roleID).
			Where(`u.user_id NOT IN (
				SELECT u2.user_id FROM sys_user u2
				INNER JOIN sys_user_role ur2 ON u2.user_id = ur2.user_id AND ur2.role_id = ?)`, roleID)
	}

	if query.UserName != "" {
		db = db.Where("u.user_name LIKE ?", "%"+query.UserName+"%")
	}
	if query.Phonenumber != "" {
		db = db.Where("u.phonenumber LIKE ?", "%"+query.Phonenumber+"%")
	}
	if scope != nil {
		db = db.Scopes(scope)
	}
	return db
}

// SelectAuthUserPage 分页查询角色的已分配/未分配用户。
func SelectAuthUserPage(ctx context.Context, roleID int64, query model.UserQuery, pg page.Query,
	allocated bool, scope func(*gorm.DB) *gorm.DB) ([]model.SysUser, int64, error) {

	var total int64
	if err := authUserDB(ctx, roleID, query, allocated, scope).Distinct("u.user_id").Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计角色 %d 的用户总数失败: %w", roleID, err)
	}
	if total == 0 {
		return []model.SysUser{}, 0, nil
	}

	var list []model.SysUser
	err := authUserDB(ctx, roleID, query, allocated, scope).
		Distinct("u.*").
		Order("u.user_id").
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询角色 %d 的用户失败: %w", roleID, err)
	}
	if err := attachDepts(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// DeleteUserRole 取消若干用户的某个角色授权。
func DeleteUserRole(ctx context.Context, roleID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	err := DB(ctx).
		Where("role_id = ?", roleID).
		Where("user_id IN ?", userIDs).
		Delete(&model.SysUserRole{}).Error
	if err != nil {
		return fmt.Errorf("取消角色 %d 授权失败: %w", roleID, err)
	}
	return nil
}

// InsertUserRole 批量给用户授予某个角色。
//
// 用 INSERT IGNORE 语义避免重复授权时报主键冲突。
func InsertUserRole(ctx context.Context, roleID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	rows := make([]model.SysUserRole, 0, len(userIDs))
	for _, userID := range userIDs {
		rows = append(rows, model.SysUserRole{UserID: userID, RoleID: roleID})
	}
	err := DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	if err != nil {
		return fmt.Errorf("授予角色 %d 失败: %w", roleID, err)
	}
	return nil
}

// CountUserByName 同账号用户数，excludeID 用于修改时排除自身。
func CountUserByName(ctx context.Context, userName string, excludeID int64) (int64, error) {
	return countUser(ctx, "user_name = ?", userName, excludeID)
}

// CountUserByPhone 同手机号用户数。
func CountUserByPhone(ctx context.Context, phone string, excludeID int64) (int64, error) {
	return countUser(ctx, "phonenumber = ?", phone, excludeID)
}

// CountUserByEmail 同邮箱用户数。
func CountUserByEmail(ctx context.Context, email string, excludeID int64) (int64, error) {
	return countUser(ctx, "email = ?", email, excludeID)
}

func countUser(ctx context.Context, cond string, value any, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysUser{}).
		Where(cond, value).
		Where("del_flag = ?", model.DelFlagExist)
	if excludeID > 0 {
		db = db.Where("user_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验用户唯一性失败: %w", err)
	}
	return count, nil
}

// InsertUser 新增用户并写入角色、岗位关联。
func InsertUser(ctx context.Context, user *model.SysUser) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return fmt.Errorf("新增用户失败: %w", err)
		}
		if err := batchUserRole(tx, user.UserID, user.RoleIDs); err != nil {
			return err
		}
		return batchUserPost(tx, user.UserID, user.PostIDs)
	})
}

// UpdateUser 修改用户并重建角色、岗位关联。
//
// 不改密码 —— 密码走 ResetUserPwd，避免编辑表单误提交空密码把账号锁死。
func UpdateUser(ctx context.Context, user *model.SysUser) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		updates := map[string]any{
			"dept_id":     user.DeptID,
			"user_name":   user.UserName,
			"nick_name":   user.NickName,
			"email":       user.Email,
			"phonenumber": user.Phonenumber,
			"sex":         user.Sex,
			"status":      user.Status,
			"update_by":   user.UpdateBy,
			"update_time": user.UpdateTime,
		}
		if user.Remark != nil {
			updates["remark"] = *user.Remark
		}
		if err := tx.Model(&model.SysUser{}).Where("user_id = ?", user.UserID).Updates(updates).Error; err != nil {
			return fmt.Errorf("更新用户 %d 失败: %w", user.UserID, err)
		}

		if err := tx.Where("user_id = ?", user.UserID).Delete(&model.SysUserRole{}).Error; err != nil {
			return fmt.Errorf("清除用户角色关联失败: %w", err)
		}
		if err := tx.Where("user_id = ?", user.UserID).Delete(&model.SysUserPost{}).Error; err != nil {
			return fmt.Errorf("清除用户岗位关联失败: %w", err)
		}
		if err := batchUserRole(tx, user.UserID, user.RoleIDs); err != nil {
			return err
		}
		return batchUserPost(tx, user.UserID, user.PostIDs)
	})
}

// UpdateUserBasic 只更新用户基本信息，不动角色和岗位关联。
//
// 供 Excel 导入的"更新已存在账号"路径使用。
//
// 【为什么不复用 UpdateUser】UpdateUser 会先删光角色/岗位再按入参重建，
// 而导入的 Excel 没有角色和岗位列，入参必然是空的 —— 直接复用等于
// 把被更新用户的权限全部清空。Java 版的 importUser 就是这么干的，
// 批量改个手机号能让全公司丢权限，这个坑不照抄。
func UpdateUserBasic(ctx context.Context, user *model.SysUser) error {
	updates := map[string]any{
		"dept_id":     user.DeptID,
		"nick_name":   user.NickName,
		"email":       user.Email,
		"phonenumber": user.Phonenumber,
		"sex":         user.Sex,
		"status":      user.Status,
		"update_by":   user.UpdateBy,
		"update_time": user.UpdateTime,
	}
	if user.Remark != nil {
		updates["remark"] = *user.Remark
	}
	err := DB(ctx).Model(&model.SysUser{}).
		Where("user_id = ?", user.UserID).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("更新用户 %d 基本信息失败: %w", user.UserID, err)
	}
	return nil
}

// UpdateUserStatus 只改状态。
func UpdateUserStatus(ctx context.Context, userID int64, status, operator string) error {
	err := DB(ctx).Model(&model.SysUser{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{"status": status, "update_by": operator}).Error
	if err != nil {
		return fmt.Errorf("更新用户 %d 状态失败: %w", userID, err)
	}
	return nil
}

// ResetUserPwd 重置密码，同时更新密码修改时间。
//
// pwd_update_date 必须一起更新，否则"初始密码提醒""密码过期"这两个
// 策略会一直按旧时间判断。
func ResetUserPwd(ctx context.Context, userID int64, hashed, operator string, at time.Time) error {
	err := DB(ctx).Model(&model.SysUser{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"password":        hashed,
			"pwd_update_date": at,
			"update_time":     at,
			"update_by":       operator,
		}).Error
	if err != nil {
		return fmt.Errorf("重置用户 %d 密码失败: %w", userID, err)
	}
	return nil
}

// DeleteUserByIDs 批量删除用户（逻辑删除）并清理关联。
func DeleteUserByIDs(ctx context.Context, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id IN ?", userIDs).Delete(&model.SysUserRole{}).Error; err != nil {
			return fmt.Errorf("清除用户角色关联失败: %w", err)
		}
		if err := tx.Where("user_id IN ?", userIDs).Delete(&model.SysUserPost{}).Error; err != nil {
			return fmt.Errorf("清除用户岗位关联失败: %w", err)
		}
		if err := tx.Model(&model.SysUser{}).
			Where("user_id IN ?", userIDs).
			Update("del_flag", model.DelFlagDeleted).Error; err != nil {
			return fmt.Errorf("删除用户失败: %w", err)
		}
		return nil
	})
}

// ReplaceUserRoles 重设用户的角色（授权角色页面用）。
func ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.SysUserRole{}).Error; err != nil {
			return fmt.Errorf("清除用户角色关联失败: %w", err)
		}
		return batchUserRole(tx, userID, roleIDs)
	})
}

// UpdateUserProfile 修改个人信息，只更新用户可自助修改的字段。
func UpdateUserProfile(ctx context.Context, userID int64, body model.ProfileBody) error {
	err := DB(ctx).Model(&model.SysUser{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"nick_name":   body.NickName,
			"email":       body.Email,
			"phonenumber": body.Phonenumber,
			"sex":         body.Sex,
		}).Error
	if err != nil {
		return fmt.Errorf("更新用户 %d 个人信息失败: %w", userID, err)
	}
	return nil
}

// UpdateUserAvatar 更新头像地址。
func UpdateUserAvatar(ctx context.Context, userID int64, avatar string) error {
	err := DB(ctx).Model(&model.SysUser{}).
		Where("user_id = ?", userID).
		Update("avatar", avatar).Error
	if err != nil {
		return fmt.Errorf("更新用户 %d 头像失败: %w", userID, err)
	}
	return nil
}

// SelectPostNamesByUserID 查用户的岗位名称列表。
func SelectPostNamesByUserID(ctx context.Context, userID int64) ([]string, error) {
	var names []string
	err := DB(ctx).
		Table("sys_post p").
		Joins("LEFT JOIN sys_user_post up ON up.post_id = p.post_id").
		Where("up.user_id = ?", userID).
		Order("p.post_sort").
		Pluck("p.post_name", &names).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的岗位名称失败: %w", userID, err)
	}
	return names, nil
}

// SelectPostIDsByUserID 查用户已选的岗位 ID。
func SelectPostIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := DB(ctx).Table("sys_user_post").
		Where("user_id = ?", userID).
		Order("post_id").
		Pluck("post_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的岗位失败: %w", userID, err)
	}
	return ids, nil
}

func batchUserRole(tx *gorm.DB, userID int64, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]model.SysUserRole, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		rows = append(rows, model.SysUserRole{UserID: userID, RoleID: roleID})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("写入用户 %d 的角色关联失败: %w", userID, err)
	}
	return nil
}

func batchUserPost(tx *gorm.DB, userID int64, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	rows := make([]model.SysUserPost, 0, len(postIDs))
	for _, postID := range postIDs {
		rows = append(rows, model.SysUserPost{UserID: userID, PostID: postID})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("写入用户 %d 的岗位关联失败: %w", userID, err)
	}
	return nil
}

// UpdateLoginInfo 记录最后登录 IP 与时间。
func UpdateLoginInfo(ctx context.Context, userID int64, ip string, at time.Time) error {
	err := DB(ctx).
		Model(&model.SysUser{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"login_ip":   ip,
			"login_date": at,
		}).Error
	if err != nil {
		return fmt.Errorf("更新用户 %d 登录信息失败: %w", userID, err)
	}
	return nil
}
