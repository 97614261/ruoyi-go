package apitest

import (
	"fmt"
	"strings"
	"testing"
)

// rootDeptID 初始数据里的顶级部门"若依科技"。
const rootDeptID = 100

func newDeptPayload(parentID int64, suffix string) map[string]any {
	return map[string]any{
		"parentId": parentID,
		"deptName": testPrefix + "部门" + suffix,
		"orderNum": 9,
		"leader":   "负责人",
		"phone":    "13800000000",
		"email":    "dept@example.com",
		"status":   "0",
	}
}

func createDept(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/dept", body), "新增部门 "+fmt.Sprint(body["deptName"]))

	name := fmt.Sprint(body["deptName"])
	list := dataArray(t, doGet(t, "/system/dept/list"), "部门列表")
	item := findBy(list, "deptName", name)
	if item == nil {
		t.Fatalf("新增部门后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "deptId")

	t.Cleanup(func() { _ = doDelete(t, "/system/dept/"+idPath(id)) })
	return id
}

// TestDeptCRUD 部门增删改查，逐字段核对。
func TestDeptCRUD(t *testing.T) {
	body := newDeptPayload(rootDeptID, "crud")
	id := createDept(t, body)
	path := "/system/dept/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查部门")
	assertField(t, detail, "deptName", body["deptName"], "新增后")
	assertField(t, detail, "orderNum", 9, "新增后")
	assertField(t, detail, "leader", "负责人", "新增后")
	assertField(t, detail, "email", "dept@example.com", "新增后")
	assertString(t, detail, "部门详情", "status")
	// ancestors 由服务端计算：父部门 100 的 ancestors 是 "0"，所以这里应是 "0,100"
	assertField(t, detail, "ancestors", "0,100", "新增后（ancestors 由服务端算）")
	// 详情要带上级部门名称
	assertField(t, detail, "parentName", "若依科技", "新增后")

	// 改：排序改 0
	updated := payload(body)
	updated["deptId"] = id
	updated["orderNum"] = 0
	updated["deptName"] = testPrefix + "部门改名"
	mustOK(t, doPut(t, "/system/dept", updated), "修改部门")

	detail = dataObject(t, doGet(t, path), "改后查部门")
	assertField(t, detail, "orderNum", 0, "改后（排序 0 不能被跳过）")
	assertField(t, detail, "deptName", testPrefix+"部门改名", "改后")

	mustOK(t, doDelete(t, path), "删除部门")
	mustFail(t, doGet(t, path), "", "删除后再查")
}

// TestDeptUpdateSort 保存部门排序的请求格式和批量更新结果。
func TestDeptUpdateSort(t *testing.T) {
	firstID := createDept(t, newDeptPayload(rootDeptID, "sort_1"))
	secondID := createDept(t, newDeptPayload(rootDeptID, "sort_2"))

	mustOK(t, doPut(t, "/system/dept/updateSort", map[string]any{
		"deptIds":   idPath(firstID) + "," + idPath(secondID),
		"orderNums": "2,1",
	}), "保存部门排序")

	first := dataObject(t, doGet(t, "/system/dept/"+idPath(firstID)), "查询第一个部门排序")
	second := dataObject(t, doGet(t, "/system/dept/"+idPath(secondID)), "查询第二个部门排序")
	assertField(t, first, "orderNum", 2, "部门排序更新后")
	assertField(t, second, "orderNum", 1, "部门排序更新后")

	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{"ID 与排序数量不一致", map[string]any{"deptIds": idPath(firstID) + "," + idPath(secondID), "orderNums": "1"}, "排序参数不匹配"},
		{"部门 ID 非数字", map[string]any{"deptIds": "invalid", "orderNums": "1"}, "排序参数格式错误"},
		{"排序值非数字", map[string]any{"deptIds": idPath(firstID), "orderNums": "invalid"}, "排序参数格式错误"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustFail(t, doPut(t, "/system/dept/updateSort", tc.body), tc.want, tc.name)
		})
	}
}

