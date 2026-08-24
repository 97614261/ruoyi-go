// Package job 定时任务的命名注册表。[L1]
//
// 【为什么不是反射】
// Java 版把 invoke_target 当成 "bean 名.方法名(参数)" 用反射调用任意 Spring bean。
// 这是个 RCE 面 —— RuoYi 自己为此加了一圈补丁：白名单只允许
// com.ruoyi.quartz.task 包，黑名单挡掉 java.net.URL / InitialContext /
// snakeyaml / springframework / apache，再禁掉 rmi: / ldap: / http(s)。
//
// Go 侧改成显式注册：只有代码里 Register 过的名字才能被调用，查不到就是查不到。
// 注册表本身就是白名单，不需要黑名单，也没有"漏堵一个包"的可能。
//
// 代价是新增任务要改代码重新部署 —— 但 Java 版同样如此（白名单锁死在一个包里，
// 任务方法必须先写进代码）。这一点见 docs/DECISIONS.md 2026-08-23 的更正。
//
// 本包不依赖数据库，需要访问数据的任务由 service 层注册进来，避免循环依赖。
package job

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Task 一个可被调度的任务。
//
// args 是从 invoke_target 括号里解析出来的参数，已去掉引号和 L/D 后缀。
// 类型转换由任务自己做 —— Go 没有反射按签名匹配那一套，
// 也不需要：参数来自后台配置，本来就该由任务自己校验。
type Task func(ctx context.Context, args []string) error

var (
	mu       sync.RWMutex
	registry = make(map[string]Task)
)

// Register 注册一个任务。重复注册会 panic —— 那是编码错误，越早暴露越好。
//
// 名字建议沿用 Java 的 "对象.方法" 风格（如 ryTask.ryNoParams），
// 这样数据库里现存的 sys_job 记录不用改就能跑。
func Register(name string, task Task) {
	if name == "" || task == nil {
		panic("job.Register: 名称和任务都不能为空")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[name]; exists {
		panic("job.Register: 任务名重复注册: " + name)
	}
	registry[name] = task
}

// Lookup 按名字取任务。
func Lookup(name string) (Task, bool) {
	mu.RLock()
	defer mu.RUnlock()
	task, ok := registry[name]
	return task, ok
}

// Names 返回全部已注册的任务名，已排序。
//
// 校验失败时把可选值列给用户，省得他去翻代码猜名字。
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve 解析 invoke_target 并找出对应的任务。
//
// 支持的格式（与 Java 版一致，数据库里现存的记录可以直接用）：
//
//	ryTask.ryNoParams
//	ryTask.ryParams('ry')
//	ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)
func Resolve(invokeTarget string) (Task, []string, error) {
	name, args, err := Parse(invokeTarget)
	if err != nil {
		return nil, nil, err
	}
	task, ok := Lookup(name)
	if !ok {
		return nil, nil, fmt.Errorf("任务 %s 未注册，可用的任务：%s", name, strings.Join(Names(), "、"))
	}
	return task, args, nil
}

// Parse 拆出任务名和参数，不查注册表。
func Parse(invokeTarget string) (string, []string, error) {
	target := strings.TrimSpace(invokeTarget)
	if target == "" {
		return "", nil, fmt.Errorf("调用目标不能为空")
	}

	open := strings.Index(target, "(")
	if open < 0 {
		// 无参形式，整串就是任务名
		if strings.Contains(target, ")") {
			return "", nil, fmt.Errorf("调用目标 %q 括号不匹配", invokeTarget)
		}
		return target, nil, nil
	}

	if !strings.HasSuffix(target, ")") {
		return "", nil, fmt.Errorf("调用目标 %q 括号不匹配", invokeTarget)
	}
	name := strings.TrimSpace(target[:open])
	if name == "" {
		return "", nil, fmt.Errorf("调用目标 %q 缺少任务名", invokeTarget)
	}

	inner := strings.TrimSpace(target[open+1 : len(target)-1])
	if inner == "" {
		return name, nil, nil
	}

	tokens, err := splitArgs(inner)
	if err != nil {
		return "", nil, fmt.Errorf("调用目标 %q 的参数无法解析: %w", invokeTarget, err)
	}

	args := make([]string, 0, len(tokens))
	for _, token := range tokens {
		args = append(args, normalizeArg(token))
	}
	return name, args, nil
}

// splitArgs 按逗号切分参数，**引号内的逗号不算分隔符**。
//
// 不能简单 strings.Split(inner, ",") —— 'a,b' 这样的字符串参数会被切成两半。
// Java 那边用的是带前瞻的正则，Go 的 regexp 不支持前瞻，所以手写扫描。
func splitArgs(inner string) ([]string, error) {
	var (
		tokens []string
		buf    strings.Builder
		quote  rune
	)

	for _, r := range inner {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			buf.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			buf.WriteRune(r)
		case r == ',':
			tokens = append(tokens, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("引号没有闭合")
	}
	tokens = append(tokens, buf.String())
	return tokens, nil
}

// normalizeArg 把字面量整理成干净的字符串。
//
// 对齐 Java 的类型判定规则，但结果统一是字符串：
//
//	'ry' / "ry"  ->  ry        去引号
//	true         ->  true      原样
//	2000L        ->  2000      去掉长整型后缀
//	316.50D      ->  316.50    去掉浮点后缀
//	100          ->  100       原样
//
// 后缀只在剩下的部分确实是数字时才去掉，否则 "SQLD" 这种会被砍成 "SQL"。
func normalizeArg(token string) string {
	arg := strings.TrimSpace(token)
	if len(arg) >= 2 {
		first, last := arg[0], arg[len(arg)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return arg[1 : len(arg)-1]
		}
	}

	if len(arg) >= 2 {
		suffix := arg[len(arg)-1]
		if suffix == 'L' || suffix == 'l' || suffix == 'D' || suffix == 'd' {
			body := arg[:len(arg)-1]
			if _, err := strconv.ParseFloat(body, 64); err == nil {
				return body
			}
		}
	}
	return arg
}
