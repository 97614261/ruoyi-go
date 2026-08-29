package middleware

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestDesensitizeJSON(t *testing.T) {
	secretValues := []string{"upper-secret", "old-secret", "quoted-secret", "nested-secret", "array-secret"}
	raw := `{
		"Password":"upper-secret",
		"oldPassword":"old-secret",
		"newPassword":123456,
		"confirmPassword":{"raw":"quoted-secret"},
		"profile":{"PASSWORD":"nested-secret"},
		"items":[{"NewPassword":"array-secret"}],
		"remark":"keep-me"
	}`

	got := desensitizeJSON(raw)
	for _, secret := range secretValues {
		if strings.Contains(got, secret) {
			t.Errorf("脱敏结果不应包含 %q，实际=%s", secret, got)
		}
	}
	if strings.Contains(got, "123456") {
		t.Errorf("非字符串密码值也必须整体脱敏，实际=%s", got)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("脱敏结果应保持合法 JSON：%v，实际=%s", err, got)
	}
	if decoded["Password"] != redactedValue || decoded["oldPassword"] != redactedValue ||
		decoded["newPassword"] != redactedValue || decoded["confirmPassword"] != redactedValue {
		t.Errorf("顶层敏感字段应统一替换，实际=%v", decoded)
	}
	profile := decoded["profile"].(map[string]any)
	if profile["PASSWORD"] != redactedValue {
		t.Errorf("嵌套且大小写变化的密码字段应被替换，实际=%v", profile)
	}
	if decoded["remark"] != "keep-me" {
		t.Errorf("非敏感字段不应被修改，实际=%v", decoded["remark"])
	}
}

func TestDesensitizeJSONEscapesAndInvalidInput(t *testing.T) {
	got := desensitizeJSON(`{"password":"a\"b","remark":"正常"}`)
	if strings.Contains(got, `a\"b`) || !strings.Contains(got, `"password":"******"`) {
		t.Errorf("带转义引号的密码应整体替换，实际=%s", got)
	}

	for _, raw := range []string{
		`{"password":"secret"`,
		`{"password":"secret"} trailing-secret`,
	} {
		if got := desensitizeJSON(raw); got != unparseableLogValue {
			t.Errorf("非法 JSON 不得原样记入日志，输入=%q，实际=%q", raw, got)
		}
	}
	if got := desensitizeJSON(`"raw-secret"`); got != unsupportedLogValue {
		t.Errorf("没有字段上下文的 JSON 标量应保守省略，实际=%q", got)
	}
}

func TestDesensitizeQuery(t *testing.T) {
	raw := "userId=7&%70assword=first-secret&PASSWORD=second-secret&newPassword=third-secret&remark=keep-me"
	got := desensitizeQuery(raw)
	for _, secret := range []string{"first-secret", "second-secret", "third-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("查询串脱敏结果不应包含 %q，实际=%s", secret, got)
		}
	}

	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("脱敏结果应保持合法查询串：%v，实际=%s", err, got)
	}
	for _, key := range []string{"password", "PASSWORD", "newPassword"} {
		if values.Get(key) != redactedValue {
			t.Errorf("字段 %q 应被替换，实际=%q", key, values.Get(key))
		}
	}
	if values.Get("remark") != "keep-me" || values.Get("userId") != "7" {
		t.Errorf("非敏感查询参数不应被修改，实际=%v", values)
	}

	if got := desensitizeQuery("password=%ZZsecret"); got != unparseableLogValue {
		t.Errorf("非法查询串不得原样记入日志，实际=%q", got)
	}
}

func TestDesensitizeBodyByContentType(t *testing.T) {
	if got := desensitizeBody(`{"Password":"secret"}`, "application/json; charset=utf-8"); strings.Contains(got, "secret") {
		t.Errorf("JSON 请求体泄漏密码：%s", got)
	}
	if got := desensitizeBody("Password=secret", "application/x-www-form-urlencoded"); strings.Contains(got, "secret") {
		t.Errorf("表单请求体泄漏密码：%s", got)
	}
	if got := desensitizeBody(multipartLogValue, "multipart/form-data; boundary=test"); got != multipartLogValue {
		t.Errorf("multipart 固定提示不应被改写，实际=%q", got)
	}
	if got := desensitizeBody("raw-secret", "text/plain"); got != unsupportedLogValue {
		t.Errorf("未知格式请求体应保守省略，实际=%q", got)
	}
}
