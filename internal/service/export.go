package service

import (
	"ruoyi-go/pkg/errs"
)

// MaxExportRows 单次导出的行数上限。
//
// 【这个数是实测定出来的，不是格式上限】
// xlsx 单表能放 1048575 行，但那是**格式**能装多少，不是**进程**扛得住多少。
// pkg/excelx 的基准（10 列、贴近 SysUser 的结构）：
//
//	行数      堆峰值    分配总量   分配次数    耗时
//	1 万      ~9 MB     50 MB      98 万       80 ms
//	10 万     ~90 MB    321 MB     980 万      796 ms
//
// 完全线性，约 900 字节/行常驻、每行 98 次分配。
// 按格式上限 1048575 行外推就是 **约 940 MB 堆峰值**，
// 而整个服务压测时的常驻内存才 65 MB —— 一次导出把内存翻十几倍，
// 两三个人同时点就 OOM。那正好抵消掉换 Go 省下来的内存。
//
// 10 万行 ≈ 90 MB，配合 MaxConcurrentExports=2 最多占 180 MB，是能接受的量级。
//
// 【为什么不是流式就没事】excelize 的 StreamWriter 确实在流式写，
// 它只是 churn 很凶（980 万次分配），堆峰值仍随行数线性增长。
// "用了 StreamWriter 所以内存恒定" 是想当然，实测不成立。
//
// 【与 Java 的差异】Java 版 ExcelUtil 的 sheetSize = 65536，超过拆多个工作表——
// 那是 xls 时代的遗留。我们保持单表，超限直接拒绝。
const MaxExportRows = 100000

// MaxConcurrentExports 同时进行的导出数上限。
//
// 没有这个限制的话，行数上限就是摆设：10 个人各导 10 万行 = 900 MB。
const MaxConcurrentExports = 2

// checkExportSize 校验导出行数。
//
// 【绝不截断】超限一律报错。截断后照常返回文件的话，用户拿到的是一份
// 看起来正常、实际少了数据的表格 —— 而且他不会发现。
// 宁可让他导不出来，也不能给他错的数据。
//
// 提示语必须说清三件事：查出来多少条、上限多少、下一步该怎么办。
// 只说"导出失败"等于让用户去猜。
func checkExportSize(count int) error {
	if count <= MaxExportRows {
		return nil
	}
	return errs.Newf(
		"本次查询共 %d 条数据，超过单次导出上限 %d 条。请缩小查询范围后分批导出"+
			"（例如按时间段、部门或状态筛选），导出的数据不会被截断，请勿直接使用部分结果。",
		count, MaxExportRows)
}
