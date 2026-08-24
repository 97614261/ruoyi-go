package excelx

import (
	"io"
	"runtime"
	"testing"
	"time"
)

// benchRow 一行导出数据，字段数量和类型贴近真实的 SysUser。
type benchRow struct {
	UserID      int64  `excel:"name:用户序号;cell:numeric"`
	UserName    string `excel:"name:登录名称"`
	NickName    string `excel:"name:用户名称"`
	Email       string `excel:"name:用户邮箱"`
	Phonenumber string `excel:"name:手机号码;cell:text"`
	Sex         string `excel:"name:用户性别;converter:0=男,1=女,2=未知"`
	Status      string `excel:"name:账号状态;converter:0=正常,1=停用"`
	LoginIP     string `excel:"name:最后登录IP"`
	DeptName    string `excel:"name:部门名称"`
	Leader      string `excel:"name:部门负责人"`
}

func makeRows(n int) []benchRow {
	rows := make([]benchRow, n)
	for i := range rows {
		rows[i] = benchRow{
			UserID:      int64(900000 + i),
			UserName:    "perf_user_name_placeholder",
			NickName:    "压测用户名称占位",
			Email:       "someone@example.com",
			Phonenumber: "13800000000",
			Sex:         "0",
			Status:      "0",
			LoginIP:     "192.168.100.200",
			DeptName:    "研发部门",
			Leader:      "负责人",
		}
	}
	return rows
}

// BenchmarkExport 量一下导出的内存开销。
//
// 【为什么需要它】
// CONVENTIONS 第一节写着"导出的真实瓶颈在数据列表而非工作簿" ——
// 那是一句**从原理推出来的话，从来没量过**。同一类推断这个项目里已经错过两次
// （find_in_set 只占 3.6%、定时任务的能力差异被高估），所以这里要有数字。
//
// 跑法：
//
//	go test ./pkg/excelx/ -bench=Export -benchmem -run=^$
//
// 关注 B/op：那是**每导出一次**分配的字节数。乘以并发数就是内存峰值的量级。
// excelx 内部走 StreamWriter，工作簿本身只保留少量行；
// 但传进去的那个切片是完整装在内存里的，两者要分开看 —— 下面 ReportSliceSize 就是干这个的。
func BenchmarkExport(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		rows := makeRows(size)
		b.Run(sizeName(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Export(io.Discard, "sheet", rows); err != nil {
					b.Fatalf("导出失败: %v", err)
				}
			}
		})
	}
}

// TestReportExportMemory 分别报出「数据切片」和「导出过程」各占多少内存。
//
// 这不是断言型测试，是把数字打出来给人看的 —— 跑：
//
//	go test ./pkg/excelx/ -run=ReportExportMemory -v
//
// 之所以要分开量：如果瓶颈真在数据切片，那么优化工作簿（换库、调参数）
// 一点用都没有，得改成分批查库边读边写。
func TestReportExportMemory(t *testing.T) {
	const size = 100000

	var before, afterSlice, afterExport runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	rows := makeRows(size)

	runtime.GC()
	runtime.ReadMemStats(&afterSlice)
	sliceMB := float64(afterSlice.HeapAlloc-before.HeapAlloc) / 1024 / 1024

	// 【采样必须限频】runtime.ReadMemStats 会 stop-the-world。
	// 之前写成死循环连续调用，测一次要 31 秒，而且测的基本是采样自己的开销。
	//
	// 【两个 channel，不是一个】stop 由主协程关、finished 由采样协程关。
	// 共用一个的话两边都 close，直接 panic: close of closed channel。
	var peak uint64
	stop := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak {
					peak = m.HeapAlloc
				}
			}
		}
	}()

	if err := Export(io.Discard, "用户数据", rows); err != nil {
		close(stop)
		<-finished
		t.Fatalf("导出失败: %v", err)
	}
	close(stop)
	<-finished

	runtime.ReadMemStats(&afterExport)
	// 采样有可能整个错过峰值（导出很快时一次都没采到），
	// 用结束时的堆兜底，免得算出 0 然后除零
	if peak < afterExport.HeapAlloc {
		peak = afterExport.HeapAlloc
	}
	peakMB := float64(peak) / 1024 / 1024

	t.Logf("行数            : %d", size)
	t.Logf("数据切片占用    : %.1f MB", sliceMB)
	t.Logf("导出期间堆峰值  : %.1f MB", peakMB)
	t.Logf("→ 切片占峰值的  : %.0f%%", sliceMB/peakMB*100)
	t.Log("如果这个比例很高，说明瓶颈确实在数据列表，优化工作簿没用，")
	t.Log("得改成分批查库边读边喂给 StreamWriter。")

	runtime.KeepAlive(rows)
}

func sizeName(n int) string {
	switch {
	case n >= 100000:
		return "10万行"
	case n >= 10000:
		return "1万行"
	default:
		return "1000行"
	}
}
