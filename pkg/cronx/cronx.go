// Package cronx 把 Quartz 风格的 cron 表达式适配到 robfig/cron。
//
// 【为什么需要这一层】
// 前端的 cron 生成器和数据库里现存的 sys_job 记录用的都是 Quartz 语法，
// 而 robfig/cron 实现的是标准 cron。两者长得很像，差异全是**静默出错**的那种：
//
//	段数     Quartz 是 6 或 7 段（秒在最前，年可选），robfig 默认 5 段
//	星期     Quartz 1=周日..7=周六；robfig 0=周日..6=周六，**整体差 1**
//	?        两边都支持（robfig 内部按 * 处理）
//	L W #    Quartz 独有，robfig 完全不认
//
// 星期差 1 是最危险的一处：前端选「每周一」生成 2，直接喂给 robfig 会变成周二。
// 不报错、不告警，只是每周错一天，几乎不可能靠肉眼发现。
package cronx

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Parser 六段解析器（秒 分 时 日 月 周），与 Quartz 前六段对齐。
var Parser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// unsupported Quartz 独有、robfig 不认的特殊字符。
//
// 宁可明确报错也不要"尽力而为地跑" —— 静默按错误的时间执行，
// 比配不上去糟糕得多。
var unsupported = []struct {
	token string
	desc  string
}{
	{"L", "L（月末/最后一个星期几）"},
	{"W", "W（最近的工作日）"},
	{"#", "#（第几个星期几）"},
}

// Translate 把 Quartz 表达式转换成 robfig 能解析的六段表达式。
func Translate(expression string) (string, error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return "", fmt.Errorf("cron 表达式不能为空")
	}

	fields := strings.Fields(expr)
	if len(fields) < 6 || len(fields) > 7 {
		return "", fmt.Errorf("cron 表达式应为 6 或 7 段（秒 分 时 日 月 周 [年]），实际 %d 段", len(fields))
	}
	// 第 7 段是年。robfig 不支持年，而 Quartz 里它几乎总是 * ——
	// 真写了具体年份就必须拒绝，不能假装支持
	if len(fields) == 7 && fields[6] != "*" && fields[6] != "?" {
		return "", fmt.Errorf("暂不支持指定年份（第 7 段 %q），请留空或填 *", fields[6])
	}
	fields = fields[:6]

	for _, field := range fields {
		upper := strings.ToUpper(field)
		for _, bad := range unsupported {
			if strings.Contains(upper, bad.token) {
				return "", fmt.Errorf("暂不支持 Quartz 的 %s 语法", bad.desc)
			}
		}
	}

	dow, err := shiftDayOfWeek(fields[5])
	if err != nil {
		return "", err
	}
	fields[5] = dow

	return strings.Join(fields, " "), nil
}

// shiftDayOfWeek 把 Quartz 的星期编号（1=周日..7=周六）转成
// robfig 的编号（0=周日..6=周六），也就是每个星期值减一。
//
// 【不能对整个字段无脑减 1】这里踩过坑：
//
//	3/2   周二起每隔 2 天   →  错减成 2/1（周二起每天），触发频率变 3.5 倍
//	2-6/2 周一到周五隔天    →  错减成 1-5/1
//
// `/` 后面是**步长**，是"每隔几"，不是星期几，绝不能跟着减。
// 所以先按逗号拆列表项，每项再拆出步长，只对星期值做偏移。
//
// SUN/MON 这类名称两边含义相同，原样保留。
func shiftDayOfWeek(field string) (string, error) {
	if field == "*" || field == "?" || field == "" {
		return field, nil
	}

	// 先拆列表：2-4/2,6 这种如果直接按 / 拆，会把 "2,6" 整个当成步长
	items := strings.Split(field, ",")
	for i, item := range items {
		value, step, hasStep := strings.Cut(item, "/")
		shifted, err := shiftWeekdayValues(value)
		if err != nil {
			return "", err
		}
		if hasStep {
			items[i] = shifted + "/" + step
		} else {
			items[i] = shifted
		}
	}
	return strings.Join(items, ","), nil
}

// shiftWeekdayValues 只处理星期值部分（可能是 *、?、单个数字、范围 a-b 或名称）。
func shiftWeekdayValues(value string) (string, error) {
	var out strings.Builder
	number := strings.Builder{}

	flush := func() error {
		if number.Len() == 0 {
			return nil
		}
		text := number.String()
		number.Reset()

		parsed := 0
		for _, r := range text {
			parsed = parsed*10 + int(r-'0')
		}
		if parsed < 1 || parsed > 7 {
			return fmt.Errorf("星期取值应在 1-7 之间（1=周日），实际 %s", text)
		}
		out.WriteString(fmt.Sprint(parsed - 1))
		return nil
	}

	for _, r := range value {
		if r >= '0' && r <= '9' {
			number.WriteRune(r)
			continue
		}
		if err := flush(); err != nil {
			return "", err
		}
		out.WriteRune(r)
	}
	if err := flush(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Parse 转换并解析，返回可用于调度的 Schedule。
func Parse(expression string) (cron.Schedule, error) {
	translated, err := Translate(expression)
	if err != nil {
		return nil, err
	}
	schedule, err := Parser.Parse(translated)
	if err != nil {
		// 不要把 robfig 的原始报错直接抛给用户 —— 里面是转换后的表达式，
		// 和他在界面上填的那个对不上，只会更困惑
		return nil, fmt.Errorf("cron 表达式 %q 不正确", expression)
	}
	return schedule, nil
}

// Valid 表达式是否合法。
func Valid(expression string) bool {
	_, err := Parse(expression)
	return err == nil
}

// Next 算下一次执行时间，表达式非法时返回零值。
func Next(expression string, from time.Time) time.Time {
	schedule, err := Parse(expression)
	if err != nil {
		return time.Time{}
	}
	return schedule.Next(from)
}
