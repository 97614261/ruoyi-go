package model

import "ruoyi-go/pkg/types"

// 登录日志状态。
const (
	LoginStatusSuccess = "0"
	LoginStatusFail    = "1"
)

// SysLogininfor 系统访问记录表 sys_logininfor。
//
// 字段顺序即导出列顺序，与 Java 版 SysLogininfor 的声明顺序一致。
type SysLogininfor struct {
	InfoID        int64      `gorm:"column:info_id;primaryKey;autoIncrement" json:"infoId" excel:"name:序号;cell:numeric"`
	UserName      string     `gorm:"column:user_name" json:"userName" excel:"name:用户账号"`
	IPAddr        string     `gorm:"column:ipaddr" json:"ipaddr" excel:"name:登录地址"`
	LoginLocation string     `gorm:"column:login_location" json:"loginLocation" excel:"name:登录地点"`
	Browser       string     `gorm:"column:browser" json:"browser" excel:"name:浏览器"`
	OS            string     `gorm:"column:os" json:"os" excel:"name:操作系统"`
	Status        string     `gorm:"column:status" json:"status" excel:"name:登录状态;converter:0=成功,1=失败"`
	Msg           string     `gorm:"column:msg" json:"msg" excel:"name:提示消息"`
	LoginTime     types.Time `gorm:"column:login_time" json:"loginTime" excel:"name:访问时间;width:30"`
}

func (SysLogininfor) TableName() string { return "sys_logininfor" }

// 业务类型，对齐 Java 版 BusinessType 枚举的序号。
const (
	BusinessTypeOther   = 0
	BusinessTypeInsert  = 1
	BusinessTypeUpdate  = 2
	BusinessTypeDelete  = 3
	BusinessTypeGrant   = 4
	BusinessTypeExport  = 5
	BusinessTypeImport  = 6
	BusinessTypeForce   = 7
	BusinessTypeGenCode = 8
	BusinessTypeClean   = 9
)

// 操作状态。
const (
	OperStatusSuccess = 0
	OperStatusFail    = 1
)

// SysOperLog 操作日志记录表 sys_oper_log。
type SysOperLog struct {
	OperID        int64      `gorm:"column:oper_id;primaryKey;autoIncrement" json:"operId" excel:"name:操作序号;cell:numeric"`
	Title         string     `gorm:"column:title" json:"title" excel:"name:操作模块"`
	BusinessType  int        `gorm:"column:business_type" json:"businessType" excel:"name:业务类型;converter:0=其它,1=新增,2=修改,3=删除,4=授权,5=导出,6=导入,7=强退,8=生成代码,9=清空数据"`
	Method        string     `gorm:"column:method" json:"method" excel:"name:请求方法"`
	RequestMethod string     `gorm:"column:request_method" json:"requestMethod" excel:"name:请求方式"`
	OperatorType  int        `gorm:"column:operator_type" json:"operatorType" excel:"name:操作类别;converter:0=其它,1=后台用户,2=手机端用户"`
	OperName      string     `gorm:"column:oper_name" json:"operName" excel:"name:操作人员"`
	DeptName      string     `gorm:"column:dept_name" json:"deptName" excel:"name:部门名称"`
	OperURL       string     `gorm:"column:oper_url" json:"operUrl" excel:"name:请求地址"`
	OperIP        string     `gorm:"column:oper_ip" json:"operIp" excel:"name:操作地址"`
	OperLocation  string     `gorm:"column:oper_location" json:"operLocation" excel:"name:操作地点"`
	OperParam     string     `gorm:"column:oper_param" json:"operParam" excel:"name:请求参数"`
	JSONResult    string     `gorm:"column:json_result" json:"jsonResult" excel:"name:返回参数"`
	Status        int        `gorm:"column:status" json:"status" excel:"name:状态;converter:0=正常,1=异常"`
	ErrorMsg      string     `gorm:"column:error_msg" json:"errorMsg" excel:"name:错误消息"`
	OperTime      types.Time `gorm:"column:oper_time" json:"operTime" excel:"name:操作时间;width:30"`
	CostTime      int64      `gorm:"column:cost_time" json:"costTime" excel:"name:消耗时间;suffix:毫秒"`
}

func (SysOperLog) TableName() string { return "sys_oper_log" }

// LogininforQuery 登录日志的查询条件。
type LogininforQuery struct {
	IPAddr    string `form:"ipaddr"`
	UserName  string `form:"userName"`
	Status    string `form:"status"`
	BeginTime string `form:"params[beginTime]"`
	EndTime   string `form:"params[endTime]"`
}

// OperLogQuery 操作日志的查询条件。
type OperLogQuery struct {
	Title        string `form:"title"`
	OperName     string `form:"operName"`
	BusinessType string `form:"businessType"`
	Status       string `form:"status"`
	BeginTime    string `form:"params[beginTime]"`
	EndTime      string `form:"params[endTime]"`
}

// LogininforSortColumns 登录日志允许排序的字段白名单。
var LogininforSortColumns = map[string]string{
	"infoId":    "info_id",
	"userName":  "user_name",
	"status":    "status",
	"loginTime": "login_time",
}

// OperLogSortColumns 操作日志允许排序的字段白名单。
var OperLogSortColumns = map[string]string{
	"operId":       "oper_id",
	"title":        "title",
	"businessType": "business_type",
	"operName":     "oper_name",
	"status":       "status",
	"operTime":     "oper_time",
	"costTime":     "cost_time",
}
