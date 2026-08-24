package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/types"
)

// processStart 进程启动时间，用于计算运行时长。
var processStart = time.Now()

// gb 字节转 GB 的除数。
const gb = 1024 * 1024 * 1024

// ServerStatus 采集服务器运行状态。
//
// Java 版用 OSHI，Go 侧用 gopsutil，字段结构保持一致，前端不用改。
// "jvm" 那一段填的是 Go runtime 的对应信息。
func ServerStatus(ctx context.Context) (*model.ServerInfo, error) {
	info := &model.ServerInfo{}

	info.CPU = collectCPU(ctx)
	info.Mem = collectMem(ctx)
	info.JVM = collectRuntime()
	info.Sys = collectSys()
	info.SysFiles = collectDisks(ctx)

	return info, nil
}

func collectCPU(ctx context.Context) model.CPUInfo {
	result := model.CPUInfo{CPUNum: runtime.NumCPU(), Total: 100}

	// 采样 500ms 取使用率。Java 版也是采样式的，瞬时值没有意义。
	percents, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false)
	if err != nil || len(percents) == 0 {
		result.Free = 100
		return result
	}
	used := round2(percents[0])
	result.Used = used
	result.Free = round2(100 - used)

	// 系统态占比需要 CPU times，取不到就留 0，不影响页面展示
	if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
		if total > 0 {
			result.Sys = round2(t.System / total * 100)
			result.Wait = round2(t.Iowait / total * 100)
		}
	}
	return result
}

func collectMem(ctx context.Context) model.MemInfo {
	stat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return model.MemInfo{}
	}
	return model.MemInfo{
		Total: round2(float64(stat.Total) / gb),
		Used:  round2(float64(stat.Used) / gb),
		Free:  round2(float64(stat.Available) / gb),
		Usage: round2(stat.UsedPercent),
	}
}

// collectRuntime 填 Go 运行时信息到 jvm 段。
func collectRuntime() model.RuntimeInfo {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	// Sys 是向操作系统申请的总量，HeapAlloc 是当前在用的堆
	total := float64(ms.Sys) / 1024 / 1024
	used := float64(ms.HeapAlloc) / 1024 / 1024
	free := total - used
	if free < 0 {
		free = 0
	}
	usage := 0.0
	if total > 0 {
		usage = used / total * 100
	}

	home, _ := os.Executable()
	return model.RuntimeInfo{
		Total:     round2(total),
		Max:       round2(total),
		Used:      round2(used),
		Free:      round2(free),
		Usage:     round2(usage),
		Version:   runtime.Version(),
		Home:      home,
		Name:      "Go Runtime",
		StartTime: types.Time(processStart).String(),
		RunTime:   formatDuration(time.Since(processStart)),
		InputArgs: fmt.Sprintf("GOMAXPROCS=%d NumGoroutine=%d", runtime.GOMAXPROCS(0), runtime.NumGoroutine()),
	}
}

func collectSys() model.SysInfo {
	hostname, _ := os.Hostname()
	workDir, _ := os.Getwd()
	return model.SysInfo{
		ComputerName: hostname,
		ComputerIP:   localIP(),
		UserDir:      workDir,
		OSName:       runtime.GOOS,
		OSArch:       runtime.GOARCH,
	}
}

func collectDisks(ctx context.Context) []model.SysFileInfo {
	files := make([]model.SysFileInfo, 0)

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return files
	}
	for _, p := range partitions {
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue // 光驱、无权限的挂载点会失败，跳过即可
		}
		files = append(files, model.SysFileInfo{
			DirName:     p.Mountpoint,
			SysTypeName: p.Fstype,
			TypeName:    p.Device,
			Total:       formatBytes(usage.Total),
			Free:        formatBytes(usage.Free),
			Used:        formatBytes(usage.Used),
			Usage:       round2(usage.UsedPercent),
		})
	}
	return files
}

// localIP 取本机对外 IP。
//
// 用 UDP "拨号"到公网地址来让内核选出出口网卡，不会真的发包。
// 直接遍历网卡容易选到 docker0 之类的虚拟接口。
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		if hostInfo, err := host.Info(); err == nil {
			return hostInfo.Hostname
		}
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%d天%d小时%d分钟", days, hours, minutes)
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
