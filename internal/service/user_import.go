package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/go-playground/validator/v10"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/validate"
)

// ConfigKeyInitPassword sys_config 里的初始密码键。
const ConfigKeyInitPassword = "sys.user.initPassword"

// maxImportRows 单次导入的行数上限。
//
// 导入是逐行查库 + 插入，比导出重得多；不设限的话一个几万行的文件
// 能把连接池占满、把请求拖到超时。
const maxImportRows = 5000

const maxImportMessageDetails = 100

// ImportUsers 从 Excel 导入用户。
//
// updateSupport 为真时，已存在的账号执行更新；否则记为失败。
// 返回给前端展示的结果文案（前端用 v-html 渲染，所以用 <br/> 换行）。
func ImportUsers(ctx context.Context, operator *model.SysUser, r io.Reader, updateSupport bool, operatorName string) (string, error) {
	users, rowErrors, err := excelx.Import[model.SysUser](r, "", maxImportRows)
	if err != nil {
		var limitErr excelx.RowLimitError
		if errors.As(err, &limitErr) {
			return "", errs.Newf("单次最多导入 %d 条数据", limitErr.Limit)
		}
		return "", errs.Wrap(err, "解析 Excel 失败，请确认文件格式")
	}
	if len(users) == 0 && len(rowErrors) == 0 {
		return "", errs.New("导入用户数据不能为空！")
	}

	// Keep the preload and every following user write in the same single-process
	// critical section, otherwise a concurrent create can invalidate byName.
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	deptWriteMu.Lock()
	defer deptWriteMu.Unlock()

	deptSet := make(map[int64]struct{})
	names := make([]string, 0, len(users))
	for i := range users {
		names = append(names, users[i].UserName)
		if users[i].DeptID != nil {
			deptSet[*users[i].DeptID] = struct{}{}
		}
	}
	deptIDs := make([]int64, 0, len(deptSet))
	for id := range deptSet {
		deptIDs = append(deptIDs, id)
	}
	if _, err := checkDeptIDs(ctx, operator, deptIDs); err != nil {
		return "", err
	}
	existingUsers, err := repository.SelectUsersByNames(ctx, names)
	if err != nil {
		return "", err
	}
	byName := make(map[string]model.SysUser, len(existingUsers))
	existingIDs := make([]int64, 0, len(existingUsers))
	for name, user := range existingUsers {
		byName[strings.ToLower(name)] = user
		existingIDs = append(existingIDs, user.UserID)
	}
	if updateSupport {
		if _, err := checkUserIDs(ctx, operator, existingIDs); err != nil {
			return "", err
		}
	}

	initPassword, err := GetConfigValueByKey(ctx, ConfigKeyInitPassword)
	if err != nil {
		return "", err
	}
	if initPassword == "" {
		return "", errs.New("未配置初始密码（sys.user.initPassword），请先在参数设置中补充")
	}
	if !validPasswordLength(initPassword) {
		return "", errs.Newf("初始密码长度必须在%d到%d个字符之间", passwordMinLength, passwordMaxLength)
	}
	hashed, err := HashPassword(initPassword)
	if err != nil {
		return "", err
	}

	var (
		successNum int
		failureNum int
		successMsg strings.Builder
		failureMsg strings.Builder
	)

	// 解析阶段的行错误直接计入失败
	for _, rowErr := range rowErrors {
		failureNum++
		appendImportDetail(&failureMsg, failureNum, rowErr.Error())
	}

	for i := range users {
		user := users[i]
		existing, exists := byName[strings.ToLower(user.UserName)]
		_, _, err := importOneUser(ctx, operator, &user, existingIf(exists, &existing),
			updateSupport, operatorName, hashed)
		if err != nil {
			failureNum++
			appendImportDetail(&failureMsg, failureNum,
				fmt.Sprintf("账号 %s 导入失败：%s", user.UserName, messageOf(err)))
			continue
		}
		byName[strings.ToLower(user.UserName)] = user
		successNum++
		if successNum <= maxImportMessageDetails {
			fmt.Fprintf(&successMsg, "<br/>%d、账号 %s 导入成功", successNum, html.EscapeString(user.UserName))
		}
	}
	if failureNum > 0 {
		if failureNum > maxImportMessageDetails {
			fmt.Fprintf(&failureMsg, "<br/>仅展示前 %d 条错误，其余 %d 条已省略",
				maxImportMessageDetails, failureNum-maxImportMessageDetails)
		}
		return "", errs.Newf("很抱歉，导入失败！共 %d 条数据格式不正确，错误如下：%s", failureNum, failureMsg.String())
	}
	if successNum > maxImportMessageDetails {
		fmt.Fprintf(&successMsg, "<br/>仅展示前 %d 条成功记录，其余 %d 条已省略",
			maxImportMessageDetails, successNum-maxImportMessageDetails)
	}
	return fmt.Sprintf("恭喜您，数据已全部导入成功！共 %d 条，数据如下：%s", successNum, successMsg.String()), nil
}

