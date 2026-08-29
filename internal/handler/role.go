package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/excelx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/response"
)

// RoleList GET /system/role/list
func RoleList(c *gin.Context) {
	var query model.RoleQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.RoleSortColumns)

	list, total, err := service.ListRolePage(c.Request.Context(), currentUser(c), query, pg)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, contractRoles(list), total)
}

// RoleExport POST /system/role/export
func RoleExport(c *gin.Context) {
	var query model.RoleQuery
	if err := c.ShouldBind(&query); err != nil {
		response.FailDownload(c, response.CodeError, bindMessage(err))
		return
	}

	list, err := service.ListRoleExport(c.Request.Context(), currentUser(c), query)
	if err != nil {
		failDownload(c, err)
		return
	}
	if err := excelx.WriteResponse(c, "角色数据.xlsx", "角色数据", list); err != nil {
		failDownload(c, err)
		return
	}
}

// RoleOptionSelect GET /system/role/optionselect
func RoleOptionSelect(c *gin.Context) {
	list, err := service.ListRoleAll(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractRoles(list))
}

// RoleDeptTree GET /system/role/deptTree/:roleId
//
// 【平铺字段】depts 和 checkedKeys 都在顶层，不在 data 里。
// 同类的 /system/menu/roleMenuTreeselect/:roleId 也是这个形态。
func RoleDeptTree(c *gin.Context) {
	id, err := parseID(c.Param("roleId"))
	if err != nil {
		fail(c, err)
		return
	}

	depts, checkedKeys, err := service.RoleDeptTree(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("checkedKeys", checkedKeys).
		Put("depts", depts).
		JSON(c)
}

// RoleAuthUserAllocated GET /system/role/authUser/allocatedList
func RoleAuthUserAllocated(c *gin.Context) {
	authUserList(c, true)
}

// RoleAuthUserUnallocated GET /system/role/authUser/unallocatedList
func RoleAuthUserUnallocated(c *gin.Context) {
	authUserList(c, false)
}

func authUserList(c *gin.Context, allocated bool) {
	roleID, err := parseID(c.Query("roleId"))
	if err != nil {
		fail(c, err)
		return
	}
	var query model.UserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	pg := page.Parse(c, model.UserSortColumns)

	list, total, err := service.ListAuthUserPage(c.Request.Context(), currentUser(c), roleID, query, pg, allocated)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, list, total)
}

// RoleAuthUserCancel PUT /system/role/authUser/cancel
//
// 请求体是 {userId, roleId}，取消单个用户的授权。
func RoleAuthUserCancel(c *gin.Context) {
	var body model.SysUserRole
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CancelAuthUser(c.Request.Context(), currentUser(c), body.RoleID, []int64{body.UserID}); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleAuthUserCancelAll PUT /system/role/authUser/cancelAll?roleId=2&userIds=1,2
//
// 参数在 query string 里，不是请求体。
func RoleAuthUserCancelAll(c *gin.Context) {
	roleID, userIDs, err := authUserParams(c)
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.CancelAuthUser(c.Request.Context(), currentUser(c), roleID, userIDs); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleAuthUserSelectAll PUT /system/role/authUser/selectAll?roleId=2&userIds=1,2
func RoleAuthUserSelectAll(c *gin.Context) {
	roleID, userIDs, err := authUserParams(c)
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.GrantAuthUser(c.Request.Context(), currentUser(c), roleID, userIDs); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

func authUserParams(c *gin.Context) (int64, []int64, error) {
	roleID, err := parseID(c.Query("roleId"))
	if err != nil {
		return 0, nil, err
	}
	userIDs, err := parseIDs(c.Query("userIds"))
	if err != nil {
		return 0, nil, err
	}
	return roleID, userIDs, nil
}

// RoleGet GET /system/role/:roleId
func RoleGet(c *gin.Context) {
	id, err := parseID(c.Param("roleId"))
	if err != nil {
		fail(c, err)
		return
	}

	role, err := service.GetRole(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	// selectRoleById 走的是 selectRoleVo，没选 create_by / update_by / update_time
	response.OkData(c, contractRole(*role))
}

// RoleAdd POST /system/role
func RoleAdd(c *gin.Context) {
	var role model.SysRole
	if err := c.ShouldBindJSON(&role); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateRole(c.Request.Context(), currentUser(c), &role, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleEdit PUT /system/role
func RoleEdit(c *gin.Context) {
	var role model.SysRole
	if err := c.ShouldBindJSON(&role); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateRole(c.Request.Context(), currentUser(c), &role, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleDataScope PUT /system/role/dataScope
//
// 局部更新接口，用专用 DTO 而不是实体 —— 实体的 notblank 会拒掉只带
// roleId/dataScope 的请求。
func RoleDataScope(c *gin.Context) {
	var body model.RoleDataScopeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.AuthDataScope(c.Request.Context(), currentUser(c), body, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleChangeStatus PUT /system/role/changeStatus
func RoleChangeStatus(c *gin.Context) {
	var body model.RoleStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.ChangeRoleStatus(c.Request.Context(), currentUser(c), body.RoleID, body.Status, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// RoleRemove DELETE /system/role/:roleIds
func RoleRemove(c *gin.Context) {
	ids, err := parseIDs(c.Param("roleIds"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteRoles(c.Request.Context(), currentUser(c), ids); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
