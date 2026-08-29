package handler

import (
	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
)

// MenuList GET /system/menu/list
//
// 与部门列表一样返回 AjaxResult 而不是 TableDataInfo：数据在 data 里，不分页。
func MenuList(c *gin.Context) {
	var query model.MenuQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}

	list, err := service.ListMenus(c.Request.Context(), currentUser(c), query)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, contractMenuList(list))
}

// MenuTreeSelect GET /system/menu/treeselect
func MenuTreeSelect(c *gin.Context) {
	var query model.MenuQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}

	tree, err := service.MenuTreeSelect(c.Request.Context(), currentUser(c), query)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, tree)
}

// MenuRoleTreeSelect GET /system/menu/roleMenuTreeselect/:roleId
//
// 【平铺字段】checkedKeys 和 menus 都在顶层，形态同 role/deptTree。
func MenuRoleTreeSelect(c *gin.Context) {
	id, err := parseID(c.Param("roleId"))
	if err != nil {
		fail(c, err)
		return
	}

	menus, checkedKeys, err := service.RoleMenuTreeSelect(c.Request.Context(), currentUser(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("checkedKeys", checkedKeys).
		Put("menus", menus).
		JSON(c)
}

// MenuGet GET /system/menu/:menuId
func MenuGet(c *gin.Context) {
	id, err := parseID(c.Param("menuId"))
	if err != nil {
		fail(c, err)
		return
	}
	menu, err := service.GetMenu(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	// selectMenuById 走的是 selectMenuVo，与列表同一套选列
	response.OkData(c, contractMenu(*menu))
}

// MenuAdd POST /system/menu
func MenuAdd(c *gin.Context) {
	var menu model.SysMenu
	if err := c.ShouldBindJSON(&menu); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.CreateMenu(c.Request.Context(), currentUser(c), &menu, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// MenuEdit PUT /system/menu
func MenuEdit(c *gin.Context) {
	var menu model.SysMenu
	if err := c.ShouldBindJSON(&menu); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateMenu(c.Request.Context(), currentUser(c), &menu, currentUsername(c)); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// MenuUpdateSort PUT /system/menu/updateSort
func MenuUpdateSort(c *gin.Context) {
	var body model.MenuSortBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	if err := service.UpdateMenuSort(c.Request.Context(), currentUser(c), body); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// MenuRemove DELETE /system/menu/:menuId
//
// 菜单是单个删除，不支持逗号批量。
func MenuRemove(c *gin.Context) {
	id, err := parseID(c.Param("menuId"))
	if err != nil {
		fail(c, err)
		return
	}
	if err := service.DeleteMenu(c.Request.Context(), currentUser(c), id); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}
