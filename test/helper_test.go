package apitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// response 一次请求的结果。
type response struct {
	Status int            // HTTP 状态码，正常情况恒为 200
	Code   int            // 业务 code，200 成功
	Msg    string         // 提示语
	Raw    map[string]any // 解析后的完整响应体
	Body   []byte         // 原始字节，导出这类二进制响应用得到
	Header http.Header
}

// request 发一次请求。body 传 url.Values 会用表单编码，其余用 JSON。
func request(method, path, token string, body any) response {
	var (
		reader      io.Reader
		contentType string
	)
	switch v := body.(type) {
	case nil:
		// 无请求体
	case url.Values:
		reader = strings.NewReader(v.Encode())
		contentType = "application/x-www-form-urlencoded"
	default:
		data, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("序列化请求体失败: %v", err))
		}
		reader = bytes.NewReader(data)
		contentType = "application/json"
	}

	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	result := response{
		Status: rec.Code,
		Body:   rec.Body.Bytes(),
		Header: rec.Header(),
	}
	// 二进制响应（导出）解析不了 JSON，属正常
	if err := json.Unmarshal(result.Body, &result.Raw); err == nil {
		if code, ok := result.Raw["code"].(float64); ok {
			result.Code = int(code)
		}
		result.Msg, _ = result.Raw["msg"].(string)
	}
	return result
}

// ---------- 带管理员身份的快捷方法 ----------
//
// 每个都会顺带做一次密码泄漏检查，不用在用例里逐个写。

func doGet(t *testing.T, path string) response {
	t.Helper()
	r := request(http.MethodGet, path, adminToken, nil)
	assertNoPasswordLeak(t, path, r)
	return r
}

func doPost(t *testing.T, path string, body any) response {
	t.Helper()
	r := request(http.MethodPost, path, adminToken, body)
	assertNoPasswordLeak(t, path, r)
	return r
}

func doPut(t *testing.T, path string, body any) response {
	t.Helper()
	r := request(http.MethodPut, path, adminToken, body)
	assertNoPasswordLeak(t, path, r)
	return r
}

func doDelete(t *testing.T, path string) response {
	t.Helper()
	r := request(http.MethodDelete, path, adminToken, nil)
	assertNoPasswordLeak(t, path, r)
	return r
}

// ---------- 基础断言 ----------

// mustOK 断言业务成功。
func mustOK(t *testing.T, r response, what string) {
	t.Helper()
	if r.Status != http.StatusOK {
		t.Fatalf("%s：HTTP 状态码应为 200，实际 %d，响应=%s", what, r.Status, truncBody(r.Body))
	}
	if r.Code != 200 {
		t.Fatalf("%s：业务 code 应为 200，实际 %d，msg=%q", what, r.Code, r.Msg)
	}
}

// mustFail 断言业务失败，且提示语里出现 wantIn。
//
// wantIn 传空表示只要求失败、不检查文案。
func mustFail(t *testing.T, r response, wantIn, what string) {
	t.Helper()
	// 【关键】HTTP 状态码必须仍是 200 —— RuoYi 前端只看 body 里的 code，
	// 返回 4xx/5xx 会让前端走到 axios 的 catch 分支，提示语丢失
	if r.Status != http.StatusOK {
		t.Fatalf("%s：错误响应的 HTTP 状态码也应为 200，实际 %d", what, r.Status)
	}
	if r.Code == 200 {
		t.Fatalf("%s：本应失败，却返回成功（msg=%q）", what, r.Msg)
	}
	if wantIn != "" && !strings.Contains(r.Msg, wantIn) {
		t.Errorf("%s：错误提示里应包含 %q，实际 msg=%q", what, wantIn, r.Msg)
	}
}

// ---------- 契约断言（响应形状）----------
//
// 这组是本项目最该测的东西：响应结构和前端契约对不上时，
// 后端毫无感知，前端却会整片报错。

// assertTopLevel 断言这些字段**平铺在顶层**。
func assertTopLevel(t *testing.T, r response, what string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := r.Raw[key]; !ok {
			t.Errorf("%s：字段 %q 应平铺在顶层，实际顶层只有 %v", what, key, topKeys(r.Raw))
		}
	}
}

// assertNoTopLevel 断言这些字段**不在**顶层。
func assertNoTopLevel(t *testing.T, r response, what string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := r.Raw[key]; ok {
			t.Errorf("%s：字段 %q 不该出现在顶层", what, key)
		}
	}
}

// assertNoKey 断言对象里没有这个键（区别于"键存在但值为 null"）。
//
// 树选择结构的叶子节点就是靠"没有 children 键"表达的，
// 输出空数组会让前端把叶子当成可展开节点。
func assertNoKey(t *testing.T, obj map[string]any, key, what string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Errorf("%s：不该有 %q 键，实际值=%v", what, key, obj[key])
	}
}

// assertString 断言这些字段是字符串类型。
//
// status / isFrame / menuType 这些在 Java 里是 String，前端用 === 严格比较。
// 建模成数字的话前端判断永远不成立，且提交时反序列化直接失败。
func assertString(t *testing.T, obj map[string]any, what string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := obj[key]
		if !ok {
			t.Errorf("%s：缺少字段 %q", what, key)
			continue
		}
		if _, isStr := value.(string); !isStr {
			t.Errorf("%s：字段 %q 应为字符串（前端用 === 比较），实际是 %T（值 %v）", what, key, value, value)
		}
	}
}

