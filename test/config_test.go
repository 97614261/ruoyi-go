package apitest

import (
	"fmt"
	"net/url"
	"testing"
)

func newConfigPayload(suffix string) map[string]any {
	return map[string]any{
		"configName":  testPrefix + "参数" + suffix,
		"configKey":   "zz.test.key." + suffix,
		"configValue": "value_" + suffix,
		"configType":  "N",
		"remark":      "测试参数",
	}
}

func createConfig(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/config", body), "新增参数")

	name := fmt.Sprint(body["configName"])
	list := pageRows(t, doGet(t, "/system/config/list?pageSize=100&configName="+url.QueryEscape(name)), "查参数")
	item := findBy(list, "configName", name)
	if item == nil {
		t.Fatalf("新增参数后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "configId")

	t.Cleanup(func() { _ = doDelete(t, "/system/config/"+idPath(id)) })
	return id
}

// TestConfigCRUD 参数配置增删改查。
func TestConfigCRUD(t *testing.T) {
	body := newConfigPayload("crud")
	id := createConfig(t, body)
	path := "/system/config/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查参数")
	assertField(t, detail, "configName", body["configName"], "新增后")
	assertField(t, detail, "configKey", body["configKey"], "新增后")
	assertField(t, detail, "configValue", body["configValue"], "新增后")
	assertField(t, detail, "configType", "N", "新增后")

	updated := payload(body)
	updated["configId"] = id
	updated["configValue"] = "changed"
	mustOK(t, doPut(t, "/system/config", updated), "修改参数")

	detail = dataObject(t, doGet(t, path), "改后查参数")
	assertField(t, detail, "configValue", "changed", "改后")

	mustOK(t, doDelete(t, path), "删除参数")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestConfigGetByKeyContract 按键取值：值放在 msg 里，不是 data。
//
// Java 的 success(configValue) 命中的是 success(String msg) 重载，只设 msg。
// 前端读的也是 response.msg。放进 data 前端就取不到值了。
func TestConfigGetByKeyContract(t *testing.T) {
	r := doGet(t, "/system/config/configKey/sys.user.initPassword")
	mustOK(t, r, "按键取参数值")

	if r.Msg == "" || r.Msg == "操作成功" {
		t.Errorf("参数值应放在 msg 里（前端读 response.msg），实际 msg=%q，顶层字段=%v", r.Msg, topKeys(r.Raw))
	}
	assertNoTopLevel(t, r, "按键取参数值", "data")

	// 键名里带点号，路由参数要能正确匹配
	r = doGet(t, "/system/config/configKey/sys.account.captchaEnabled")
	mustOK(t, r, "带点号的参数键")
}

func TestConfigGetByKeyUsesLeastPrivilege(t *testing.T) {
	menuIDForPermission := func(permission string) int64 {
		t.Helper()
		for _, menu := range dataArray(t, doGet(t, "/system/menu/list"), "菜单列表") {
			if menu["perms"] == permission {
				return idOf(t, menu, "menuId")
			}
		}
		t.Fatalf("找不到权限 %s 对应的菜单", permission)
		return 0
	}
	tokenWithPermission := func(suffix, permission string) string {
		t.Helper()
		role := newRolePayload("config_key_" + suffix)
		role["menuIds"] = []int64{menuIDForPermission(permission)}
		roleID := createRole(t, role)
		user := newUserPayload("config_key_" + suffix)
		user["roleIds"] = []int64{roleID}
		createUser(t, user)
		return mustLogin(t, fmt.Sprint(user["userName"]))
	}

	userToken := tokenWithPermission("user", "system:user:list")
	mustOK(t, request("GET", "/system/config/configKey/sys.user.initPassword", userToken, nil),
		"用户管理员读取初始密码")
	if got := request("GET", "/system/config/configKey/sys.account.captchaEnabled", userToken, nil); got.Code != 403 {
		t.Fatalf("用户管理员不应读取任意参数，实际 code=%d msg=%q", got.Code, got.Msg)
	}

	configToken := tokenWithPermission("config", "system:config:query")
	mustOK(t, request("GET", "/system/config/configKey/sys.account.captchaEnabled", configToken, nil),
		"参数管理员读取参数")
}

// TestConfigCacheInvalidation 改参数后，按键取值要立刻反映。
func TestConfigCacheInvalidation(t *testing.T) {
	body := newConfigPayload("cache")
	id := createConfig(t, body)
	key := fmt.Sprint(body["configKey"])

	r := doGet(t, "/system/config/configKey/"+key)
	mustOK(t, r, "取新参数的值")
	if r.Msg != body["configValue"] {
		t.Fatalf("期望取到 %v，实际 %q", body["configValue"], r.Msg)
	}

	updated := payload(body)
	updated["configId"] = id
	updated["configValue"] = "after_change"
	mustOK(t, doPut(t, "/system/config", updated), "改参数值")

	r = doGet(t, "/system/config/configKey/"+key)
	if r.Msg != "after_change" {
		t.Errorf("改参数后缓存应被清除，期望 after_change，实际 %q", r.Msg)
	}
}

// TestConfigKeyRenameClearsOldCache 改键名时，旧键的缓存也要清掉。
func TestConfigKeyRenameClearsOldCache(t *testing.T) {
	body := newConfigPayload("rename")
	id := createConfig(t, body)
	oldKey := fmt.Sprint(body["configKey"])

	// 先读一次把旧键灌进缓存
	mustOK(t, doGet(t, "/system/config/configKey/"+oldKey), "预热旧键缓存")

	newKey := oldKey + ".new"
	updated := payload(body)
	updated["configId"] = id
	updated["configKey"] = newKey
	mustOK(t, doPut(t, "/system/config", updated), "改参数键名")

	// 旧键应查不到值了
	r := doGet(t, "/system/config/configKey/"+oldKey)
	if r.Msg == body["configValue"] {
		t.Errorf("键名改了之后，旧键 %s 不该还能从缓存里读到值", oldKey)
	}
	// 新键要能读到
	r = doGet(t, "/system/config/configKey/"+newKey)
	if r.Msg != body["configValue"] {
		t.Errorf("新键 %s 应能读到值 %v，实际 %q", newKey, body["configValue"], r.Msg)
	}
}

// TestConfigBuiltinCannotDelete 系统内置参数不允许删除。
func TestConfigBuiltinCannotDelete(t *testing.T) {
	// 参数 1 是内置的 sys.index.skinName
	mustFail(t, doDelete(t, "/system/config/1"), "内置参数", "删除内置参数")

	// 确认它还在
	mustOK(t, doGet(t, "/system/config/1"), "内置参数应仍存在")
}

// TestConfigRefreshCache 刷新缓存后仍能正常取值。
func TestConfigRefreshCache(t *testing.T) {
	mustOK(t, doDelete(t, "/system/config/refreshCache"), "刷新参数缓存")
	mustOK(t, doGet(t, "/system/config/configKey/sys.user.initPassword"), "刷新后取值")
}

// TestConfigValidation 参数配置的字段校验。
func TestConfigValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 configName", func(p map[string]any) map[string]any { return omit(p, "configName") }, "ConfigName"},
		{"少传 configKey", func(p map[string]any) map[string]any { return omit(p, "configKey") }, "ConfigKey"},
		{"少传 configValue", func(p map[string]any) map[string]any { return omit(p, "configValue") }, "ConfigValue"},
		{"configName 纯空格", func(p map[string]any) map[string]any { return with(p, "configName", "  ") }, "ConfigName"},
		{"configValue 纯空格", func(p map[string]any) map[string]any { return with(p, "configValue", "  ") }, "ConfigValue"},
		{"configName 超过 100 字", func(p map[string]any) map[string]any { return with(p, "configName", repeatText(101)) }, "ConfigName"},
		{"configKey 超过 100 字", func(p map[string]any) map[string]any { return with(p, "configKey", repeatText(101)) }, "ConfigKey"},
		{"configValue 恰好 500 字", func(p map[string]any) map[string]any { return with(p, "configValue", repeatText(500)) }, ""},
		{"configValue 超过 500 字", func(p map[string]any) map[string]any { return with(p, "configValue", repeatText(501)) }, "ConfigValue"},
		{"少传 remark", func(p map[string]any) map[string]any { return omit(p, "remark") }, ""},
		{"remark 超过 500 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(501)) }, "Remark"},
		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newConfigPayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/config", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestConfigKeyUnique 参数键名不能重复。
func TestConfigKeyUnique(t *testing.T) {
	body := newConfigPayload("uniq")
	createConfig(t, body)

	dup := newConfigPayload("uniq2")
	dup["configKey"] = body["configKey"]
	mustFail(t, doPost(t, "/system/config", dup), "参数键名已存在", "重复的参数键名")
}
