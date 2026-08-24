package apitest

import (
	"fmt"
	"net/url"
	"testing"
)

func newDictTypePayload(suffix string) map[string]any {
	return map[string]any{
		"dictName": testPrefix + "字典" + suffix,
		// dictType 有 @Pattern 约束：小写字母开头，只含小写字母、数字、下划线
		"dictType": "zz_test_type_" + suffix,
		"status":   "0",
		"remark":   "测试字典",
	}
}

func newDictDataPayload(dictType, suffix string) map[string]any {
	return map[string]any{
		"dictType":  dictType,
		"dictLabel": testPrefix + "标签" + suffix,
		"dictValue": "v" + suffix,
		"dictSort":  1,
		"isDefault": "N",
		"status":    "0",
	}
}

func createDictType(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/dict/type", body), "新增字典类型")

	name := fmt.Sprint(body["dictName"])
	list := pageRows(t, doGet(t, "/system/dict/type/list?pageSize=100&dictName="+url.QueryEscape(name)), "查字典类型")
	item := findBy(list, "dictName", name)
	if item == nil {
		t.Fatalf("新增字典类型后按名称 %s 查不到", name)
	}
	id := idOf(t, item, "dictId")

	t.Cleanup(func() { _ = doDelete(t, "/system/dict/type/"+idPath(id)) })
	return id
}

func createDictData(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/dict/data", body), "新增字典数据")

	label := fmt.Sprint(body["dictLabel"])
	list := pageRows(t, doGet(t, "/system/dict/data/list?pageSize=100&dictLabel="+url.QueryEscape(label)), "查字典数据")
	item := findBy(list, "dictLabel", label)
	if item == nil {
		t.Fatalf("新增字典数据后按标签 %s 查不到", label)
	}
	id := idOf(t, item, "dictCode")

	t.Cleanup(func() { _ = doDelete(t, "/system/dict/data/"+idPath(id)) })
	return id
}

// TestDictTypeCRUD 字典类型增删改查。
func TestDictTypeCRUD(t *testing.T) {
	body := newDictTypePayload("crud")
	id := createDictType(t, body)
	path := "/system/dict/type/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查字典类型")
	assertField(t, detail, "dictName", body["dictName"], "新增后")
	assertField(t, detail, "dictType", body["dictType"], "新增后")
	assertString(t, detail, "字典类型详情", "status")

	updated := payload(body)
	updated["dictId"] = id
	updated["dictName"] = testPrefix + "字典改名"
	mustOK(t, doPut(t, "/system/dict/type", updated), "修改字典类型")

	detail = dataObject(t, doGet(t, path), "改后查字典类型")
	assertField(t, detail, "dictName", testPrefix+"字典改名", "改后")

	mustOK(t, doDelete(t, path), "删除字典类型")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestDictTypeRenameSyncsData 改字典类型的 dictType 时，下面的字典数据要跟着改。
//
// dict_type 是字符串外键，改了类型不改数据，那批数据就成了孤儿：
// 类型列表里看得到、点进去一条数据都没有。
func TestDictTypeRenameSyncsData(t *testing.T) {
	typeBody := newDictTypePayload("sync")
	typeID := createDictType(t, typeBody)
	oldType := fmt.Sprint(typeBody["dictType"])

	createDictData(t, newDictDataPayload(oldType, "s1"))

	newType := oldType + "_new"
	updated := payload(typeBody)
	updated["dictId"] = typeID
	updated["dictType"] = newType
	mustOK(t, doPut(t, "/system/dict/type", updated), "改字典类型的 type")

	// 按新类型应能查到那条数据
	list := pageRows(t, doGet(t, "/system/dict/data/list?pageSize=100&dictType="+newType), "按新类型查数据")
	if len(list) == 0 {
		t.Errorf("字典类型改名后，下属字典数据的 dictType 应同步更新，否则数据会变成孤儿")
	}
	// 按旧类型应查不到
	old := pageRows(t, doGet(t, "/system/dict/data/list?pageSize=100&dictType="+oldType), "按旧类型查数据")
	if len(old) != 0 {
		t.Errorf("旧类型下不该还有 %d 条数据", len(old))
	}
}

// TestDictCacheInvalidation 改字典数据后，按类型查的接口要立刻反映变化。
//
// 这个接口走 Redis 缓存，增删改都必须清缓存，否则前端下拉框
// 会一直显示旧内容，最长要等 30 分钟 TTL 过期。
func TestDictCacheInvalidation(t *testing.T) {
	typeBody := newDictTypePayload("cache")
	createDictType(t, typeBody)
	dictType := fmt.Sprint(typeBody["dictType"])

	// 先查一次，把空结果灌进缓存
	before := dataArray(t, doGet(t, "/system/dict/data/type/"+dictType), "按类型查字典（新增前）")
	if len(before) != 0 {
		t.Fatalf("新建的字典类型下不该有数据，实际 %d 条", len(before))
	}

	dataID := createDictData(t, newDictDataPayload(dictType, "c1"))

	// 新增后必须立刻能查到
	after := dataArray(t, doGet(t, "/system/dict/data/type/"+dictType), "按类型查字典（新增后）")
	if len(after) != 1 {
		t.Fatalf("新增字典数据后缓存应被清除，期望查到 1 条，实际 %d 条", len(after))
	}

	// 改标签后也要立刻反映
	updated := newDictDataPayload(dictType, "c1")
	updated["dictCode"] = dataID
	updated["dictLabel"] = testPrefix + "改后的标签"
	mustOK(t, doPut(t, "/system/dict/data", updated), "改字典数据")

	after = dataArray(t, doGet(t, "/system/dict/data/type/"+dictType), "按类型查字典（改后）")
	if len(after) != 1 || fmt.Sprint(after[0]["dictLabel"]) != testPrefix+"改后的标签" {
		t.Errorf("改字典数据后缓存应被清除，实际查到 %v", after)
	}

	// 删除后也要立刻反映
	mustOK(t, doDelete(t, "/system/dict/data/"+idPath(dataID)), "删字典数据")
	after = dataArray(t, doGet(t, "/system/dict/data/type/"+dictType), "按类型查字典（删后）")
	if len(after) != 0 {
		t.Errorf("删除字典数据后缓存应被清除，实际还剩 %d 条", len(after))
	}
}

