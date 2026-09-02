package service

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

// GetProfile 查当前用户的个人信息，以及角色组、岗位组文案。
//
// roleGroup / postGroup 是逗号拼接的名称串，前端直接展示。
func GetProfile(ctx context.Context, userID int64) (*model.SysUser, string, string, error) {
	user, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return nil, "", "", err
	}
	if user == nil {
		return nil, "", "", errs.New("用户不存在")
	}

	roleNames := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		roleNames = append(roleNames, role.RoleName)
	}

	postNames, err := repository.SelectPostNamesByUserID(ctx, userID)
	if err != nil {
		return nil, "", "", err
	}
	return user, strings.Join(roleNames, ","), strings.Join(postNames, ","), nil
}

// UpdateProfile 修改个人信息。
//
// 只允许改昵称、邮箱、手机号、性别 —— 其余字段（部门、角色、状态）
// 必须走用户管理，否则用户可以自己改部门绕过数据权限。
func UpdateProfile(ctx context.Context, userID int64, body model.ProfileBody) error {
	userWriteMu.Lock()
	defer userWriteMu.Unlock()

	current, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if current == nil {
		return errs.New("用户不存在")
	}

	if body.Phonenumber != "" {
		count, err := repository.CountUserByPhone(ctx, body.Phonenumber, userID)
		if err != nil {
			return err
		}
		if count > 0 {
			return errs.Newf("修改用户'%s'失败，手机号码已存在", current.UserName)
		}
	}
	if body.Email != "" {
		count, err := repository.CountUserByEmail(ctx, body.Email, userID)
		if err != nil {
			return err
		}
		if count > 0 {
			return errs.Newf("修改用户'%s'失败，邮箱账号已存在", current.UserName)
		}
	}

	if err := repository.UpdateUserProfile(ctx, userID, body); err != nil {
		return err
	}
	return RefreshOnlineUserByID(ctx, userID)
}

// UpdateProfilePwd 修改自己的密码。
//
// 【注意】旧密码必须重新查库校验，不能用会话里的。
// LoginUser 序列化进 Redis 时密码被剔除了（SysUser.MarshalJSON 置空），
// 拿会话里的值比对永远失败。
func UpdateProfilePwd(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	if oldPassword == "" || newPassword == "" {
		return errs.New("旧密码和新密码不能为空")
	}
	if !validPasswordLength(newPassword) {
		return errs.New("新密码长度必须在 5 到 20 个字符之间")
	}

	user, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errs.New("用户不存在")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)) != nil {
		return errs.New("修改密码失败，旧密码错误")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(newPassword)) == nil {
		return errs.New("新密码不能与旧密码相同")
	}

	hashed, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := RevokeUserSessions(ctx, userID); err != nil {
		return err
	}
	if err := repository.ResetUserPwd(ctx, userID, hashed, user.UserName, time.Now()); err != nil {
		return err
	}
	return nil
}

// UnlockScreen 校验锁屏密码。
//
// 只做校验、不改任何状态 —— 锁屏是纯前端行为（store/modules/lock.js 里存标记），
// 后端唯一的职责是确认"坐在屏幕前的还是本人"。
//
// 【必须回库取密码】会话里的 LoginUser 序列化时把密码置空了
// （SysUser.MarshalJSON），拿它比对永远失败。这一点和改密码是同一个坑。
func UnlockScreen(ctx context.Context, userID int64, password string) error {
	if password == "" {
		return errs.New("密码不能为空")
	}

	user, err := repository.SelectUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errs.New("服务器超时，请重新登录")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return errs.New("密码错误，请重新输入")
	}
	return nil
}

// AvatarUpdateResult 区分数据库是否已经接受新头像，供调用方正确回收文件。
type AvatarUpdateResult struct {
	OldAvatar string
	Persisted bool
}

// UpdateAvatar 保存头像地址，并返回被替换的旧头像。
func UpdateAvatar(ctx context.Context, userID int64, avatar string) (AvatarUpdateResult, error) {
	oldAvatar, found, err := repository.ReplaceUserAvatar(ctx, userID, avatar)
	if err != nil {
		return AvatarUpdateResult{}, err
	}
	if !found {
		return AvatarUpdateResult{}, errs.New("用户不存在")
	}

	result := AvatarUpdateResult{OldAvatar: oldAvatar, Persisted: true}
	if err := RefreshOnlineUserByID(ctx, userID); err != nil {
		return result, err
	}
	return result, nil
}
