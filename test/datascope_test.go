package apitest

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// 数据权限取值，对齐 sys_role.data_scope
const (
	scopeAll        = "1" // 全部数据
	scopeCustom     = "2" // 自定义
	scopeDept       = "3" // 本部门
	scopeDeptAndSub = "4" // 本部门及以下
	scopeSelf       = "5" // 仅本人
)

// TestDataScope 数据权限的端到端验证。
//
// 【为什么必须真的换个账号登录】
// 数据权限是"这个人能看到哪些行"，管理员自己怎么点都是全量，
// 看不出任何问题。只有建一套部门树、建一个受限角色、建一个属于某部门的用户，
// 再用那个用户登录去查列表，才能验证过滤条件真的生效了。
//
// 这也是唯一能发现"数据权限静默失效"的方式 —— 该失效时接口不报错，
// 只是悄悄多返回了别人部门的数据。
func TestDataScope(t *testing.T) {
	// ---------- 建部门树 ----------
	//
	//   100（若依科技）
	//     ├─ 甲部门
	//     │    └─ 甲部门下属
	//     └─ 乙部门
	deptA := createDept(t, newDeptPayload(rootDeptID, "scope甲"))
	deptASub := createDept(t, newDeptPayload(deptA, "scope甲下属"))
	deptB := createDept(t, newDeptPayload(rootDeptID, "scope乙"))

	// ---------- 建角色 ----------
	//
	// 菜单权限给全量：这里要验的是数据过滤，不是功能权限。
	// 少给一个 system:user:list 就会卡在没权限上，跟数据权限无关。
	roleBody := newRolePayload("scope")
	roleBody["menuIds"] = allMenuIDs(t)
	roleID := createRole(t, roleBody)

	// ---------- 建用户 ----------
	//
	// 主角在甲部门；另外三个分别在甲部门、甲部门下属、乙部门。
	actorBody := newUserPayload("scopeself")
	actorBody["deptId"] = deptA
	actorBody["roleIds"] = []int64{roleID}
	createUser(t, actorBody)
	actor := fmt.Sprint(actorBody["userName"])

	inA := createUserInDept(t, "scopea", deptA)
	inASub := createUserInDept(t, "scopeasub", deptASub)
	inB := createUserInDept(t, "scopeb", deptB)

	// ---------- 逐个数据权限验证 ----------

	t.Run("本部门及以下", func(t *testing.T) {
		token := loginWithScope(t, actor, roleID, scopeDeptAndSub, nil)
		assertSees(t, token, "本部门及以下", actor, inA, inASub)
		assertBlind(t, token, "本部门及以下", inB, "admin")
	})

	t.Run("本部门", func(t *testing.T) {
		token := loginWithScope(t, actor, roleID, scopeDept, nil)
		assertSees(t, token, "本部门", actor, inA)
		assertBlind(t, token, "本部门", inASub, inB, "admin")
	})

	t.Run("仅本人", func(t *testing.T) {
		token := loginWithScope(t, actor, roleID, scopeSelf, nil)
		assertSees(t, token, "仅本人", actor)
		assertBlind(t, token, "仅本人", inA, inASub, inB, "admin")
	})

	t.Run("全部数据", func(t *testing.T) {
		token := loginWithScope(t, actor, roleID, scopeAll, nil)
		assertSees(t, token, "全部数据", actor, inA, inASub, inB, "admin")
	})

	t.Run("自定义（只授权乙部门）", func(t *testing.T) {
		token := loginWithScope(t, actor, roleID, scopeCustom, []int64{deptB})
		assertSees(t, token, "自定义", inB)
		// 自定义范围里没有甲部门，所以连自己都看不到 —— 这是 Java 版的行为，不要"修正"
		assertBlind(t, token, "自定义", actor, inA, inASub, "admin")
	})
}

// TestDataScopeAppliesToDeptList 部门列表同样受数据权限约束。
//
// 只测用户列表是不够的：数据权限是逐接口挂的，
// 漏挂一个接口就等于开了个后门（比如从部门树里看到别人的组织架构）。
func TestDataScopeAppliesToDeptList(t *testing.T) {
	deptA := createDept(t, newDeptPayload(rootDeptID, "dscope甲"))
	deptB := createDept(t, newDeptPayload(rootDeptID, "dscope乙"))

	roleBody := newRolePayload("dscope")
	roleBody["menuIds"] = allMenuIDs(t)
	roleID := createRole(t, roleBody)

	actorBody := newUserPayload("dscopeuser")
	actorBody["deptId"] = deptA
	actorBody["roleIds"] = []int64{roleID}
	createUser(t, actorBody)

	setRoleDataScope(t, roleID, scopeDeptAndSub, nil)
	token := mustLogin(t, fmt.Sprint(actorBody["userName"]))

	r := request(http.MethodGet, "/system/dept/list", token, nil)
	mustOK(t, r, "受限用户查部门列表")

	depts := dataArray(t, r, "受限用户的部门列表")
	visible := make(map[int64]bool, len(depts))
	for _, dept := range depts {
		visible[idOf(t, dept, "deptId")] = true
	}

	if !visible[deptA] {
		t.Errorf("本部门及以下：应能看到自己所在的部门 %d，实际可见 %v", deptA, visible)
	}
	if visible[deptB] {
		t.Errorf("本部门及以下：不该看到平级的部门 %d，越权了", deptB)
	}
}

