package handler

import "ruoyi-go/internal/model"

// 这些转换只服务响应层：数据库模型仍保留适合写入和校验的 Go 类型，
// 对外响应则复刻 Java MyBatis 查询实际填充的字段和 Jackson 的 null 语义。

func cachedJavaParams() map[string]any {
	// Java 登录会话和字典缓存由 Fastjson 反序列化，空 HashMap 会带上类型标记。
	return map[string]any{"@type": "java.util.HashMap"}
}

func contractRole(role model.SysRole) map[string]any {
	result := roleMap(role)
	// Java selectRoleVo 没有选择这三列。
	result["createBy"] = nil
	result["updateBy"] = nil
	result["updateTime"] = nil
	return result
}

func contractRoles(roles []model.SysRole) []map[string]any {
	result := make([]map[string]any, 0, len(roles))
	for _, role := range roles {
		result = append(result, contractRole(role))
	}
	return result
}

func contractPost(post model.SysPost) map[string]any {
	result := postMap(post)
	// Java selectPostVo 没有选择更新者和更新时间。
	result["updateBy"] = nil
	result["updateTime"] = nil
	return result
}

func contractPosts(posts []model.SysPost) []map[string]any {
	result := make([]map[string]any, 0, len(posts))
	for _, post := range posts {
		result = append(result, contractPost(post))
	}
	return result
}

// contractDept 部门**列表**用（`selectDeptVo`）。
//
// 详情走的是另一条 SQL，列不一样，见 contractDeptDetail。
func contractDept(dept model.SysDept) map[string]any {
	item := deptMap(dept)
	item["parentName"] = nil
	item["children"] = []any{}
	item["updateBy"] = nil
	item["updateTime"] = nil
	item["remark"] = nil
	return item
}

func contractDeptList(depts []model.SysDept) []map[string]any {
	result := make([]map[string]any, 0, len(depts))
	for _, dept := range depts {
		result = append(result, contractDept(dept))
	}
	return result
}

// contractDeptDetail 部门**详情**用。
//
// 【这是全项目唯一一处详情和列表选列不同的接口，不要图省事复用 contractDept】
// Java 的 selectDeptById 没有 include selectDeptVo，而是自己写了一条：
//
//	select d.dept_id, d.parent_id, d.ancestors, d.dept_name, d.order_num,
//	       d.leader, d.phone, d.email, d.status,
//	       (select dept_name from sys_dept where dept_id = d.parent_id) parent_name
//	from sys_dept d where d.dept_id = #{deptId}
//
// 和列表比：**少了** create_by / create_time / del_flag（列表里 selectDeptVo 有），
// **多了** parent_name（列表里没有，固定 null）。两个方向都有差异。
func contractDeptDetail(dept model.SysDept) map[string]any {
	item := deptMap(dept)
	item["children"] = []any{}
	// 列表选了这三列，详情没选
	item["createBy"] = nil
	item["createTime"] = nil
	item["delFlag"] = nil
	// 两边都没选
	item["updateBy"] = nil
	item["updateTime"] = nil
	item["remark"] = nil
	// 详情独有：模型上是 omitempty，根部门时键会整个消失，
	// 而 Java 的子查询取不到父部门时输出 null，键是在的
	item["parentName"] = nil
	if dept.ParentName != "" {
		item["parentName"] = dept.ParentName
	}
	return item
}

func contractMenu(menu model.SysMenu) map[string]any {
	item := menuMap(menu)
	item["parentName"] = nil
	item["children"] = []any{}
	item["createBy"] = nil
	item["updateBy"] = nil
	item["updateTime"] = nil
	item["remark"] = nil
	// Java 是 ifnull(perms,'') as perms，取不到时是空串不是 null
	if menu.Perms == nil {
		item["perms"] = ""
	}
	return item
}

func contractMenuList(menus []model.SysMenu) []map[string]any {
	result := make([]map[string]any, 0, len(menus))
	for _, menu := range menus {
		result = append(result, contractMenu(menu))
	}
	return result
}

func contractDictType(dictType model.SysDictType) map[string]any {
	item := dictTypeMap(dictType)
	item["updateBy"] = nil
	item["updateTime"] = nil
	return item
}

func contractDictTypes(items []model.SysDictType) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, dictType := range items {
		result = append(result, contractDictType(dictType))
	}
	return result
}

func contractDictDatum(data model.SysDictData) map[string]any {
	item := dictDataMap(data)
	item["updateBy"] = nil
	item["updateTime"] = nil
	return item
}

