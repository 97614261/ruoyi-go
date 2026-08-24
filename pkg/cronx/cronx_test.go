package cronx

import (
	"testing"
	"time"
)

// TestTranslateDayOfWeek 星期编号必须整体减 1。
//
// 【这是本包存在的唯一理由，也是最危险的一处】
// Quartz：1=周日 … 7=周六；robfig：0=周日 … 6=周六。
// 前端 cron 生成器选「每周一」吐出 2，直接喂给 robfig 就是周二 ——
// 不报错、不告警，只是每周错一天。
func TestTranslateDayOfWeek(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"每周一", "0 0 9 ? * 2", "0 0 9 ? * 1"},
		{"每周日", "0 0 9 ? * 1", "0 0 9 ? * 0"},
		{"每周六", "0 0 9 ? * 7", "0 0 9 ? * 6"},
		{"周一到周五", "0 0 9 ? * 2-6", "0 0 9 ? * 1-5"},
		{"周一三五", "0 0 9 ? * 2,4,6", "0 0 9 ? * 1,3,5"},
		// 【/ 后面是步长，不是星期几，绝不能跟着减 1】
		// 这几条曾经全错：3/2 被减成 2/1，触发频率变 3.5 倍，而且不报错
		{"从周二起每隔两天", "0 0 9 ? * 3/2", "0 0 9 ? * 2/2"},
		{"周一到周五隔天", "0 0 9 ? * 2-6/2", "0 0 9 ? * 1-5/2"},
		{"通配带步长", "0 0 9 ? * */3", "0 0 9 ? * */3"},
		{"列表里混着范围和步长", "0 0 9 ? * 2-4/2,7", "0 0 9 ? * 1-3/2,6"},
		{"名称带步长", "0 0 9 ? * MON/2", "0 0 9 ? * MON/2"},
		{"通配不动", "0 0 9 * * *", "0 0 9 * * *"},
		{"问号不动", "0 0 9 ? * ?", "0 0 9 ? * ?"},
		// 名称两边含义相同，不能也减 1
		{"星期用名称", "0 0 9 ? * MON", "0 0 9 ? * MON"},
		{"名称范围", "0 0 9 ? * MON-FRI", "0 0 9 ? * MON-FRI"},
		// 只有第 6 段是星期，其它段里的数字一个都不能动
		{"其它段的数字不受影响", "5 10 15 20 6 ?", "5 10 15 20 6 ?"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Translate(tc.in)
			if err != nil {
				t.Fatalf("Translate(%q) 报错: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Translate(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTranslateRejects 不支持的语法必须明确报错，不能"尽力而为"。
//
// 静默按错误的时间执行，比配不上去糟糕得多。
func TestTranslateRejects(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		wantIn string
	}{
		{"空表达式", "", "不能为空"},
		{"只有空格", "   ", "不能为空"},
		{"5 段标准 cron", "0 3 * * *", "6 或 7 段"},
		{"8 段", "0 0 3 * * ? * *", "6 或 7 段"},
		{"指定具体年份", "0 0 3 * * ? 2027", "不支持指定年份"},
		{"月末 L", "0 0 3 L * ?", "不支持"},
		{"最后一个周五 L", "0 0 3 ? * 6L", "不支持"},
		{"最近工作日 W", "0 0 3 15W * ?", "不支持"},
		{"第三个周五 #", "0 0 3 ? * 6#3", "不支持"},
		{"星期为 0", "0 0 3 ? * 0", "星期取值"},
		{"星期为 8", "0 0 3 ? * 8", "星期取值"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Translate(tc.in)
			if err == nil {
				t.Fatalf("Translate(%q) 本应报错", tc.in)
			}
			if !contains(err.Error(), tc.wantIn) {
				t.Errorf("Translate(%q) 的报错应包含 %q，实际 %q", tc.in, tc.wantIn, err)
			}
		})
	}
}

// TestTranslateAllowsYearWildcard 第 7 段是通配时应当被忽略而不是报错。
//
// 前端的 cron 生成器默认就会带一个 * 作为年份。
func TestTranslateAllowsYearWildcard(t *testing.T) {
	for _, expr := range []string{"0 0 3 * * ? *", "0 0 3 * * ? ?"} {
		got, err := Translate(expr)
		if err != nil {
			t.Fatalf("Translate(%q) 报错: %v", expr, err)
		}
		if got != "0 0 3 * * ?" {
			t.Errorf("Translate(%q) = %q，年份段应被丢掉", expr, got)
		}
	}
}

// TestNextMatchesWeekday 端到端验证：算出来的下次执行时间落在正确的星期。
//
// 上面的 TestTranslateDayOfWeek 只验证了字符串转换，
// 这条验证转换之后 robfig 真的按预期时间触发 —— 转换对了但解析器行为不同样白搭。
func TestNextMatchesWeekday(t *testing.T) {
	// 2026-08-23 是周日
	from := time.Date(2026, 8, 23, 12, 0, 0, 0, time.Local)

	cases := []struct {
		name string
		expr string
		want time.Weekday
	}{
		{"Quartz 2 = 周一", "0 0 9 ? * 2", time.Monday},
		{"Quartz 6 = 周五", "0 0 9 ? * 6", time.Friday},
		{"Quartz 1 = 周日", "0 0 9 ? * 1", time.Sunday},
		{"名称 MON = 周一", "0 0 9 ? * MON", time.Monday},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := Next(tc.expr, from)
			if next.IsZero() {
				t.Fatalf("Next(%q) 返回零值", tc.expr)
			}
			if next.Weekday() != tc.want {
				t.Errorf("Next(%q) = %s（%s），期望落在%s",
					tc.expr, next.Format("2006-01-02 15:04:05"), next.Weekday(), tc.want)
			}
		})
	}
}

// TestNextEveryTenSeconds 种子数据里的表达式要能算出下次时间。
func TestNextEveryTenSeconds(t *testing.T) {
	from := time.Date(2026, 8, 23, 12, 0, 3, 0, time.Local)
	next := Next("0/10 * * * * ?", from)

	if next.IsZero() {
		t.Fatal("Next 返回零值")
	}
	if next.Second() != 10 {
		t.Errorf("12:00:03 的下一次应是 12:00:10，实际 %s", next.Format("15:04:05"))
	}
}

// TestValidAndNextOnInvalid 非法表达式：Valid 为假，Next 返回零值而不是 panic。
func TestValidAndNextOnInvalid(t *testing.T) {
	const bad = "这不是 cron"
	if Valid(bad) {
		t.Errorf("Valid(%q) 应为 false", bad)
	}
	if next := Next(bad, time.Now()); !next.IsZero() {
		t.Errorf("Next(%q) 应返回零值，实际 %v", bad, next)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
