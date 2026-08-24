package apitest

import (
	"fmt"
	"testing"
)

func newDirPayload(suffix string) map[string]any {
	return map[string]any{
		"menuName": testPrefix + "目录" + suffix,
		"parentId": 0,
		"orderNum": 90,
		"path":     testPrefix + "dir" + suffix,
		"menuType": "M",
		"isFrame":  "1", // 字符串，不是数字
		"isCache":  "0",
		"visible":  "0",
		"status":   "0",
		"icon":     "#",
	}
}

func newButtonPayload(parentID int64, suffix string) map[string]any {
	return map[string]any{
		"menuName": testPrefix + "按钮" + suffix,
		"parentId": parentID,
		"orderNum": 1,
		"menuType": "F",
		"perms":    "zztest:demo:" + suffix,
		"isFrame":  "1",
		"isCache":  "0",
		"visible":  "0",
		"status":   "0",
		// 按钮没有 path 和 routeName，这正是路由冲突检查最容易误伤的地方
	}
}

func createMenu(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/menu", body), "新增菜单 "+fmt.Sprint(body["menuName"]))

	name := fmt.Sprint(body["menuName"])
	list := dataArray(t, doGet(t, "/system/menu/list"), "菜单列表")
	item := findBy(list, "menuName", name)
	if item == nil {
		t.Fatalf("新增菜单后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "menuId")

	t.Cleanup(func() { _ = doDelete(t, "/system/menu/"+idPath(id)) })
	return id
}

// TestMenuButtonCanBeCreatedRepeatedly 连续新增多个按钮。
//
// 【这是回归测试】路由冲突检查的 SQL 必须带 menu_type IN ('M','C')，
// 否则按钮之间会因为 path 和 route_name 同为空串而互相判定冲突 ——
// 库里只要已有一个按钮，第二个就永远加不进去。
func TestMenuButtonCanBeCreatedRepeatedly(t *testing.T) {
	dirID := createMenu(t, newDirPayload("btn"))

	// 连加三个按钮，全都必须成功
	for i := 1; i <= 3; i++ {
		suffix := fmt.Sprintf("b%d", i)
		body := newButtonPayload(dirID, suffix)
		r := doPost(t, "/system/menu", body)
		mustOK(t, r, fmt.Sprintf("新增第 %d 个按钮", i))

		list := dataArray(t, doGet(t, "/system/menu/list"), "菜单列表")
		item := findBy(list, "menuName", body["menuName"])
		if item == nil {
			t.Fatalf("第 %d 个按钮新增后查不到", i)
		}
		id := idOf(t, item, "menuId")
		t.Cleanup(func() { _ = doDelete(t, "/system/menu/"+idPath(id)) })
	}
}

// TestMenuFieldTypes isFrame / isCache / menuType 必须是字符串。
//
// 前端 <el-radio value="0"> 提交的是字符串，列表回显用 === 严格比较。
// 建模成数字会两头都断：提交反序列化失败，回显判断永远不成立。
func TestMenuFieldTypes(t *testing.T) {
	id := createMenu(t, newDirPayload("type"))

	detail := dataObject(t, doGet(t, "/system/menu/"+idPath(id)), "菜单详情")
	assertString(t, detail, "菜单详情", "isFrame", "isCache", "menuType", "visible", "status")
	assertField(t, detail, "isFrame", "1", "菜单详情")
	assertField(t, detail, "isCache", "0", "菜单详情")

	// 列表里也要是字符串
	list := dataArray(t, doGet(t, "/system/menu/list"), "菜单列表")
	if item := findBy(list, "menuId", id); item != nil {
		assertString(t, item, "菜单列表项", "isFrame", "isCache", "menuType")
	}
}

// TestMenuCRUD 菜单增删改查。
func TestMenuCRUD(t *testing.T) {
	body := newDirPayload("crud")
	id := createMenu(t, body)
	path := "/system/menu/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查菜单")
	assertField(t, detail, "menuName", body["menuName"], "新增后")
	assertField(t, detail, "orderNum", 90, "新增后")
	assertField(t, detail, "path", body["path"], "新增后")

	// 改：排序改成 0
	updated := payload(body)
	updated["menuId"] = id
	updated["orderNum"] = 0
	updated["menuName"] = testPrefix + "目录改名"
	mustOK(t, doPut(t, "/system/menu", updated), "修改菜单")

	detail = dataObject(t, doGet(t, path), "改后查菜单")
	assertField(t, detail, "orderNum", 0, "改后（排序 0 不能被跳过）")
	assertField(t, detail, "menuName", testPrefix+"目录改名", "改后")

	mustOK(t, doDelete(t, path), "删除菜单")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestMenuSelfParent 上级菜单不能选自己。
func TestMenuSelfParent(t *testing.T) {
	id := createMenu(t, newDirPayload("self"))

	body := newDirPayload("self")
	body["menuId"] = id
	body["parentId"] = id
	mustFail(t, doPut(t, "/system/menu", body), "上级菜单不能选择自己", "父菜单设成自己")
}

// TestMenuFrameMustBeHTTP 外链菜单的地址必须是 http(s)。
func TestMenuFrameMustBeHTTP(t *testing.T) {
	// isFrame = "0" 表示是外链（注意 0 才是"是"，容易反）
	body := with(newDirPayload("frame"), "isFrame", "0")
	body["path"] = "not-a-url"
	mustFail(t, doPost(t, "/system/menu", body), "http", "外链地址不是 http")

	// 合法外链应当通过
	ok := with(newDirPayload("frame2"), "isFrame", "0")
	ok["path"] = "http://example.com"
	createMenu(t, ok)
}

// TestMenuDeleteGuards 有子菜单 / 已分配给角色时不允许删除。
func TestMenuDeleteGuards(t *testing.T) {
	dirID := createMenu(t, newDirPayload("guard"))
	createMenu(t, newButtonPayload(dirID, "guard1"))

	mustFail(t, doDelete(t, "/system/menu/"+idPath(dirID)), "存在子菜单", "删除有子菜单的目录")

	// 菜单 1（系统管理）在初始数据里已分配给 admin 角色
	r := doDelete(t, "/system/menu/1")
	if r.Code == 200 {
		t.Fatal("菜单 1 有子菜单且已分配给角色，不该删除成功")
	}
}

// TestMenuTreeSelectContract 树选择结构：叶子节点不能有 children 键。
func TestMenuTreeSelectContract(t *testing.T) {
	r := doGet(t, "/system/menu/treeselect")
	mustOK(t, r, "菜单树")
	tree := dataArray(t, r, "菜单树")
	if len(tree) == 0 {
		t.Fatal("菜单树不该为空")
	}
	assertTreeShape(t, tree, "菜单树")
}

// TestMenuRoleTreeSelectContract 角色菜单树：checkedKeys 和 menus 平铺在顶层。
func TestMenuRoleTreeSelectContract(t *testing.T) {
	r := doGet(t, "/system/menu/roleMenuTreeselect/2")
	mustOK(t, r, "角色菜单树")

	assertTopLevel(t, r, "角色菜单树", "checkedKeys", "menus")
	assertNoTopLevel(t, r, "角色菜单树", "data")

	menus := toObjects(t, r.Raw["menus"], "角色菜单树的 menus")
	assertTreeShape(t, menus, "角色菜单树")
}

// TestMenuValidation 菜单的字段校验。
func TestMenuValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 menuName", func(p map[string]any) map[string]any { return omit(p, "menuName") }, "MenuName"},
		{"menuName 纯空格", func(p map[string]any) map[string]any { return with(p, "menuName", "   ") }, "MenuName"},
		{"menuName 超过 50 字", func(p map[string]any) map[string]any { return with(p, "menuName", repeatText(51)) }, "MenuName"},
		{"少传 menuType", func(p map[string]any) map[string]any { return omit(p, "menuType") }, "MenuType"},
		{"orderNum 负数", func(p map[string]any) map[string]any { return with(p, "orderNum", -1) }, "OrderNum"},
		// 少传要拒（对齐 Java 的 @NotNull），传 0 要过 —— 见 post_test 的说明
		{"少传 orderNum", func(p map[string]any) map[string]any { return omit(p, "orderNum") }, "OrderNum"},
		{"orderNum = 0", func(p map[string]any) map[string]any { return with(p, "orderNum", 0) }, ""},
		// isFrame 传数字：前端发的是字符串，发数字应当被明确告知类型不对
		{"isFrame 传数字", func(p map[string]any) map[string]any { return with(p, "isFrame", 1) }, "isFrame"},
		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newDirPayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/menu", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				cleanupMenuByName(t, fmt.Sprint(body["menuName"]))
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// assertTreeShape 递归校验树选择结构：
// 每个节点都要有 id/label/disabled；叶子节点不能带 children 键。
func assertTreeShape(t *testing.T, nodes []map[string]any, what string) {
	t.Helper()
	for _, node := range nodes {
		for _, key := range []string{"id", "label", "disabled"} {
			if _, ok := node[key]; !ok {
				t.Errorf("%s：节点缺少 %q 字段，实际字段=%v", what, key, topKeys(node))
			}
		}
		raw, ok := node["children"]
		if !ok {
			continue // 叶子节点，正确
		}
		children := toObjects(t, raw, what+" 的 children")
		if len(children) == 0 {
			t.Errorf("%s：叶子节点不该输出空的 children 键（前端据此判断能否展开）", what)
			continue
		}
		assertTreeShape(t, children, what)
	}
}

func cleanupMenuByName(t *testing.T, name string) {
	t.Helper()
	list := dataArray(t, doGet(t, "/system/menu/list"), "清理查菜单")
	if item := findBy(list, "menuName", name); item != nil {
		_ = doDelete(t, "/system/menu/"+idPath(idOf(t, item, "menuId")))
	}
}