// TestDataScopeNoRuleMatchedSeesNothing 一条规则都没命中时，必须一行都查不到。
//
// 这是最容易写反的分支：拼过滤条件时如果一个角色都没匹配上，
// 很自然会写成"不加条件" —— 那就变成了全量可见，方向完全相反。
func TestDataScopeNoRuleMatchedSeesNothing(t *testing.T) {
	roleBody := newRolePayload("norule")
	roleBody["menuIds"] = allMenuIDs(t)
	roleID := createRole(t, roleBody)

	actorBody := newUserPayload("noruleuser")
	actorBody["deptId"] = testDeptID
	actorBody["roleIds"] = []int64{roleID}
	createUser(t, actorBody)
	actor := fmt.Sprint(actorBody["userName"])

	// 先用"仅本人"确认这个账号本来查得动，排除"其实是没权限"的干扰
	token := loginWithScope(t, actor, roleID, scopeSelf, nil)
	rows := pageRows(t, requestOK(t, token, "/system/user/list?pageNum=1&pageSize=100"), "仅本人")
	if len(rows) == 0 {
		t.Fatal("仅本人时至少应看到自己，测试前提不成立")
	}

	// 再把数据权限设成一个不存在的取值，等价于"没有任何规则命中"
	token = loginWithScope(t, actor, roleID, "9", nil)
	rows = pageRows(t, requestOK(t, token, "/system/user/list?pageNum=1&pageSize=100"), "无匹配规则")
	if len(rows) != 0 {
		t.Errorf("没有任何数据权限规则命中时应当一行都查不到，实际返回 %d 行（说明兜底写成了不加条件）", len(rows))
	}
}

// ---------- 辅助 ----------

// createUserInDept 在指定部门下建一个用户，返回登录账号。
func createUserInDept(t *testing.T, suffix string, deptID int64) string {
	t.Helper()
	body := newUserPayload(suffix)
	body["deptId"] = deptID
	createUser(t, body)
	return fmt.Sprint(body["userName"])
}

// loginWithScope 把角色的数据权限改成 scope，然后用 actor 重新登录。
//
// 【必须重新登录】数据权限来自 Redis 里缓存的会话（含角色及其 dataScope）。
// 改完角色不重新登录，用的还是旧会话，测的就是改之前的行为。
func loginWithScope(t *testing.T, actorName string, roleID int64, scope string, deptIDs []int64) string {
	t.Helper()
	setRoleDataScope(t, roleID, scope, deptIDs)
	return mustLogin(t, actorName)
}

func setRoleDataScope(t *testing.T, roleID int64, scope string, deptIDs []int64) {
	t.Helper()
	if deptIDs == nil {
		deptIDs = []int64{}
	}
	mustOK(t, doPut(t, "/system/role/dataScope", map[string]any{
		"roleId":            roleID,
		"dataScope":         scope,
		"deptIds":           deptIDs,
		"deptCheckStrictly": false,
	}), "把角色的数据权限设为 "+scope)
}

func mustLogin(t *testing.T, userName string) string {
	t.Helper()
	token, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("账号 %s 登录失败：%v", userName, err)
	}
	return token
}

func requestOK(t *testing.T, token, path string) response {
	t.Helper()
	r := request(http.MethodGet, path, token, nil)
	assertNoPasswordLeak(t, path, r)
	mustOK(t, r, path)
	return r
}

// canSee 按账号名精确查询，判断当前会话能不能看到这条记录。
//
// 【不要改成"拉一页列表再比对"】用户数超过 pageSize 时想找的记录会翻到第二页，
// 断言就会时对时错。带条件查询是唯一稳定的做法。
// 另外必须做精确比对：userName 是模糊匹配，
// zz_test_scopea 会同时命中 zz_test_scopeasub。
func canSee(t *testing.T, token, userName string) bool {
	t.Helper()
	path := "/system/user/list?pageNum=1&pageSize=100&userName=" + url.QueryEscape(userName)
	rows := pageRows(t, requestOK(t, token, path), "按账号查用户")
	return findBy(rows, "userName", userName) != nil
}

func assertSees(t *testing.T, token, scope string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !canSee(t, token, want) {
			t.Errorf("数据权限「%s」：应当能看到账号 %s，实际查不到", scope, want)
		}
	}
}

func assertBlind(t *testing.T, token, scope string, unwanted ...string) {
	t.Helper()
	for _, name := range unwanted {
		if canSee(t, token, name) {
			t.Errorf("数据权限「%s」：不该看到账号 %s，越权了", scope, name)
		}
	}
}

// allMenuIDs 取全部菜单 ID。
//
// 不写死：菜单表是可编辑的，写死的话别人加一个菜单这里就不准了。
func allMenuIDs(t *testing.T) []int64 {
	t.Helper()
	menus := dataArray(t, doGet(t, "/system/menu/list"), "菜单列表")
	ids := make([]int64, 0, len(menus))
	for _, menu := range menus {
		ids = append(ids, idOf(t, menu, "menuId"))
	}
	if len(ids) == 0 {
		t.Fatal("菜单列表为空，无法给测试角色授权")
	}
	return ids
}