func contractDictData(items []model.SysDictData) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, data := range items {
		result = append(result, contractDictDatum(data))
	}
	return result
}

func contractUserList(users []model.SysUser) []map[string]any {
	result := make([]map[string]any, 0, len(users))
	for _, user := range users {
		item := contractUser(user, false)
		result = append(result, item)
	}
	return result
}

func contractUser(user model.SysUser, full bool) map[string]any {
	result := userMap(user)
	delete(result, "userType") // Java SysUser 没有该响应属性。
	result["roleId"] = nil

	if full {
		roles := make([]map[string]any, 0, len(user.Roles))
		for _, role := range user.Roles {
			roles = append(roles, contractEmbeddedRole(role, false))
		}
		result["roles"] = roles
		if user.Dept != nil {
			result["dept"] = contractUserDept(*user.Dept, true)
		}
		return result
	}

	// Java selectUserList 没有选择这些列。
	result["pwdUpdateDate"] = nil
	result["updateBy"] = nil
	result["updateTime"] = nil
	result["roles"] = []any{}
	if user.Dept != nil {
		result["dept"] = contractUserDept(*user.Dept, false)
	}
	return result
}

// contractLoginUser 复刻 Redis 登录会话中的 Java SysUser。该对象经过
// Fastjson 反序列化，和直接从 MyBatis 查询出来的用户存在可观察差异。
func contractLoginUser(user model.SysUser) map[string]any {
	result := contractUser(user, true)
	result["params"] = cachedJavaParams()
	if user.Avatar == "" {
		result["avatar"] = nil
	}
	if user.UpdateBy == "" {
		result["updateBy"] = nil
	}
	if user.Dept != nil {
		dept := contractUserDept(*user.Dept, true)
		dept["params"] = cachedJavaParams()
		result["dept"] = dept
	}
	roles := make([]map[string]any, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, contractEmbeddedRole(role, true))
	}
	result["roles"] = roles
	return result
}

func contractEmbeddedRole(role model.SysRole, cached bool) map[string]any {
	result := roleMap(role)
	// SysUserMapper.RoleResult 只填充这六个业务字段；其余保持 Java 零值。
	result["menuCheckStrictly"] = false
	result["deptCheckStrictly"] = false
	result["delFlag"] = nil
	result["createBy"] = nil
	result["createTime"] = nil
	result["updateBy"] = nil
	result["updateTime"] = nil
	result["remark"] = nil
	if cached {
		result["params"] = cachedJavaParams()
	}
	return result
}

func contractUserDept(dept model.SysDept, full bool) map[string]any {
	result := map[string]any{
		"deptId":     dept.DeptID,
		"parentId":   nil,
		"ancestors":  nil,
		"deptName":   dept.DeptName,
		"orderNum":   nil,
		"leader":     dept.Leader,
		"phone":      nil,
		"email":      nil,
		"status":     nil,
		"delFlag":    nil,
		"parentName": nil,
		"children":   []any{},
		"createBy":   nil,
		"createTime": nil,
		"updateBy":   nil,
		"updateTime": nil,
		"remark":     nil,
	}
	if full {
		result["parentId"] = dept.ParentID
		result["ancestors"] = dept.Ancestors
		result["orderNum"] = dept.OrderNum
		result["status"] = dept.Status
	}
	return result
}

func contractNotices(items []model.SysNotice) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, notice := range items {
		item := noticeMap(notice)
		result = append(result, item)
	}
	return result
}

// 以下 map 构造器刻意逐字段列出响应模型，避免先 Marshal 再 Unmarshal 的重复开销，
// 也避免序列化失败被忽略后静默返回空对象。模型字段变更时必须同步更新这里和契约测试。

func roleMap(role model.SysRole) map[string]any {
	return map[string]any{
		"roleId": role.RoleID, "roleName": role.RoleName, "roleKey": role.RoleKey,
		"roleSort": role.RoleSort, "dataScope": role.DataScope,
		"menuCheckStrictly": role.MenuCheckStrictly, "deptCheckStrictly": role.DeptCheckStrictly,
		"status": role.Status, "delFlag": role.DelFlag,
		"createBy": role.CreateBy, "createTime": role.CreateTime,
		"updateBy": role.UpdateBy, "updateTime": role.UpdateTime, "remark": role.Remark,
		"flag": role.Flag, "permissions": role.Permissions,
		"menuIds": role.MenuIDs, "deptIds": role.DeptIDs, "admin": role.IsAdmin(),
	}
}

