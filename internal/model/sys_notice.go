package model

import "ruoyi-go/pkg/types"

// SysNotice 通知公告表 sys_notice。
//
// 【注意】notice_content 在库里是 longblob。Go 侧必须建模成 string，
// 若用 []byte，encoding/json 会把它编码成 base64，前端拿到的是乱码。
type SysNotice struct {
	NoticeID    int64  `gorm:"column:notice_id;primaryKey;autoIncrement" json:"noticeId"`
	NoticeTitle string `gorm:"column:notice_title" json:"noticeTitle" binding:"notblank,xss,max=50"`
	// NoticeType 取值来自字典 sys_notice_type，不要写 oneof
	NoticeType    string     `gorm:"column:notice_type" json:"noticeType" binding:"required"`
	NoticeContent string     `gorm:"column:notice_content" json:"noticeContent"`
	Status        string     `gorm:"column:status" json:"status"`
	CreateBy      string     `gorm:"column:create_by" json:"createBy"`
	CreateTime    types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy      string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime    types.Time `gorm:"column:update_time" json:"updateTime"`
	// remark 列长是 255，不是其它表的 500
	Remark *string `gorm:"column:remark" json:"remark" binding:"omitempty,max=255"`
}

func (SysNotice) TableName() string { return "sys_notice" }

// SysNoticeRead 公告已读记录表 sys_notice_read。
type SysNoticeRead struct {
	ReadID   int64      `gorm:"column:read_id;primaryKey;autoIncrement" json:"readId"`
	NoticeID int64      `gorm:"column:notice_id" json:"noticeId"`
	UserID   int64      `gorm:"column:user_id" json:"userId"`
	ReadTime types.Time `gorm:"column:read_time" json:"readTime"`
}

func (SysNoticeRead) TableName() string { return "sys_notice_read" }

// NoticeTopItem 顶栏公告列表项。
//
// 刻意不含 notice_content：那是个 longblob，顶栏只显示标题，
// 每次进页面都把富文本正文捞出来纯属浪费。
type NoticeTopItem struct {
	NoticeID    int64      `gorm:"column:notice_id" json:"noticeId"`
	NoticeTitle string     `gorm:"column:notice_title" json:"noticeTitle"`
	NoticeType  string     `gorm:"column:notice_type" json:"noticeType"`
	Status      string     `gorm:"column:status" json:"status"`
	CreateBy    string     `gorm:"column:create_by" json:"createBy"`
	CreateTime  types.Time `gorm:"column:create_time" json:"createTime"`
	IsRead      bool       `gorm:"column:is_read" json:"isRead"`
}

// NoticeReadUser 公告已读用户列表项。
type NoticeReadUser struct {
	UserID   int64      `gorm:"column:user_id" json:"userId"`
	UserName string     `gorm:"column:user_name" json:"userName"`
	NickName string     `gorm:"column:nick_name" json:"nickName"`
	DeptName string     `gorm:"column:dept_name" json:"deptName"`
	ReadTime types.Time `gorm:"column:read_time" json:"readTime"`
}

// NoticeQuery 公告列表的查询条件。
type NoticeQuery struct {
	NoticeTitle string `form:"noticeTitle"`
	NoticeType  string `form:"noticeType"`
	CreateBy    string `form:"createBy"`
	Status      string `form:"status"`
}

// NoticeReadUserQuery 已读用户列表的查询条件。
type NoticeReadUserQuery struct {
	NoticeID int64 `form:"noticeId"`
	// SearchValue 前端发的就是这个名字（ReadUsers.vue 的 queryParams.searchValue），
	// Java 侧的方法签名也是 readUsersList(Long noticeId, String searchValue)。
	//
	// 【别改成 userName】曾经写成 userName，结果是搜索框输什么都返回全部 ——
	// 参数名对不上时 Go 只是拿到空值，不会报错，页面看起来"正常"只是没过滤。
	// 这类问题只能靠比对前端实际发的参数发现。
	SearchValue string `form:"searchValue"`
}

// NoticeSortColumns 允许排序的字段白名单。
var NoticeSortColumns = map[string]string{
	"noticeId":    "notice_id",
	"noticeTitle": "notice_title",
	"noticeType":  "notice_type",
	"status":      "status",
	"createTime":  "create_time",
}