func importOneUser(ctx context.Context, operator *model.SysUser, user *model.SysUser, existing *model.SysUser,
	updateSupport bool, operatorName, hashedPassword string) (int64, string, error) {

	if strings.TrimSpace(user.UserName) == "" {
		return 0, "", errs.New("登录名称不能为空")
	}

	if existing == nil {
		// 校验规则与接口保持一致，不另写一套
		if err := validate.Struct(user); err != nil {
			return 0, "", errs.New(firstValidationMessage(err))
		}
		user.UserID = 0
		user.Password = hashedPassword
		user.DelFlag = model.DelFlagExist
		if user.UserType == "" {
			user.UserType = "00"
		}
		if user.Status == "" {
			user.Status = model.StatusNormal
		}
		if err := checkUserStatus(user.Status); err != nil {
			return 0, "", err
		}
		user.CreateBy = operatorName
		user.CreateTime = types.Now()
		return 0, "", repository.InsertUser(ctx, user)
	}

	if !updateSupport {
		return 0, "", errs.New("已存在")
	}

	if err := validate.Struct(user); err != nil {
		return 0, "", errs.New(firstValidationMessage(err))
	}
	if err := CheckUserAllowed(existing.UserID); err != nil {
		return 0, "", err
	}
	user.UserID = existing.UserID
	user.UpdateBy = operatorName
	user.UpdateTime = types.Now()
	if user.Status == "" {
		user.Status = existing.Status
	}
	if err := checkUserStatus(user.Status); err != nil {
		return 0, "", err
	}
	if user.Status == model.StatusDisable {
		if err := RevokeUserSessions(ctx, user.UserID); err != nil {
			return 0, "", err
		}
	}
	// 用 UpdateUserBasic 而不是 UpdateUser：导入表里没有角色和岗位列，
	// 走 UpdateUser 会把被更新用户的角色、岗位全部清空。理由详见该函数注释。
	if user.Status == model.StatusDisable {
		if err := repository.UpdateUserBasic(ctx, user); err != nil {
			return 0, "", err
		}
	} else if err := mutatePermissionState(ctx, []int64{user.UserID}, func() error {
		return repository.UpdateUserBasic(ctx, user)
	}); err != nil {
		return 0, "", err
	}
	return user.UserID, user.Status, nil
}

func existingIf(ok bool, user *model.SysUser) *model.SysUser {
	if !ok {
		return nil
	}
	return user
}

func appendImportDetail(builder *strings.Builder, number int, message string) {
	if number > maxImportMessageDetails {
		return
	}
	const maxDetailRunes = 300
	runes := []rune(message)
	if len(runes) > maxDetailRunes {
		message = string(runes[:maxDetailRunes]) + "..."
	}
	fmt.Fprintf(builder, "<br/>%d、%s", number, html.EscapeString(message))
}

// ImportUserTemplate 生成用户导入模板。
func ImportUserTemplate(w io.Writer) error {
	return excelx.Template[model.SysUser](w, "用户数据")
}

// firstValidationMessage 取首个校验失败字段的可读描述。
//
// 与 handler.bindMessage 是同一意图，但那个在 handler 包里、
// 依赖 gin 的错误类型；这里的输入来自 validate.Struct，单独处理。
func firstValidationMessage(err error) string {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		return fmt.Sprintf("%s 不合法", verrs[0].Field())
	}
	return "数据格式不正确"
}

// messageOf 取面向用户的错误文案。
func messageOf(err error) string {
	if bizErr := errs.As(err); bizErr != nil {
		return bizErr.Msg
	}
	return "系统异常"
}