// TestDeptAncestorsRebuild 改上级部门时，子孙的 ancestors 必须同步重算。
//
// ancestors 是冗余字段，数据权限的"本部门及以下"完全依赖它。
// 改了父级却不改子孙，整棵树的权限判断就全错了。
func TestDeptAncestorsRebuild(t *testing.T) {
	// 造两层：A（挂在 100 下）→ B（挂在 A 下）
	aID := createDept(t, newDeptPayload(rootDeptID, "anc_a"))
	bID := createDept(t, newDeptPayload(aID, "anc_b"))

	bDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(bID)), "查 B 部门")
	wantB := fmt.Sprintf("0,%d,%d", rootDeptID, aID)
	assertField(t, bDetail, "ancestors", wantB, "B 的初始 ancestors")

	// 再造一个 C，把 A 挪到 C 下面
	cID := createDept(t, newDeptPayload(rootDeptID, "anc_c"))

	moveA := newDeptPayload(cID, "anc_a")
	moveA["deptId"] = aID
	mustOK(t, doPut(t, "/system/dept", moveA), "把 A 挪到 C 下")

	// A 的 ancestors 应变成 0,100,C
	aDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(aID)), "挪动后查 A")
	wantA := fmt.Sprintf("0,%d,%d", rootDeptID, cID)
	assertField(t, aDetail, "ancestors", wantA, "A 挪动后的 ancestors")

	// 【关键】B 的 ancestors 也必须跟着变
	bDetail = dataObject(t, doGet(t, "/system/dept/"+idPath(bID)), "挪动后查 B")
	wantB = fmt.Sprintf("0,%d,%d,%d", rootDeptID, cID, aID)
	assertField(t, bDetail, "ancestors", wantB, "父部门挪动后，子部门的 ancestors 必须同步重算")
}

func TestDeptRejectsDescendantParentAndRebuildsRootMove(t *testing.T) {
	aID := createDept(t, newDeptPayload(rootDeptID, "cycle_a"))
	bID := createDept(t, newDeptPayload(aID, "cycle_b"))
	cID := createDept(t, newDeptPayload(bID, "cycle_c"))

	cycle := newDeptPayload(cID, "cycle_a")
	cycle["deptId"] = aID
	mustFail(t, doPut(t, "/system/dept", cycle), "上级部门不能是自己的下级", "部门不能挂到后代下")

	aDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(aID)), "循环修改失败后查询 A")
	assertField(t, aDetail, "parentId", rootDeptID, "循环修改不得写入")
	assertField(t, aDetail, "ancestors", "0,100", "循环修改不得破坏 ancestors")

	moveRoot := newDeptPayload(0, "cycle_a")
	moveRoot["deptId"] = aID
	mustOK(t, doPut(t, "/system/dept", moveRoot), "把 A 移到根节点")

	aDetail = dataObject(t, doGet(t, "/system/dept/"+idPath(aID)), "移到根后查询 A")
	assertField(t, aDetail, "parentId", 0, "根部门 parentId")
	assertField(t, aDetail, "ancestors", "0", "根部门 ancestors")
	bDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(bID)), "移到根后查询 B")
	assertField(t, bDetail, "ancestors", fmt.Sprintf("0,%d", aID), "根移动后 B ancestors")
	cDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(cID)), "移到根后查询 C")
	assertField(t, cDetail, "ancestors", fmt.Sprintf("0,%d,%d", aID, bID), "根移动后 C ancestors")
}

// TestDeptExcludeChild 排除子部门：上级下拉里不能出现自己和自己的下级。
func TestDeptExcludeChild(t *testing.T) {
	aID := createDept(t, newDeptPayload(rootDeptID, "ex_a"))
	bID := createDept(t, newDeptPayload(aID, "ex_b"))

	list := dataArray(t, doGet(t, "/system/dept/list/exclude/"+idPath(aID)), "排除 A 的部门列表")

	if findBy(list, "deptId", aID) != nil {
		t.Errorf("排除列表里不该包含自己（部门 %d）", aID)
	}
	if findBy(list, "deptId", bID) != nil {
		t.Errorf("排除列表里不该包含下级部门（部门 %d），否则能把上级设成自己的子部门形成环", bID)
	}
	if findBy(list, "deptId", int64(rootDeptID)) == nil {
		t.Errorf("排除列表里应包含无关部门 %d", rootDeptID)
	}
}

// TestDeptDeleteGuards 有下级 / 有用户时不允许删除。
func TestDeptDeleteGuards(t *testing.T) {
	aID := createDept(t, newDeptPayload(rootDeptID, "del_a"))
	createDept(t, newDeptPayload(aID, "del_b"))

	mustFail(t, doDelete(t, "/system/dept/"+idPath(aID)), "存在下级部门", "删除有下级的部门")

	// 部门 103（研发部门）在初始数据里有 admin 用户
	r := doDelete(t, "/system/dept/103")
	if r.Code == 200 {
		t.Fatal("部门 103 下有用户，不该删除成功")
	}
}

// TestDeptDisableWithNormalChild 停用父部门时，若有启用的子部门应被拒。
func TestDeptDisableWithNormalChild(t *testing.T) {
	aID := createDept(t, newDeptPayload(rootDeptID, "dis_a"))
	createDept(t, newDeptPayload(aID, "dis_b")) // 子部门是启用状态

	disable := newDeptPayload(rootDeptID, "dis_a")
	disable["deptId"] = aID
	disable["status"] = "1"
	mustFail(t, doPut(t, "/system/dept", disable), "未停用的子部门", "停用有启用子部门的父部门")
}

