package job

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

// 内置的演示任务，对齐 ry_20260417.sql 里 sys_job 的三条种子数据：
//
//	ryTask.ryNoParams
//	ryTask.ryParams('ry')
//	ryTask.ryMultipleParams('ry', true, 2000L, 316.50D, 100)
//
// 【它们没有任何业务价值】Java 版的 RyTask 三个方法全是 System.out.println，
// 这里照搬只是为了让数据库里现存的三条记录点得动、能看到日志。
//
// 真实业务任务写在下面 registerBusinessTasks 里，或者由 service 层调
// job.Register 注册（需要访问数据库的任务必须走后者，本包不依赖 repository）。
func init() {
	Register("ryTask.ryNoParams", func(ctx context.Context, _ []string) error {
		slog.InfoContext(ctx, "定时任务：执行无参方法")
		return nil
	})

	Register("ryTask.ryParams", func(ctx context.Context, args []string) error {
		slog.InfoContext(ctx, "定时任务：执行有参方法", "params", strings.Join(args, ","))
		return nil
	})

	Register("ryTask.ryMultipleParams", func(ctx context.Context, args []string) error {
		// 演示参数怎么取：注册表只保证给到干净的字符串，
		// 类型转换和缺参处理由任务自己负责
		slog.InfoContext(ctx, "定时任务：执行多参方法",
			"字符串", argAt(args, 0),
			"布尔", argBool(args, 1),
			"长整型", argInt(args, 2),
			"浮点型", argFloat(args, 3),
			"整型", argInt(args, 4),
		)
		return nil
	})
}

// ---------- 参数读取小工具，写业务任务时可以直接用 ----------

// argAt 取第 index 个参数，越界返回空串。
func argAt(args []string, index int) string {
	if index < 0 || index >= len(args) {
		return ""
	}
	return args[index]
}

func argInt(args []string, index int) int64 {
	value, _ := strconv.ParseInt(argAt(args, index), 10, 64)
	return value
}

func argFloat(args []string, index int) float64 {
	value, _ := strconv.ParseFloat(argAt(args, index), 64)
	return value
}

func argBool(args []string, index int) bool {
	value, _ := strconv.ParseBool(argAt(args, index))
	return value
}