// assertField 断言字段值相等。
//
// 数字比对走 normalize，所以传 int、int64、float64 都行，
// 不用像以前那样记得写 float64(0)。原因见 normalize 的注释。
func assertField(t *testing.T, obj map[string]any, key string, want any, what string) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Errorf("%s：缺少字段 %q", what, key)
		return
	}
	if normalize(got) != normalize(want) {
		t.Errorf("%s：字段 %q 期望 %v，实际 %v", what, key, want, got)
	}
}

// assertNoPasswordLeak 递归检查响应里有没有暴露密码。
//
// 对每个请求自动执行。用递归遍历而不是字符串匹配，避免
// sys.account.passwordValidateDays 这类键名造成误报，
// 也能穿透嵌套对象和数组。
func assertNoPasswordLeak(t *testing.T, path string, r response) {
	t.Helper()
	walkJSON(r.Raw, func(key string, value any) {
		if key != "password" {
			return
		}
		text, ok := value.(string)
		if !ok || text == "" || text == "******" {
			return
		}
		t.Errorf("%s：响应里泄漏了密码字段（值=%q）", path, text)
	})
}

func walkJSON(node any, visit func(key string, value any)) {
	switch v := node.(type) {
	case map[string]any:
		for key, value := range v {
			visit(key, value)
			walkJSON(value, visit)
		}
	case []any:
		for _, item := range v {
			walkJSON(item, visit)
		}
	}
}

// ---------- 取值工具 ----------

// dataObject 取 data 字段并断言它是对象。
func dataObject(t *testing.T, r response, what string) map[string]any {
	t.Helper()
	raw, ok := r.Raw["data"]
	if !ok {
		t.Fatalf("%s：响应里没有 data 字段，顶层只有 %v", what, topKeys(r.Raw))
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s：data 应为对象，实际是 %T", what, raw)
	}
	return obj
}

// dataArray 取 data 字段并断言它是数组。
func dataArray(t *testing.T, r response, what string) []map[string]any {
	t.Helper()
	raw, ok := r.Raw["data"]
	if !ok {
		t.Fatalf("%s：响应里没有 data 字段，顶层只有 %v", what, topKeys(r.Raw))
	}
	return toObjects(t, raw, what+" 的 data")
}

// pageRows 取分页响应的 rows，并顺带断言 total/rows 是平铺的。
func pageRows(t *testing.T, r response, what string) []map[string]any {
	t.Helper()
	assertTopLevel(t, r, what, "total", "rows")
	assertNoTopLevel(t, r, what, "data")
	return toObjects(t, r.Raw["rows"], what+" 的 rows")
}

func toObjects(t *testing.T, raw any, what string) []map[string]any {
	t.Helper()
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s 应为数组，实际是 %T", what, raw)
	}
	result := make([]map[string]any, 0, len(list))
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s 第 %d 个元素应为对象，实际是 %T", what, i, item)
		}
		result = append(result, obj)
	}
	return result
}

// idOf 从响应对象里取出主键，转成 int64。
//
// 【不要用字符串充当 ID 塞进请求体】JSON 里的数字解析出来是 float64，
// 直接 fmt.Sprint 大数会变成 1e+06 这种科学计数法；
// 而模型里主键是 int64，传字符串会直接反序列化失败。
func idOf(t *testing.T, obj map[string]any, key string) int64 {
	t.Helper()
	raw, ok := obj[key]
	if !ok {
		t.Fatalf("对象里没有主键字段 %q，实际字段=%v", key, topKeys(obj))
	}
	number, ok := raw.(float64)
	if !ok {
		t.Fatalf("主键 %q 应为数字，实际是 %T（值 %v）", key, raw, raw)
	}
	return int64(number)
}

// idPath 把主键转成可以拼进 URL 的字符串。
func idPath(id int64) string {
	return strconv.FormatInt(id, 10)
}

// findBy 在列表里按字段值找一条记录，找不到返回 nil。
func findBy(list []map[string]any, key string, want any) map[string]any {
	target := normalize(want)
	for _, item := range list {
		if normalize(item[key]) == target {
			return item
		}
	}
	return nil
}

// normalize 把值转成可比对的字符串，专门处理 JSON 数字。
//
// 【这是踩过的坑，不要改回 fmt.Sprint】
// JSON 里的数字解析出来一律是 float64，而 fmt.Sprint 对 float64 走的是 %v（等价 %g），
// 大整数会变成科学计数法：
//
//	fmt.Sprint(float64(1000007))  =>  "1.000007e+06"
//	fmt.Sprint(int64(1000007))    =>  "1000007"
//
// 两边永远不相等，于是 findBy 静默失配、断言无声失效。
// 主键小于 100 万时完全看不出来 —— 压测灌完数据把 auto_increment 顶过 100 万，
// 这个 bug 才浮出来。
func normalize(value any) string {
	number, ok := value.(float64)
	if !ok {
		return fmt.Sprint(value)
	}
	// 整数值用定点输出，避免指数形式；小数保留原样
	if number == math.Trunc(number) && math.Abs(number) < 1e15 {
		return strconv.FormatInt(int64(number), 10)
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

func topKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	return keys
}

func truncBody(body []byte) string {
	const max = 300
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}

// ---------- 构造测试数据的小工具 ----------

// payload 复制一份 map，避免用例之间互相污染。
func payload(base map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for key, value := range base {
		result[key] = value
	}
	return result
}

// with 复制并覆盖一个字段。
func with(base map[string]any, key string, value any) map[string]any {
	result := payload(base)
	result[key] = value
	return result
}

// omit 复制并删掉一个字段（模拟"少传"）。
func omit(base map[string]any, keys ...string) map[string]any {
	result := payload(base)
	for _, key := range keys {
		delete(result, key)
	}
	return result
}

// repeatText 生成指定长度的中文串，用于超长校验。
func repeatText(n int) string {
	return strings.Repeat("测", n)
}