// TestDictTypeDeleteWithData 下面还有字典数据的类型不允许删除。
func TestDictTypeDeleteWithData(t *testing.T) {
	typeBody := newDictTypePayload("del")
	typeID := createDictType(t, typeBody)
	createDictData(t, newDictDataPayload(fmt.Sprint(typeBody["dictType"]), "d1"))

	mustFail(t, doDelete(t, "/system/dict/type/"+idPath(typeID)), "已分配", "删除还有数据的字典类型")
}

// TestDictDataByTypeContract 按类型查字典：查不到时要返回空数组而不是 null。
//
// 前端拿到就直接 v-for，null 会直接报错。
func TestDictDataByTypeContract(t *testing.T) {
	r := doGet(t, "/system/dict/data/type/根本不存在的字典类型")
	mustOK(t, r, "查不存在的字典类型")

	raw, ok := r.Raw["data"]
	if !ok {
		t.Fatal("响应里应有 data 字段")
	}
	if raw == nil {
		t.Fatal("查不到字典时 data 应为空数组而不是 null（前端会直接 v-for）")
	}
	dataArray(t, r, "查不存在的字典类型")
}

// TestDictTypeValidation 字典类型的字段校验，重点是 dictType 的格式约束。
func TestDictTypeValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 dictName", func(p map[string]any) map[string]any { return omit(p, "dictName") }, "DictName"},
		{"dictName 纯空格", func(p map[string]any) map[string]any { return with(p, "dictName", "   ") }, "DictName"},
		{"dictName 超过 100 字", func(p map[string]any) map[string]any { return with(p, "dictName", repeatText(101)) }, "DictName"},
		{"少传 dictType", func(p map[string]any) map[string]any { return omit(p, "dictType") }, "DictType"},

		// @Pattern(regexp = "^[a-z][a-z0-9_]*$")
		{"dictType 含大写", func(p map[string]any) map[string]any { return with(p, "dictType", "zzTestUpper") }, "DictType"},
		{"dictType 数字开头", func(p map[string]any) map[string]any { return with(p, "dictType", "1zz_test") }, "DictType"},
		{"dictType 含中划线", func(p map[string]any) map[string]any { return with(p, "dictType", "zz-test") }, "DictType"},
		{"dictType 含中文", func(p map[string]any) map[string]any { return with(p, "dictType", "zz_测试") }, "DictType"},
		{"dictType 合法（下划线数字）", func(p map[string]any) map[string]any { return with(p, "dictType", "zz_test_ok_9") }, ""},

		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newDictTypePayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/dict/type", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestDictDataSortRequired 字典排序必填，但 0 是合法值。
//
// 这两条必须同时成立，也是排序字段改成指针的原因：
// 非指针 int 分不清"没传"和"传了 0"，只能二选一。
//
// 【这里比 Java 严，是有意的】Java 的 SysDictData.dictSort 是裸 Long，
// 没有 @NotNull，不传会存 NULL。但排序列存 NULL 会让列表顺序变得不可预期，
// 而前端表单本就是 :min="0" 且总会带值 —— 已在 CONVENTIONS 的偏离登记里备案。
func TestDictDataSortRequired(t *testing.T) {
	typeBody := newDictTypePayload("sortreq")
	createDictType(t, typeBody)
	dictType := fmt.Sprint(typeBody["dictType"])

	mustFail(t, doPost(t, "/system/dict/data",
		omit(newDictDataPayload(dictType, "s1"), "dictSort")), "DictSort", "少传 dictSort")

	mustOK(t, doPost(t, "/system/dict/data",
		with(newDictDataPayload(dictType, "s2"), "dictSort", 0)), "dictSort = 0")

	mustFail(t, doPost(t, "/system/dict/data",
		with(newDictDataPayload(dictType, "s3"), "dictSort", -1)), "DictSort", "dictSort 负数")
}

// TestDictTypeUnique 字典类型不能重复。
func TestDictTypeUnique(t *testing.T) {
	body := newDictTypePayload("uniq")
	createDictType(t, body)

	// 【不能原样再提交一次】请求体一模一样会先被防重复提交拦下来，
	// 拿到的是"不允许重复提交"而不是"字典类型已存在"，测不到想测的东西。
	// 这里只保持 dictType 相同 —— 唯一性本来就是按它判的。
	duplicate := with(body, "dictName", testPrefix+"字典uniq另一个名字")
	mustFail(t, doPost(t, "/system/dict/type", duplicate), "字典类型已存在", "重复的字典类型")
}

// TestDictTypeOptionSelectAndRefresh 下拉与刷新缓存。
func TestDictTypeOptionSelectAndRefresh(t *testing.T) {
	r := doGet(t, "/system/dict/type/optionselect")
	mustOK(t, r, "字典类型下拉")
	dataArray(t, r, "字典类型下拉")

	mustOK(t, doDelete(t, "/system/dict/type/refreshCache"), "刷新字典缓存")

	// 刷新后按类型查仍要正常（走回源）
	mustOK(t, doGet(t, "/system/dict/data/type/sys_normal_disable"), "刷新缓存后按类型查字典")
}
