package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

// DeptList GET /system/dept/list
//
// 【注意】部门列表返回的是 AjaxResult 不是 TableDataInfo ——
// 数据在 data 里，没有 total/rows，也不分页（前端自己 handleTree 成树）。
func DeptList(c *gin.Context) {
	var query model.DeptQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}

	list, err := service.ListDepts(c.Request.Context(), currentUser(c), query)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractDeptList(list))
}

// DeptExcludeChild GET /system/dept/list/exclude/:deptId
//
// 修改部门时的上级下拉，需排除自身及所有下级，否则会选出环。
func DeptExcludeChild(c *gin.Context) {
	id, err := parseID(c.Param("deptId"))
	if err != nil {
		fail(c, err)
		return
	}

	list, err := service.ListDeptsExcludeChild(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractDeptList(list))
}

// DeptGet GET /system/dept/:deptId
func DeptGet(c *gin.Context) {
	id, err := parseID(c.Param("deptId"))
	if err != nil {
		fail(c, err)
		return
	}

	dept, err := service.GetDept(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	// 【详情和列表的选列不一样】selectDeptById 是单独写的一条 SQL，
	// 比列表少了 create_by/create_time/del_flag，却多了 parent_name
	response.OkData(c, contractDeptDetail(*dept))
}

// DeptAdd POST /system/dept
func DeptAdd(c *gin.Context) {
	var dept model.SysDept
	if err := c.ShouldBindJSON(&dept); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateDept(c.Request.Context(), currentUser(c), &dept, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DeptEdit PUT /system/dept
func DeptEdit(c *gin.Context) {
	var dept model.SysDept
	if err := c.ShouldBindJSON(&dept); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateDept(c.Request.Context(), currentUser(c), &dept, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DeptUpdateSort PUT /system/dept/updateSort
func DeptUpdateSort(c *gin.Context) {
	var body model.DeptSortBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateDeptSort(c.Request.Context(), currentUser(c), body); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// DeptRemove DELETE /system/dept/:deptId
//
// 与其他模块不同，部门是单个删除，不支持逗号批量。
func DeptRemove(c *gin.Context) {
	id, err := parseID(c.Param("deptId"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteDept(c.Request.Context(), currentUser(c), id); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// currentUser 取当前登录用户实体，未登录返回 nil。
//
// 数据权限依赖 User.Roles 里的 Permissions，这份数据来自 Redis 会话。
func currentUser(c *gin.Context) *model.SysUser {
	loginUser := middleware.CurrentUser(c)
	if loginUser == nil {
		return nil
	}
	return loginUser.User
}