// TestDeptListContract 部门列表返回的是 AjaxResult 不是 TableDataInfo。
func TestDeptListContract(t *testing.T) {
	r := doGet(t, "/system/dept/list")
	mustOK(t, r, "部门列表")

	// 数据在 data 里，不分页，没有 total/rows
	assertTopLevel(t, r, "部门列表", "code", "msg", "data")
	assertNoTopLevel(t, r, "部门列表", "total", "rows")
	dataArray(t, r, "部门列表")
}

// TestDeptValidation 部门字段校验，重点是可选指针字段。
func TestDeptValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 deptName", func(p map[string]any) map[string]any { return omit(p, "deptName") }, "DeptName"},
		{"deptName 纯空格", func(p map[string]any) map[string]any { return with(p, "deptName", "   ") }, "DeptName"},
		{"deptName 恰好 30 字", func(p map[string]any) map[string]any { return with(p, "deptName", repeatText(30)) }, ""},
		{"deptName 超过 30 字", func(p map[string]any) map[string]any { return with(p, "deptName", repeatText(31)) }, "DeptName"},
		{"orderNum 负数", func(p map[string]any) map[string]any { return with(p, "orderNum", -1) }, "OrderNum"},
		// 少传要拒（对齐 Java 的 @NotNull），传 0 要过 —— 见 post_test 的说明
		{"少传 orderNum", func(p map[string]any) map[string]any { return omit(p, "orderNum") }, "OrderNum"},
		{"orderNum = 0", func(p map[string]any) map[string]any { return with(p, "orderNum", 0) }, ""},

		// --- 可选指针字段的两个失败点 ---
		// 不传：nil 指针，validator 会直接判失败，靠 omitempty 兜住
		{"少传 email", func(p map[string]any) map[string]any { return omit(p, "email") }, ""},
		// 传空串：非 nil 指针，omitempty 不生效，靠覆盖后的 email 规则兜住
		{"email 传空串", func(p map[string]any) map[string]any { return with(p, "email", "") }, ""},
		{"email 格式非法", func(p map[string]any) map[string]any { return with(p, "email", "not-an-email") }, "Email"},
		{"email 超过 50 字", func(p map[string]any) map[string]any {
			return with(p, "email", strings.Repeat("a", 45)+"@x.com")
		}, "Email"},

		{"少传 phone", func(p map[string]any) map[string]any { return omit(p, "phone") }, ""},
		{"phone 传空串", func(p map[string]any) map[string]any { return with(p, "phone", "") }, ""},
		{"phone 超过 11 位", func(p map[string]any) map[string]any { return with(p, "phone", "123456789012") }, "Phone"},

		{"少传 leader", func(p map[string]any) map[string]any { return omit(p, "leader") }, ""},
		{"leader 超过 20 字", func(p map[string]any) map[string]any { return with(p, "leader", repeatText(21)) }, "Leader"},

		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newDeptPayload(rootDeptID, fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/dept", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				cleanupDeptByName(t, fmt.Sprint(body["deptName"]))
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestDeptUniqueName 同一父部门下不允许重名。
func TestDeptUniqueName(t *testing.T) {
	body := newDeptPayload(rootDeptID, "uniq")
	createDept(t, body)

	// 【不能原样再提交一次】请求体一模一样会先被防重复提交拦下来，
	// 拿到的是"不允许重复提交"而不是"部门名称已存在"。
	// 这里只保持 deptName 和 parentId 相同 —— 重名判定就是按这两个来的。
	duplicate := with(body, "orderNum", 88)
	mustFail(t, doPost(t, "/system/dept", duplicate), "部门名称已存在", "同父级下重名部门")

	// 不同父部门下同名应当允许
	otherParent := createDept(t, newDeptPayload(rootDeptID, "uniq_other"))
	sameName := payload(body)
	sameName["parentId"] = otherParent
	mustOK(t, doPost(t, "/system/dept", sameName), "不同父级下的同名部门")
	cleanupDeptByNameUnderParent(t, fmt.Sprint(sameName["deptName"]), otherParent)
}

func cleanupDeptByName(t *testing.T, name string) {
	t.Helper()
	list := dataArray(t, doGet(t, "/system/dept/list"), "清理查部门")
	if item := findBy(list, "deptName", name); item != nil {
		_ = doDelete(t, "/system/dept/"+idPath(idOf(t, item, "deptId")))
	}
}

func cleanupDeptByNameUnderParent(t *testing.T, name string, parentID int64) {
	t.Helper()
	list := dataArray(t, doGet(t, "/system/dept/list"), "清理查部门")
	for _, item := range list {
		if fmt.Sprint(item["deptName"]) == name && idOf(t, item, "parentId") == parentID {
			_ = doDelete(t, "/system/dept/"+idPath(idOf(t, item, "deptId")))
			return
		}
	}
}
