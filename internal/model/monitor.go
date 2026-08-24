package model

import "ruoyi-go/pkg/types"

// UserOnline 在线用户，数据来自 Redis 的 login_tokens:*。
//
// 字段名对齐 Java 版 SysUserOnline，前端表格直接按这些 prop 取值。
type UserOnline struct {
	// TokenID 就是 login_user_key（uuid），强退时用它定位会话
	TokenID       string     `json:"tokenId"`
	UserName      string     `json:"userName"`
	DeptName      string     `json:"deptName"`
	IPAddr        string     `json:"ipaddr"`
	LoginLocation string     `json:"loginLocation"`
	Browser       string     `json:"browser"`
	OS            string     `json:"os"`
	LoginTime     types.Time `json:"loginTime"`
}

// OnlineQuery 在线用户的查询条件。
type OnlineQuery struct {
	IPAddr   string `form:"ipaddr"`
	UserName string `form:"userName"`
}

// CacheItem 缓存监控里的一条记录。
type CacheItem struct {
	CacheName  string `json:"cacheName"`
	CacheKey   string `json:"cacheKey"`
	CacheValue string `json:"cacheValue"`
	Remark     string `json:"remark"`
}

// ServerInfo 服务监控数据。
//
// 字段结构对齐 Java 版 Server，前端模板写死了 server.cpu.xxx 这些路径。
// jvm 段在 Go 侧填的是 Go runtime 的对应信息（前端标题仍显示"虚拟机信息"）。
type ServerInfo struct {
	CPU      CPUInfo       `json:"cpu"`
	Mem      MemInfo       `json:"mem"`
	JVM      RuntimeInfo   `json:"jvm"`
	Sys      SysInfo       `json:"sys"`
	SysFiles []SysFileInfo `json:"sysFiles"`
}

type CPUInfo struct {
	CPUNum int     `json:"cpuNum"`
	Total  float64 `json:"total"`
	Sys    float64 `json:"sys"`
	Used   float64 `json:"used"`
	Wait   float64 `json:"wait"`
	Free   float64 `json:"free"`
}

// MemInfo 单位为 GB，与 Java 版一致（前端直接显示不做换算）。
type MemInfo struct {
	Total float64 `json:"total"`
	Used  float64 `json:"used"`
	Free  float64 `json:"free"`
	Usage float64 `json:"usage"`
}

// RuntimeInfo 对应 Java 的 Jvm 段。
type RuntimeInfo struct {
	Total     float64 `json:"total"`
	Max       float64 `json:"max"`
	Free      float64 `json:"free"`
	Used      float64 `json:"used"`
	Usage     float64 `json:"usage"`
	Version   string  `json:"version"`
	Home      string  `json:"home"`
	Name      string  `json:"name"`
	StartTime string  `json:"startTime"`
	RunTime   string  `json:"runTime"`
	InputArgs string  `json:"inputArgs"`
}

type SysInfo struct {
	ComputerName string `json:"computerName"`
	ComputerIP   string `json:"computerIp"`
	UserDir      string `json:"userDir"`
	OSName       string `json:"osName"`
	OSArch       string `json:"osArch"`
}

type SysFileInfo struct {
	DirName     string  `json:"dirName"`
	SysTypeName string  `json:"sysTypeName"`
	TypeName    string  `json:"typeName"`
	Total       string  `json:"total"`
	Free        string  `json:"free"`
	Used        string  `json:"used"`
	Usage       float64 `json:"usage"`
}