func postMap(post model.SysPost) map[string]any {
	return map[string]any{
		"postId": post.PostID, "postCode": post.PostCode, "postName": post.PostName,
		"postSort": post.PostSort, "status": post.Status,
		"createBy": post.CreateBy, "createTime": post.CreateTime,
		"updateBy": post.UpdateBy, "updateTime": post.UpdateTime,
		"remark": post.Remark, "flag": post.Flag,
	}
}

func deptMap(dept model.SysDept) map[string]any {
	return map[string]any{
		"deptId": dept.DeptID, "parentId": dept.ParentID, "ancestors": dept.Ancestors,
		"deptName": dept.DeptName, "orderNum": dept.OrderNum,
		"leader": dept.Leader, "phone": dept.Phone, "email": dept.Email,
		"status": dept.Status, "delFlag": dept.DelFlag,
		"createBy": dept.CreateBy, "createTime": dept.CreateTime,
		"updateBy": dept.UpdateBy, "updateTime": dept.UpdateTime,
	}
}

func menuMap(menu model.SysMenu) map[string]any {
	return map[string]any{
		"menuId": menu.MenuID, "menuName": menu.MenuName, "parentId": menu.ParentID,
		"orderNum": menu.OrderNum, "path": menu.Path, "component": menu.Component,
		"query": menu.Query, "routeName": menu.RouteName,
		"isFrame": menu.IsFrame, "isCache": menu.IsCache, "menuType": menu.MenuType,
		"visible": menu.Visible, "status": menu.Status, "perms": menu.Perms, "icon": menu.Icon,
		"createBy": menu.CreateBy, "createTime": menu.CreateTime,
		"updateBy": menu.UpdateBy, "updateTime": menu.UpdateTime, "remark": menu.Remark,
	}
}

func dictTypeMap(item model.SysDictType) map[string]any {
	return map[string]any{
		"dictId": item.DictID, "dictName": item.DictName, "dictType": item.DictType,
		"status": item.Status, "createBy": item.CreateBy, "createTime": item.CreateTime,
		"updateBy": item.UpdateBy, "updateTime": item.UpdateTime, "remark": item.Remark,
	}
}

func dictDataMap(item model.SysDictData) map[string]any {
	return map[string]any{
		"dictCode": item.DictCode, "dictSort": item.DictSort,
		"dictLabel": item.DictLabel, "dictValue": item.DictValue, "dictType": item.DictType,
		"cssClass": item.CssClass, "listClass": item.ListClass,
		"isDefault": item.IsDefault, "status": item.Status,
		"createBy": item.CreateBy, "createTime": item.CreateTime,
		"updateBy": item.UpdateBy, "updateTime": item.UpdateTime,
		"remark": item.Remark, "default": item.Default,
	}
}

func userMap(user model.SysUser) map[string]any {
	result := map[string]any{
		"userId": user.UserID, "deptId": user.DeptID,
		"userName": user.UserName, "nickName": user.NickName, "userType": user.UserType,
		"email": user.Email, "phonenumber": user.Phonenumber, "sex": user.Sex,
		"avatar": user.Avatar, "status": user.Status, "delFlag": user.DelFlag,
		"loginIp": user.LoginIP, "loginDate": user.LoginDate, "pwdUpdateDate": user.PwdUpdateDate,
		"createBy": user.CreateBy, "createTime": user.CreateTime,
		"updateBy": user.UpdateBy, "updateTime": user.UpdateTime, "remark": user.Remark,
		"roles": user.Roles, "roleIds": user.RoleIDs, "postIds": user.PostIDs,
		"admin": user.IsAdmin(),
	}
	if user.Dept != nil {
		result["dept"] = user.Dept
	}
	return result
}

func noticeMap(notice model.SysNotice) map[string]any {
	return map[string]any{
		"noticeId": notice.NoticeID, "noticeTitle": notice.NoticeTitle,
		"noticeType": notice.NoticeType, "noticeContent": notice.NoticeContent,
		"status": notice.Status, "createBy": notice.CreateBy, "createTime": notice.CreateTime,
		"updateBy": notice.UpdateBy, "updateTime": notice.UpdateTime,
		"remark": notice.Remark, "isRead": notice.IsRead,
	}
}

func contractCachedDictData(items []model.SysDictData) []map[string]any {
	result := contractDictData(items)
	for index := range result {
		if items[index].CssClass == nil || *items[index].CssClass == "" {
			result[index]["cssClass"] = nil
		}
		result[index]["params"] = cachedJavaParams()
	}
	return result
}
