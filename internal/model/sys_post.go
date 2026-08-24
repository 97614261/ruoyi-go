package model

import "ruoyi-go/pkg/types"

// SysPost 岗位表 sys_post。
//
// excel tag 与 Java 版 @Excel 一一对应，导出列顺序 = 字段声明顺序（同 Java）。
type SysPost struct {
	PostID int64 `gorm:"column:post_id;primaryKey;autoIncrement" json:"postId" excel:"name:岗位序号;cell:numeric"`
	// notblank 而非 required：对齐 Java 的 @NotBlank，纯空白串也要拒
	PostCode string `gorm:"column:post_code" json:"postCode" binding:"notblank,max=64" excel:"name:岗位编码"`
	PostName string `gorm:"column:post_name" json:"postName" binding:"notblank,max=50" excel:"name:岗位名称"`
	// PostSort 用**指针**，这是唯一能同时表达"必填"和"0 合法"的写法。
	//
	// 非指针 int + min=0 的话，不传和传 0 在 Go 里都是 0，分不开 ——
	// 于是"不传"会被当成 0 放行，而 Java 的 Integer + @NotNull 是拒绝的。
	// 那属于**比 Java 更宽**，会放进 Java 版拒绝的脏数据，是被禁止的方向。
	//
	// 指针 + required：nil（不传）拒绝，指向 0 的指针通过。
	// min=0 保留，负数排序无业务含义，前端表单本就是 :min="0"（这一条比 Java 严，已登记）。
	//
	// excel 侧刻意不标 cell:numeric —— Java 的 @Excel(name="岗位排序") 没有 cellType，
	// 默认按字符串导出，这里保持一致。
	PostSort *int `gorm:"column:post_sort" json:"postSort" binding:"required,min=0" excel:"name:岗位排序"`
	// Status 取值来自字典 sys_normal_disable，**不要**写成 oneof=0 1 ——
	// 字典是可以在后台编辑的，加了新值前端就能提交，硬编码会把合法提交拒掉。
	Status     string     `gorm:"column:status" json:"status" binding:"required" excel:"name:状态;converter:0=正常,1=停用"`
	CreateBy   string     `gorm:"column:create_by" json:"createBy"`
	CreateTime types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy   string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime types.Time `gorm:"column:update_time" json:"updateTime"`
	// 可选字段：omitempty 挡 nil 指针，max 对齐 varchar(500) 列长
	Remark *string `gorm:"column:remark" json:"remark" binding:"omitempty,max=500"`

	// Flag 非表字段，用户分配岗位时标记是否选中
	Flag bool `gorm:"-" json:"flag"`
}

func (SysPost) TableName() string { return "sys_post" }

// PostQuery 岗位列表的查询条件。
//
// form 标签同时服务两处：列表接口从 query string 绑定，
// 导出接口从 application/x-www-form-urlencoded 的请求体绑定
// （RuoYi 前端的 download() 是 POST + 表单编码，不是 query string）。
type PostQuery struct {
	PostCode string `form:"postCode"`
	PostName string `form:"postName"`
	Status   string `form:"status"`
}

// PostSortColumns 允许排序的字段白名单，key 为前端字段名，value 为数据库列名。
var PostSortColumns = map[string]string{
	"postId":     "post_id",
	"postCode":   "post_code",
	"postName":   "post_name",
	"postSort":   "post_sort",
	"status":     "status",
	"createTime": "create_time",
}
