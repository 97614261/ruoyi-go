package apitest

import (
	"fmt"
	"net/url"
	"testing"
)

// adminRoleID 超级管理员角色，任何写操作都必须被拒。
const adminRoleID = 1

func newRolePayload(suffix string) map[string]any {
	return map[string]any{
		"roleName":          testPrefix + "角色" + suffix,
		"roleKey":           "zz_test_role_" + suffix,
		"roleSort":          9,
		"status":            "0",
		"menuIds":           []int64{1, 100, 1001},
		"menuCheckStrictly": true,
		"deptCheckStrictly": true,
		"remark":            "测试角色",
	}
}

func createRole(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/role", body), "新增角色")

	name := fmt.Sprint(body["roleName"])
	list := pageRows(t, doGet(t, "/system/role/list?pageSize=100&roleName="+url.QueryEscape(name)), "查角色")
	item := findBy(list, "roleName", name)
	if item == nil {
		t.Fatalf("新增角色后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "roleId")

	t.Cleanup(func() { _ = doDelete(t, "/system/role/"+idPath(id)) })
	return id
}

// TestRoleCRUD 角色增删改查。
func TestRoleCRUD(t *testing.T) {
	body := newRolePayload("crud")
	id := createRole(t, body)
	path := "/system/role/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查角色")
	assertField(t, detail, "roleName", body["roleName"], "新增后")
	assertField(t, detail, "roleKey", body["roleKey"], "新增后")
	assertField(t, detail, "roleSort", 9, "新增后")
	assertString(t, detail, "角色详情", "status")
	// Java 的 isAdmin() 会被 Jackson 序列化成 admin 字段，前端读它
	if _, ok := detail["admin"]; !ok {
		t.Errorf("角色详情应带 admin 字段（Java 的 isAdmin() getter），实际字段=%v", topKeys(detail))
	}
	assertField(t, detail, "admin", false, "普通角色的 admin 应为 false")

	// 菜单权限要能查回来
	menuTree := doGet(t, "/system/menu/roleMenuTreeselect/"+idPath(id))
	mustOK(t, menuTree, "查角色的菜单树")
	checked, ok := menuTree.Raw["checkedKeys"].([]any)
	if !ok || len(checked) == 0 {
		t.Errorf("新增时勾选了菜单，checkedKeys 不该为空，实际 %v", menuTree.Raw["checkedKeys"])
	}

	// 改：排序改 0
	updated := payload(body)
	updated["roleId"] = id
	updated["roleSort"] = 0
	updated["roleName"] = testPrefix + "角色改名"
	mustOK(t, doPut(t, "/system/role", updated), "修改角色")

	detail = dataObject(t, doGet(t, path), "改后查角色")
	assertField(t, detail, "roleSort", 0, "改后（排序 0 不能被跳过）")
	assertField(t, detail, "roleName", testPrefix+"角色改名", "改后")

	mustOK(t, doDelete(t, path), "删除角色")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestRoleChangeStatus 局部更新接口：只传 roleId 和 status。
//
// 这类接口不能绑到实体上 —— 实体的 notblank 会把只带两个字段的请求直接拒掉。
func TestRoleChangeStatus(t *testing.T) {
	id := createRole(t, newRolePayload("status"))

	mustOK(t, doPut(t, "/system/role/changeStatus", map[string]any{
		"roleId": id,
		"status": "1",
	}), "停用角色（只传两个字段）")

	detail := dataObject(t, doGet(t, "/system/role/"+idPath(id)), "查角色状态")
	assertField(t, detail, "status", "1", "停用后")

	mustOK(t, doPut(t, "/system/role/changeStatus", map[string]any{
		"roleId": id,
		"status": "0",
	}), "启用角色")
}

// TestRoleDataScope 分配数据权限，同样是局部更新。
func TestRoleDataScope(t *testing.T) {
	id := createRole(t, newRolePayload("scope"))

	mustOK(t, doPut(t, "/system/role/dataScope", map[string]any{
		"roleId":            id,
		"dataScope":         "2", // 自定义
		"deptIds":           []int64{100, 101},
		"deptCheckStrictly": true,
	}), "分配自定义数据权限")

	detail := dataObject(t, doGet(t, "/system/role/"+idPath(id)), "查角色数据权限")
	assertField(t, detail, "dataScope", "2", "分配后")

	// 部门树接口要能回显勾选
	r := doGet(t, "/system/role/deptTree/"+idPath(id))
	mustOK(t, r, "角色部门树")
	checked, ok := r.Raw["checkedKeys"].([]any)
	if !ok || len(checked) == 0 {
		t.Errorf("分配了部门，checkedKeys 不该为空，实际 %v", r.Raw["checkedKeys"])
	}
}

// TestRoleDeptTreeContract deptTree 的平铺字段。
//
// 这是最典型的平铺形态：depts 和 checkedKeys 都在顶层，没有 data 包裹。
func TestRoleDeptTreeContract(t *testing.T) {
	r := doGet(t, "/system/role/deptTree/2")
	mustOK(t, r, "角色部门树")

	assertTopLevel(t, r, "角色部门树", "code", "msg", "checkedKeys", "depts")
	assertNoTopLevel(t, r, "角色部门树", "data")

	depts := toObjects(t, r.Raw["depts"], "角色部门树的 depts")
	if len(depts) == 0 {
		t.Fatal("部门树不该为空")
	}
	// 叶子节点不能有 children 键
	assertTreeShape(t, depts, "角色部门树")
}

// TestRoleAdminProtected 超级管理员角色不允许被改、停用、删除。
func TestRoleAdminProtected(t *testing.T) {
	body := newRolePayload("admin")
	body["roleId"] = adminRoleID
	mustFail(t, doPut(t, "/system/role", body), "不允许操作超级管理员角色", "修改 admin 角色")

	mustFail(t, doPut(t, "/system/role/changeStatus", map[string]any{
		"roleId": adminRoleID, "status": "1",
	}), "不允许操作超级管理员角色", "停用 admin 角色")

	mustFail(t, doDelete(t, "/system/role/"+idPath(adminRoleID)), "不允许操作超级管理员角色", "删除 admin 角色")

	mustFail(t, doPut(t, "/system/role/dataScope", map[string]any{
		"roleId": adminRoleID, "dataScope": "5",
	}), "不允许操作超级管理员角色", "改 admin 角色的数据权限")
}

// TestRoleAuthUser 角色的分配用户流程。
func TestRoleAuthUser(t *testing.T) {
	roleID := createRole(t, newRolePayload("authuser"))
	userBody := newUserPayload("roleauth")
	userID := createUser(t, userBody)
	userName := fmt.Sprint(userBody["userName"])

	// 一开始该用户在"未分配"列表里
	if !inAuthUserList(t, "unallocatedList", roleID, userName, userID) {
		t.Fatalf("新建用户 %d 应出现在未分配列表里", userID)
	}

	// 授权
	mustOK(t, doPut(t, "/system/role/authUser/selectAll?roleId="+idPath(roleID)+"&userIds="+idPath(userID), nil), "批量授权")

	if !inAuthUserList(t, "allocatedList", roleID, userName, userID) {
		t.Errorf("授权后用户 %d 应出现在已分配列表里", userID)
	}

	// 重复授权不能报错（主键冲突要被忽略）
	mustOK(t, doPut(t, "/system/role/authUser/selectAll?roleId="+idPath(roleID)+"&userIds="+idPath(userID), nil), "重复授权")

	// 取消单个
	mustOK(t, doPut(t, "/system/role/authUser/cancel", map[string]any{
		"roleId": roleID, "userId": userID,
	}), "取消授权")

	if inAuthUserList(t, "allocatedList", roleID, userName, userID) {
		t.Errorf("取消授权后用户 %d 不该还在已分配列表里", userID)
	}
}

// TestRoleAuthUserCancelAll 批量取消授权使用 query 参数，并一次移除全部指定用户。
func TestRoleAuthUserCancelAll(t *testing.T) {
	roleID := createRole(t, newRolePayload("cancelall"))
	firstBody := newUserPayload("cancelall_1")
	secondBody := newUserPayload("cancelall_2")
	firstID := createUser(t, firstBody)
	secondID := createUser(t, secondBody)
	userIDs := idPath(firstID) + "," + idPath(secondID)

	mustOK(t, doPut(t, "/system/role/authUser/selectAll?roleId="+idPath(roleID)+"&userIds="+userIDs, nil), "批量授权两个用户")

	users := []struct {
		id   int64
		name string
	}{
		{firstID, fmt.Sprint(firstBody["userName"])},
		{secondID, fmt.Sprint(secondBody["userName"])},
	}
	for _, user := range users {
		if !inAuthUserList(t, "allocatedList", roleID, user.name, user.id) {
			t.Fatalf("批量授权后用户 %d 应出现在已分配列表里", user.id)
		}
	}

	mustOK(t, doPut(t, "/system/role/authUser/cancelAll?roleId="+idPath(roleID)+"&userIds="+userIDs, nil), "批量取消两个用户授权")

	for _, user := range users {
		if inAuthUserList(t, "allocatedList", roleID, user.name, user.id) {
			t.Errorf("批量取消后用户 %d 不该还在已分配列表里", user.id)
		}
		if !inAuthUserList(t, "unallocatedList", roleID, user.name, user.id) {
			t.Errorf("批量取消后用户 %d 应回到未分配列表里", user.id)
		}
	}
}

// inAuthUserList 判断某个用户在不在角色的已分配/未分配列表里。
//
// 【必须带 userName 条件精确查，不能拉一页回来比对】
// 库里用户上万时（比如刚灌过压测数据），要找的记录会翻到后面几页，
// 断言就会时对时错 —— 这个坑在 datascope_test.go 的 canSee 里已经踩过一次。
func inAuthUserList(t *testing.T, listName string, roleID int64, userName string, userID int64) bool {
	t.Helper()
	path := fmt.Sprintf("/system/role/authUser/%s?pageNum=1&pageSize=100&roleId=%d&userName=%s",
		listName, roleID, url.QueryEscape(userName))

	rows := pageRows(t, doGet(t, path), listName)
	return findBy(rows, "userId", userID) != nil
}

// TestRoleDeleteInUse 已分配给用户的角色不允许删除。
func TestRoleDeleteInUse(t *testing.T) {
	roleID := createRole(t, newRolePayload("inuse"))
	userID := createUser(t, newUserPayload("roleinuse"))

	mustOK(t, doPut(t, "/system/role/authUser/selectAll?roleId="+idPath(roleID)+"&userIds="+idPath(userID), nil), "授权")
	mustFail(t, doDelete(t, "/system/role/"+idPath(roleID)), "已分配,不能删除", "删除已分配用户的角色")
}

// TestRoleValidation 角色字段校验。
func TestRoleValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 roleName", func(p map[string]any) map[string]any { return omit(p, "roleName") }, "RoleName"},
		{"少传 roleKey", func(p map[string]any) map[string]any { return omit(p, "roleKey") }, "RoleKey"},
		{"少传 status", func(p map[string]any) map[string]any { return omit(p, "status") }, "Status"},
		{"roleName 纯空格", func(p map[string]any) map[string]any { return with(p, "roleName", "  ") }, "RoleName"},
		{"roleName 超过 30 字", func(p map[string]any) map[string]any { return with(p, "roleName", repeatText(31)) }, "RoleName"},
		{"roleKey 超过 100 字", func(p map[string]any) map[string]any { return with(p, "roleKey", repeatText(101)) }, "RoleKey"},
		{"roleSort = 0", func(p map[string]any) map[string]any { return with(p, "roleSort", 0) }, ""},
		{"roleSort 负数", func(p map[string]any) map[string]any { return with(p, "roleSort", -1) }, "RoleSort"},
		// 少传要拒（对齐 Java 的 @NotNull），传 0 要过 —— 见 post_test 的说明
		{"少传 roleSort", func(p map[string]any) map[string]any { return omit(p, "roleSort") }, "RoleSort"},
		{"少传 menuIds（不勾菜单）", func(p map[string]any) map[string]any { return omit(p, "menuIds") }, ""},
		{"menuIds 空数组", func(p map[string]any) map[string]any { return with(p, "menuIds", []int64{}) }, ""},
		{"remark 超过 500 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(501)) }, "Remark"},
		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newRolePayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/role", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestRoleUnique 角色名称和权限字符都不能重复。
func TestRoleUnique(t *testing.T) {
	body := newRolePayload("uniq")
	createRole(t, body)

	dupName := newRolePayload("uniq2")
	dupName["roleName"] = body["roleName"]
	mustFail(t, doPost(t, "/system/role", dupName), "角色名称已存在", "重名角色")

	dupKey := newRolePayload("uniq3")
	dupKey["roleKey"] = body["roleKey"]
	mustFail(t, doPost(t, "/system/role", dupKey), "角色权限已存在", "权限字符重复的角色")
}

// TestRoleOptionSelect 角色下拉。
func TestRoleOptionSelect(t *testing.T) {
	r := doGet(t, "/system/role/optionselect")
	mustOK(t, r, "角色下拉")
	dataArray(t, r, "角色下拉")
}

// TestRoleAllOrderedBySort 「全部角色」必须按 roleSort 排序。
//
// 【必须让 roleId 顺序和 roleSort 顺序相反，否则测了等于没测】
// 种子数据里 role_id 1,2 恰好对应 role_sort 1,2，两种顺序重合 ——
// 就算 SelectRoleAll 完全不排序，按物理顺序返回也能碰巧通过。
// 之前 Java/Go 双端对拍就是这么漏掉这个 bug 的。
//
// 所以这里先建 roleSort 大的、再建 roleSort 小的：
// 后建的 roleId 更大，但必须排在前面。
//
// 依据：Java 的 selectRoleAll() 不是独立 SQL，它转调 selectRoleList
// （SysRoleServiceImpl.java:112），而那条 mapper 结尾有 order by r.role_sort。
func TestRoleAllOrderedBySort(t *testing.T) {
	later := payload(newRolePayload("sortlater"))
	later["roleSort"] = 90
	laterID := createRole(t, later)

	earlier := payload(newRolePayload("sortearlier"))
	earlier["roleSort"] = 10
	earlierID := createRole(t, earlier)

	// 后建的 roleId 更大，这是构造这个用例的前提
	if earlierID <= laterID {
		t.Fatalf("测试前提不成立：后建角色的 roleId(%d) 应大于先建的(%d)", earlierID, laterID)
	}

	// 新增用户弹窗返回的 roles 走的就是 SelectRoleAll
	r := doGet(t, "/system/user")
	mustOK(t, r, "新增用户弹窗")
	roles := toObjects(t, r.Raw["roles"], "候选角色")

	posEarlier, posLater := -1, -1
	for i, role := range roles {
		switch idOf(t, role, "roleId") {
		case earlierID:
			posEarlier = i
		case laterID:
			posLater = i
		}
	}
	if posEarlier < 0 || posLater < 0 {
		t.Fatalf("两个测试角色都应出现在候选列表里，实际位置 %d / %d", posEarlier, posLater)
	}
	if posEarlier > posLater {
		t.Errorf("roleSort=10 的角色应排在 roleSort=90 的前面，实际位置 %d > %d —— "+
			"SelectRoleAll 丢了 ORDER BY role_sort", posEarlier, posLater)
	}
}
