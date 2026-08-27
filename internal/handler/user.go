package handler

import (
	"bytes"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// UserList GET /system/user/list
func UserList(c *gin.Context) {
	var query model.UserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.UserSortColumns)

	list, total, err := service.ListUserPage(c.Request.Context(), currentUser(c), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractUserList(list), total)
}

// UserExport POST /system/user/export
func UserExport(c *gin.Context) {
	var query model.UserQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListUserExport(c.Request.Context(), currentUser(c), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "用户数据.xlsx", "用户数据", list); err != nil {
		failDownload(c, err)
		return
	}
}

// UserImportData POST /system/user/importData
//
// 表单字段名是 file，updateSupport 在 query string 里（前端 upload 组件的 data 参数）。
// 返回的 msg 里带 <br/>，前端用 v-html 渲染。
func UserImportData(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, "请选择要导入的文件")
		return
	}
	// 必须在解析前挡住大文件：excelx.Import 会把全部行读进内存，
	// service 里的行数上限是解析完才生效的，那时内存已经吃满了。
	if maxSize := uploadCfg.MaxSizeMB * 1024 * 1024; maxSize > 0 && fileHeader.Size > maxSize {
		response.Fail(c, "导入文件大小超过上限 "+strconv.FormatInt(uploadCfg.MaxSizeMB, 10)+" MB")
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		response.Fail(c, "读取上传文件失败")
		return
	}
	defer src.Close()

	updateSupport := c.Query("updateSupport") == "true" || c.PostForm("updateSupport") == "true"

	msg, err := service.ImportUsers(c.Request.Context(), currentUser(c), src, updateSupport, currentUsername(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, msg)
}

// UserImportTemplate POST /system/user/importTemplate
//
// 返回只有表头的 xlsx，含标了 type:import 的列（如部门编号）。
func UserImportTemplate(c *gin.Context) {
	var buf bytes.Buffer
	if err := service.ImportUserTemplate(&buf); err != nil {
		failDownload(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="user_template.xlsx"`)
	c.Data(http.StatusOK, excelx.ContentType, buf.Bytes())
}

// UserDeptTree GET /system/user/deptTree
func UserDeptTree(c *gin.Context) {
	depts, err := service.ListDepts(c.Request.Context(), currentUser(c), model.DeptQuery{})
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, service.BuildDeptTreeSelect(depts))
}

// UserGet 处理 GET /system/user/ 和 GET /system/user/:userId
//
// 【混合形态】data 里是用户对象，同时平铺 postIds / roleIds / roles / posts。
// 不带 userId 时（新增用户弹窗）只返回 roles 和 posts，没有 data。
func UserGet(c *gin.Context) {
	ctx := c.Request.Context()
	operator := currentUser(c)

	posts, err := service.ListPostAll(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	roles, err := service.ListRoleAll(ctx)
	if err != nil {
		fail(c, err)
		return
	}

	raw := c.Param("userId")
	result := response.New(response.CodeSuccess, response.MsgSuccess)

	if raw == "" || raw == "/" {
		// 新增场景：没有 data，也没有 postIds / roleIds
		result.
			Put("roles", contractRoles(filterAssignableRoles(roles, 0))).
			Put("posts", contractPosts(posts)).
			JSON(c)
		return
	}

	userID, err := parseID(raw)
	if err != nil {
		fail(c, err)
		return
	}
	user, postIDs, err := service.GetUser(ctx, operator, userID)
	if err != nil {
		fail(c, err)
		return
	}

	roleIDs := make([]int64, 0, len(user.Roles))
	for _, role := range user.Roles {
		roleIDs = append(roleIDs, role.RoleID)
	}

	result.
		Put("data", contractUser(*user, true)).
		Put("postIds", postIDs).
		Put("roleIds", roleIDs).
		Put("roles", contractRoles(filterAssignableRoles(roles, userID))).
		Put("posts", contractPosts(posts)).
		JSON(c)
}

// filterAssignableRoles 剔除 admin 角色。
//
// 只有 admin 用户自己能看到 admin 角色，否则任何有用户编辑权的人
// 都能把别人提成超级管理员。
func filterAssignableRoles(roles []model.SysRole, userID int64) []model.SysRole {
	if userID == model.AdminUserID {
		return roles
	}
	result := make([]model.SysRole, 0, len(roles))
	for _, role := range roles {
		if role.IsAdmin() {
			continue
		}
		result = append(result, role)
	}
	return result
}

// UserAdd POST /system/user
func UserAdd(c *gin.Context) {
	var user model.SysUser
	if err := c.ShouldBindJSON(&user); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateUser(c.Request.Context(), currentUser(c), &user, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UserEdit PUT /system/user
func UserEdit(c *gin.Context) {
	var user model.SysUser
	if err := c.ShouldBindJSON(&user); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateUser(c.Request.Context(), currentUser(c), &user, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UserResetPwd PUT /system/user/resetPwd
func UserResetPwd(c *gin.Context) {
	var body model.UserResetPwdBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.ResetUserPwd(c.Request.Context(), currentUser(c), body.UserID, body.Password, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UserChangeStatus PUT /system/user/changeStatus
func UserChangeStatus(c *gin.Context) {
	var body model.UserStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.ChangeUserStatus(c.Request.Context(), currentUser(c), body.UserID, body.Status, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UserAuthRoleGet GET /system/user/authRole/:userId
//
// 【平铺字段】user 和 roles 都在顶层，不在 data 里。
func UserAuthRoleGet(c *gin.Context) {
	id, err := parseID(c.Param("userId"))
	if err != nil {
		fail(c, err)
		return
	}

	user, roles, err := service.AuthRoleOfUser(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("user", user).
		Put("roles", roles).
		JSON(c)
}

// UserAuthRoleSave PUT /system/user/authRole?userId=1&roleIds=2,3
//
// 【注意】参数在 query string 里，不是请求体 —— 前端用的是 axios 的 params。
// roleIds 是逗号拼接的字符串。
func UserAuthRoleSave(c *gin.Context) {
	userID, err := parseID(c.Query("userId"))
	if err != nil {
		fail(c, err)
		return
	}

	var roleIDs []int64
	if raw := c.Query("roleIds"); raw != "" {
		roleIDs, err = parseIDs(raw)
		if err != nil {
			fail(c, err)
			return
		}
	}

	if err := service.AssignUserRoles(c.Request.Context(), currentUser(c), userID, roleIDs); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UserRemove DELETE /system/user/:userIds
func UserRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("userIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteUsers(c.Request.Context(), currentUser(c), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
