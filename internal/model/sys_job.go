package model

import (
	"encoding/json"

	"ruoyi-go/pkg/types"
)

// 任务状态。注意与 sys_user 的 status 含义不同：这里 1 是"暂停"不是"停用"。
const (
	JobStatusNormal = "0" // 正常（已调度）
	JobStatusPause  = "1" // 暂停
)

// 是否并发执行。**0 是允许、1 是禁止**，和直觉相反，别写反了。
const (
	JobConcurrentAllow  = "0"
	JobConcurrentForbid = "1"
)

// 计划执行错误策略。
//
// ⚠️ 只有 JobMisfireAbandon 是真实生效的，另外两个存下来但不会被执行 ——
// 内存调度器没有"跨重启的错过触发"这个概念。Java 版默认配置（ScheduleConfig
// 整个被注释掉、走 RAMJobStore）同样如此。详见 docs/DECISIONS.md。
const (
	JobMisfireDefault   = "0" // 默认
	JobMisfireImmediate = "1" // 立即执行
	JobMisfireOnce      = "2" // 执行一次
	JobMisfireAbandon   = "3" // 放弃执行
)

// 任务执行结果。
const (
	JobLogStatusNormal = "0" // 正常
	JobLogStatusFail   = "1" // 失败
)

// SysJob 定时任务调度表 sys_job。
//
// 【主键是复合的】DDL 里是 primary key (job_id, job_name, job_group)，
// 但只有 job_id 是 auto_increment，业务上所有操作也都按 job_id 走，
// 所以这里只把 job_id 标成主键。
type SysJob struct {
	JobID    int64  `gorm:"column:job_id;primaryKey;autoIncrement" json:"jobId" excel:"name:任务序号;cell:numeric"`
	JobName  string `gorm:"column:job_name" json:"jobName" binding:"notblank,max=64" excel:"name:任务名称"`
	JobGroup string `gorm:"column:job_group" json:"jobGroup" binding:"omitempty,max=64" excel:"name:任务组名"`
	// InvokeTarget 形如 ryTask.ryParams('ry')。
	// 能不能调由 internal/job 的注册表决定，不在这里用 oneof 之类写死。
	InvokeTarget   string `gorm:"column:invoke_target" json:"invokeTarget" binding:"notblank,max=500" excel:"name:调用目标字符串"`
	CronExpression string `gorm:"column:cron_expression" json:"cronExpression" binding:"notblank,max=255" excel:"name:执行表达式"`
	// MisfirePolicy / Concurrent / Status 三个列都有 DEFAULT，
	// 所以不加 required —— 与 sys_dept.status 的处理一致，见 CONVENTIONS 的偏离登记
	MisfirePolicy string     `gorm:"column:misfire_policy" json:"misfirePolicy" excel:"name:计划策略;converter:0=默认,1=立即触发执行,2=触发一次执行,3=不触发立即执行"`
	Concurrent    string     `gorm:"column:concurrent" json:"concurrent" excel:"name:并发执行;converter:0=允许,1=禁止"`
	Status        string     `gorm:"column:status" json:"status" excel:"name:任务状态;converter:0=正常,1=暂停"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark        string     `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// NextValidTime 下次执行时间，非表字段。
	// Java 版是 getNextValidTime() 这个 getter，Jackson 会把它序列化出去，
	// 前端的任务详情弹窗读的就是它。由 service 在返回前填充。
	NextValidTime types.Time `gorm:"-" json:"-"`
}

func (SysJob) TableName() string { return "sys_job" }

// MarshalJSON 补出 nextValidTime。
//
// 用自定义序列化而不是直接给字段加 json tag：为空时（cron 非法或未调度）
// 这个键整个不出现，与 Java 返回 null 的效果对前端一致 —— 前端是
// v-if 判断有没有值，不是判断是不是空串。
func (j SysJob) MarshalJSON() ([]byte, error) {
	type alias SysJob
	if j.NextValidTime.IsZero() {
		return json.Marshal(struct{ alias }{alias(j)})
	}
	return json.Marshal(struct {
		alias
		NextValidTime types.Time `json:"nextValidTime"`
	}{alias(j), j.NextValidTime})
}

// SysJobLog 定时任务调度日志表 sys_job_log。
type SysJobLog struct {
	JobLogID     int64  `gorm:"column:job_log_id;primaryKey;autoIncrement" json:"jobLogId" excel:"name:日志序号;cell:numeric"`
	JobName      string `gorm:"column:job_name" json:"jobName" excel:"name:任务名称"`
	JobGroup     string `gorm:"column:job_group" json:"jobGroup" excel:"name:任务组名"`
	InvokeTarget string `gorm:"column:invoke_target" json:"invokeTarget" excel:"name:调用目标字符串"`
	JobMessage   string `gorm:"column:job_message" json:"jobMessage" excel:"name:日志信息"`
	Status       string `gorm:"column:status" json:"status" excel:"name:执行状态;converter:0=正常,1=失败"`
	// ExceptionInfo 列长 2000，写入前必须截断，否则 SQL 报错把执行结果也搞丢
	ExceptionInfo string     `gorm:"column:exception_info" json:"exceptionInfo" excel:"name:异常信息"`
	StartTime     types.Time `gorm:"column:start_time" json:"startTime" excel:"name:开始时间;width:30"`
	EndTime       types.Time `gorm:"column:end_time" json:"endTime" excel:"name:结束时间;width:30"`
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"`
}

func (SysJobLog) TableName() string { return "sys_job_log" }

// JobQuery 定时任务列表的查询条件。
type JobQuery struct {
	JobName      string `form:"jobName"`
	JobGroup     string `form:"jobGroup"`
	Status       string `form:"status"`
	InvokeTarget string `form:"invokeTarget"`
}

// JobLogQuery 调度日志列表的查询条件。
type JobLogQuery struct {
	JobLogID     int64  `form:"jobLogId"`
	JobName      string `form:"jobName"`
	JobGroup     string `form:"jobGroup"`
	Status       string `form:"status"`
	InvokeTarget string `form:"invokeTarget"`
	BeginTime    string `form:"params[beginTime]"`
	EndTime      string `form:"params[endTime]"`
}

// JobSortColumns 允许排序的字段白名单。
var JobSortColumns = map[string]string{
	"jobId":      "job_id",
	"jobName":    "job_name",
	"jobGroup":   "job_group",
	"status":     "status",
	"createTime": "create_time",
}

// JobLogSortColumns 允许排序的字段白名单。
var JobLogSortColumns = map[string]string{
	"jobLogId":  "job_log_id",
	"jobName":   "job_name",
	"jobGroup":  "job_group",
	"status":    "status",
	"startTime": "start_time",
	"endTime":   "end_time",
}

// JobStatusBody /monitor/job/changeStatus 和 /run 的请求体。
//
// 和 changeStatus 的其它模块一样，前端只传两个字段，
// 绑到 SysJob 上会被 jobName 的 notblank 拒掉，必须用专用 DTO。
type JobStatusBody struct {
	JobID  int64  `json:"jobId" binding:"required"`
	Status string `json:"status"`
	// JobGroup 前端 run 的时候会一起传，收下但不用于定位（按 jobId 查即可）
	JobGroup string `json:"jobGroup"`
}
