// Package router 路由装配。[L5]
//
// 两样东西挂在这里，不写进 handler：
//   - 权限标识  middleware.HasPermission("system:user:list")
//   - 操作日志  middleware.OperLog("用户管理", model.BusinessTypeInsert)
//
// 操作日志只挂增删改授权这类写操作，查询不挂 —— 与 Java 的 @Log 用法一致，
// 给查询也记会让 sys_oper_log 迅速膨胀到没法用。
package router

import (
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/handler"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
)

// generatedRouteRegistrars 由代码生成器产出的同包文件在 init 中登记。
// 这样新增模块无需手工修改 registerAuthed，也不会引入反向依赖。
var generatedRouteRegistrars []func(*gin.RouterGroup)

// noRepeat 防重复提交，挂在**新增**路由上。
//
// 【为什么只挂新增，不挂修改】
// 重复新增会实打实多出一条数据，是真实伤害；重复修改是幂等的 ——
// 同样的参数改两次，结果和改一次一样。挂上去只是徒增误伤面。
//
// 【为什么不挂在整个 authed 组上】
// 导出走的是 POST，同样的查询条件连点两次导两份完全合法，
// 全局挂会把它拦掉，而且报的是"不允许重复提交"，用户根本想不到是被防重拦了。
func noRepeat() gin.HandlerFunc {
	return middleware.RepeatSubmit(middleware.DefaultRepeatInterval)
}

// New 构建路由。
func New(cfg *config.Config) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	// 空列表表示不信任任何代理头；只有显式配置的反向代理才能影响 ClientIP。
	_ = r.SetTrustedProxies(cfg.Server.TrustedProxies)
	// 限制 multipart 解析占用的内存，超出部分落临时文件。
	// 不设的话 gin 默认 32MB，配合导入这类要全量解析的接口很容易被打爆。
	r.MaxMultipartMemory = cfg.Upload.MaxSizeMB << 20

	middleware.InitCORS(cfg.Server.AllowedOrigins)
	middleware.InitLogger(cfg.Server.SlowRequestThreshold)
	middleware.InitExportLimit(service.MaxConcurrentExports)
	// Trace 必须排第一：排在 Recovery 后面的话，panic 那条日志就没有 traceId，
	// 而那恰恰是最需要能追溯的一条
	r.Use(
		middleware.Trace(),
		middleware.Recovery(),
		middleware.Logger(),
		middleware.CORS(),
		middleware.RequestTimeout(cfg.Server.RequestTimeout),
	)
	bodyLimit := middleware.RequestBodyLimit(cfg.Server.MaxRequestBodyMB << 20)
	multipartLimit := middleware.MultipartBodyLimit(
		cfg.Upload.MaxRequestSizeMB<<20, cfg.Upload.MaxSizeMB<<20)

	// 健康检查，不鉴权。真去 ping MySQL 和 Redis，依赖挂了返回 503
	r.GET("/health", handler.Health)

	// 上传文件的静态访问。前缀写死 /profile —— 前端拼头像地址时就是这个前缀。
	// 与 Java 版的 ResourcesConfig 一致，不鉴权。
	//
	// 【用 StaticFS + gin.Dir(.., false) 而不是 Static】
	// Static 底层是 http.FileServer，目录下没有 index.html 时会输出文件列表，
	// 访问 /profile/upload/2026/08/21/ 就能看到当天所有上传文件并逐个下载。
	handler.InitUpload(cfg.Upload)
	r.StaticFS(cfg.Upload.URLPrefix, gin.Dir(cfg.Upload.Path, false))

	registerAnonymous(r, bodyLimit, multipartLimit)
	registerAuthed(r, bodyLimit, multipartLimit)

	return r
}

// registerAnonymous 匿名可访问的路由。
//
// 与 Java 版 SecurityConfig 的放行清单保持一致。
// /logout 也放这里：会话过期后前端仍要能正常退出。
func registerAnonymous(r *gin.Engine, bodyLimit, multipartLimit gin.HandlerFunc) {
	// 【限流只挂匿名接口】它们是唯一不需要 token 就能打的入口，
	// 也是暴力破解的入口。登录还走 bcrypt，是全站最贵的一次请求。
	//
	// 参数写在这里而不是配置文件：想调就改这一行，比多五个配置项直观。
	// 已有的 pwd_err_cnt（5 次锁 10 分钟）只挡单个账号，
	// 挡不住拿一批账号轮着刷 —— 那是这里挡的。
	// 【阈值要按"一个 IP 后面有多少人"来定，不是按"一个人能点多快"】
	// 这类后台系统基本都装在内网，整个公司共用一个出口 IP。
	// 早上九点几十号人同时登录是常态 —— 阈值定成 20/分钟，那会儿就集体被拦，
	// 而且报的是"访问过于频繁"，运维根本想不到是自己人把自己人挤掉了。
	//
	// 暴力破解要试几千次，60/分钟拦得住；再叠加 pwd_err_cnt
	// （单个账号错 5 次锁 10 分钟），两层加起来够用了。
	r.GET("/captchaImage",
		middleware.RateLimit(middleware.RateLimitOptions{
			// 必须比登录宽：每次登录前都要先取一次验证码，还会手动刷新
			Key: "captcha", Count: 120, Window: time.Minute, ByIP: true,
		}),
		handler.Captcha)

	r.POST("/login",
		middleware.RateLimit(middleware.RateLimitOptions{
			Key: "login", Count: 60, Window: time.Minute, ByIP: true,
		}),
		bodyLimit, multipartLimit,
		handler.Login)

	r.POST("/register",
		middleware.RateLimit(middleware.RateLimitOptions{
			Key: "register", Count: 5, Window: time.Minute, ByIP: true,
		}),
		bodyLimit, multipartLimit,
		handler.Register)

	r.POST("/logout", handler.Logout)
}

// registerAuthed 需要登录的路由。
func registerAuthed(r *gin.Engine, bodyLimit, multipartLimit gin.HandlerFunc) {
	authed := r.Group("", middleware.Auth(), bodyLimit, multipartLimit)

	authed.GET("/getInfo", handler.GetInfo)
	authed.GET("/getRouters", handler.GetRouters)
	// 锁屏解锁：Java 版在 SysIndexController 里，不挂权限标识
	authed.POST("/unlockscreen", handler.UnlockScreen)

	registerPost(authed)
	registerDept(authed)
	registerRole(authed)
	registerUser(authed)
	registerMenu(authed)
	registerConfig(authed)
	registerDict(authed)
	registerNotice(authed)
	registerLog(authed)
	registerCommonFile(authed)
	registerMonitor(authed)
	registerJob(authed)
	registerGen(authed)
	for _, register := range generatedRouteRegistrars {
		register(authed)
	}
}

// registerGen 代码生成器。静态路径必须先于 /:tableId 注册。
func registerGen(g *gin.RouterGroup) {
	const title = "代码生成"
	gen := g.Group("/tool/gen")
	gen.GET("/list", middleware.HasPermission("tool:gen:list"), handler.GenList)
	gen.GET("/db/list", middleware.HasPermission("tool:gen:list"), handler.GenDBList)
	gen.GET("/column/:tableId", middleware.HasPermission("tool:gen:list"), handler.GenColumnList)
	gen.POST("/importTable", noRepeat(), middleware.HasPermission("tool:gen:import"),
		middleware.OperLog(title, model.BusinessTypeImport), handler.GenImport)
	gen.POST("/createTable", noRepeat(), middleware.HasRole(model.AdminRoleKey),
		middleware.OperLog("创建表", model.BusinessTypeOther), handler.GenCreateTable)
	gen.GET("/preview/:tableId", middleware.HasPermission("tool:gen:preview"), handler.GenPreview)
	gen.GET("/download/:tableName", middleware.HasPermission("tool:gen:code"),
		middleware.OperLog(title, model.BusinessTypeGenCode), handler.GenDownload)
	gen.GET("/genCode/:tableName", middleware.HasPermission("tool:gen:code"),
		middleware.OperLog(title, model.BusinessTypeGenCode), handler.GenWriteCode)
	gen.GET("/synchDb/:tableName", middleware.HasPermission("tool:gen:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.GenSync)
	gen.GET("/batchGenCode", middleware.HasPermission("tool:gen:code"),
		middleware.OperLog(title, model.BusinessTypeGenCode), handler.GenBatchDownload)
	gen.GET("/:tableId", middleware.HasPermission("tool:gen:query"), handler.GenGet)
	gen.PUT("", middleware.HasPermission("tool:gen:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.GenEdit)
	gen.DELETE("/:tableIds", middleware.HasPermission("tool:gen:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.GenRemove)
}

// registerJob 定时任务与调度日志。
//
// 静态路由必须排在参数路由之前（clean 不能被 /:jobLogIds 抢走）。
func registerJob(g *gin.RouterGroup) {
	const title = "定时任务"
	job := g.Group("/monitor/job")
	job.GET("/list", middleware.HasPermission("monitor:job:list"), handler.JobList)
	job.POST("/export", middleware.ExportLimit(), middleware.HasPermission("monitor:job:export"),
		middleware.OperLog(title, model.BusinessTypeExport), handler.JobExport)
	job.PUT("/changeStatus", middleware.HasPermission("monitor:job:changeStatus"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.JobChangeStatus)
	// run 与 changeStatus 共用权限标识，与 Java 版一致
	job.PUT("/run", middleware.HasPermission("monitor:job:changeStatus"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.JobRun)
	job.GET("/:jobId", middleware.HasPermission("monitor:job:query"), handler.JobGet)
	job.POST("", noRepeat(), middleware.HasPermission("monitor:job:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.JobAdd)
	job.PUT("", middleware.HasPermission("monitor:job:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.JobEdit)
	job.DELETE("/:jobIds", middleware.HasPermission("monitor:job:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.JobRemove)

	const logTitle = "调度日志"
	jobLog := g.Group("/monitor/jobLog")
	jobLog.GET("/list", middleware.HasPermission("monitor:job:list"), handler.JobLogList)
	jobLog.POST("/export", middleware.ExportLimit(), middleware.HasPermission("monitor:job:export"),
		middleware.OperLog(logTitle, model.BusinessTypeExport), handler.JobLogExport)
	jobLog.DELETE("/clean", middleware.HasPermission("monitor:job:remove"),
		middleware.OperLog(logTitle, model.BusinessTypeClean), handler.JobLogClean)
	jobLog.GET("/:jobLogId", middleware.HasPermission("monitor:job:query"), handler.JobLogGet)
	jobLog.DELETE("/:jobLogIds", middleware.HasPermission("monitor:job:remove"),
		middleware.OperLog(logTitle, model.BusinessTypeDelete), handler.JobLogRemove)
}

// registerMonitor 系统监控：在线用户、服务监控、缓存监控。
//
// 静态路由必须排在参数路由之前（getNames / clearCacheAll 等）。
func registerMonitor(g *gin.RouterGroup) {
	online := g.Group("/monitor/online")
	online.GET("/list", middleware.HasPermission("monitor:online:list"), handler.OnlineList)
	online.DELETE("/:tokenId", middleware.HasPermission("monitor:online:forceLogout"),
		middleware.OperLog("在线用户", model.BusinessTypeForce), handler.OnlineForceLogout)

	g.GET("/monitor/server", middleware.HasPermission("monitor:server:list"), handler.ServerInfo)

	cache := g.Group("/monitor/cache")
	cache.GET("", middleware.HasPermission("monitor:cache:list"), handler.CacheInfo)
	cache.GET("/getNames", middleware.HasPermission("monitor:cache:list"), handler.CacheNames)
	cache.DELETE("/clearCacheAll", middleware.HasPermission("monitor:cache:list"),
		middleware.OperLog("缓存监控", model.BusinessTypeClean), handler.CacheClearAll)
	cache.GET("/getKeys/:cacheName", middleware.HasPermission("monitor:cache:list"), handler.CacheKeys)
	cache.GET("/getValue/:cacheName/:cacheKey", middleware.HasPermission("monitor:cache:list"), handler.CacheValue)
	cache.DELETE("/clearCacheName/:cacheName", middleware.HasPermission("monitor:cache:list"),
		middleware.OperLog("缓存监控", model.BusinessTypeClean), handler.CacheClearName)
	cache.DELETE("/clearCacheKey/:cacheKey", middleware.HasPermission("monitor:cache:list"),
		middleware.OperLog("缓存监控", model.BusinessTypeClean), handler.CacheClearKey)
}

// registerCommonFile 通用文件上传与下载。
//
// 富文本编辑器插图、FileUpload / ImageUpload 组件都走这里，
// 与 Java 版一致不挂权限标识（登录后即可用）。
//
// 下载两条**要登录**：Java 的 SecurityConfig 放行清单里没有 /common/download，
// 不要因为"下载而已"就挪到匿名组。
func registerCommonFile(g *gin.RouterGroup) {
	g.POST("/common/upload", handler.CommonUpload)
	g.POST("/common/uploads", handler.CommonUploads)
	// 静态路由排在前面：/download/resource 不能被 /download 抢走
	g.GET("/common/download/resource", handler.CommonDownloadResource)
	g.GET("/common/download", handler.CommonDownload)
}

// registerPost 岗位管理。
//
// 【注意】静态路由必须先于参数路由注册：/optionselect 要排在 /:postId 前面。
// optionselect 与 Java 版一致不校验权限（用户管理页面的下拉框要用）。
func registerPost(g *gin.RouterGroup) {
	const title = "岗位管理"
	post := g.Group("/system/post")
	post.GET("/optionselect", handler.PostOptionSelect)
	post.GET("/list", middleware.HasPermission("system:post:list"), handler.PostList)
	post.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:post:export"),
		middleware.OperLog(title, model.BusinessTypeExport), handler.PostExport)
	post.GET("/:postId", middleware.HasPermission("system:post:query"), handler.PostGet)
	post.POST("", noRepeat(), middleware.HasPermission("system:post:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.PostAdd)
	post.PUT("", middleware.HasPermission("system:post:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.PostEdit)
	post.DELETE("/:postIds", middleware.HasPermission("system:post:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.PostRemove)
}

// registerDept 部门管理。
//
// 静态路由 /list、/list/exclude/:deptId、/updateSort 必须排在 /:deptId 之前。
func registerDept(g *gin.RouterGroup) {
	const title = "部门管理"
	dept := g.Group("/system/dept")
	dept.GET("/list", middleware.HasPermission("system:dept:list"), handler.DeptList)
	dept.GET("/list/exclude/:deptId", middleware.HasPermission("system:dept:list"), handler.DeptExcludeChild)
	dept.PUT("/updateSort", middleware.HasPermission("system:dept:edit"),
		middleware.OperLog("保存部门排序", model.BusinessTypeUpdate), handler.DeptUpdateSort)
	dept.GET("/:deptId", middleware.HasPermission("system:dept:query"), handler.DeptGet)
	dept.POST("", noRepeat(), middleware.HasPermission("system:dept:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.DeptAdd)
	dept.PUT("", middleware.HasPermission("system:dept:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.DeptEdit)
	dept.DELETE("/:deptId", middleware.HasPermission("system:dept:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.DeptRemove)
}

// registerRole 角色管理。
func registerRole(g *gin.RouterGroup) {
	const title = "角色管理"
	role := g.Group("/system/role")
	role.GET("/list", middleware.HasPermission("system:role:list"), handler.RoleList)
	role.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:role:export"),
		middleware.OperLog(title, model.BusinessTypeExport), handler.RoleExport)
	role.GET("/optionselect", middleware.HasPermission("system:role:query"), handler.RoleOptionSelect)
	role.GET("/deptTree/:roleId", middleware.HasPermission("system:role:query"), handler.RoleDeptTree)
	role.GET("/authUser/allocatedList", middleware.HasPermission("system:role:list"), handler.RoleAuthUserAllocated)
	role.GET("/authUser/unallocatedList", middleware.HasPermission("system:role:list"), handler.RoleAuthUserUnallocated)
	role.PUT("/authUser/cancel", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeGrant), handler.RoleAuthUserCancel)
	role.PUT("/authUser/cancelAll", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeGrant), handler.RoleAuthUserCancelAll)
	role.PUT("/authUser/selectAll", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeGrant), handler.RoleAuthUserSelectAll)
	role.PUT("/dataScope", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.RoleDataScope)
	role.PUT("/changeStatus", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.RoleChangeStatus)
	role.GET("/:roleId", middleware.HasPermission("system:role:query"), handler.RoleGet)
	role.POST("", noRepeat(), middleware.HasPermission("system:role:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.RoleAdd)
	role.PUT("", middleware.HasPermission("system:role:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.RoleEdit)
	role.DELETE("/:roleIds", middleware.HasPermission("system:role:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.RoleRemove)
}

// registerUser 用户管理。
//
// 静态路由必须排在 /:userId 之前。
// 新增用户弹窗调的是 GET /system/user/（带尾斜杠、无 ID），
// Java 同时映射 "/" 和 "/{userId}"。Go 同时注册空路径和 "/"，
// 避免依赖客户端跟随 Gin 的尾斜杠重定向。
func registerUser(g *gin.RouterGroup) {
	const title = "用户管理"
	user := g.Group("/system/user")
	user.GET("/list", middleware.HasPermission("system:user:list"), handler.UserList)
	user.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:user:export"),
		middleware.OperLog(title, model.BusinessTypeExport), handler.UserExport)
	user.POST("/importData", middleware.HasPermission("system:user:import"),
		middleware.OperLog(title, model.BusinessTypeImport), handler.UserImportData)
	// 【模板下载不挂权限】Java 的 importTemplate 方法上没有 @PreAuthorize。
	// 模板是一张只有表头的空表，不含任何数据，没有保护的必要；
	// 真正需要权限的是 importData。
	user.POST("/importTemplate", handler.UserImportTemplate)
	user.GET("/deptTree", middleware.HasPermission("system:user:list"), handler.UserDeptTree)
	// 个人中心：只操作自己，不挂权限标识（与 Java 一致）
	user.GET("/profile", handler.ProfileGet)
	user.PUT("/profile", middleware.OperLog("个人信息", model.BusinessTypeUpdate), handler.ProfileUpdate)
	user.PUT("/profile/updatePwd", middleware.OperLog("个人信息", model.BusinessTypeUpdate), handler.ProfileUpdatePwd)
	user.POST("/profile/avatar", middleware.OperLog("用户头像", model.BusinessTypeUpdate), handler.ProfileAvatar)
	user.PUT("/resetPwd", middleware.HasPermission("system:user:resetPwd"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.UserResetPwd)
	user.PUT("/changeStatus", middleware.HasPermission("system:user:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.UserChangeStatus)
	user.GET("/authRole/:userId", middleware.HasPermission("system:user:query"), handler.UserAuthRoleGet)
	user.PUT("/authRole", middleware.HasPermission("system:user:edit"),
		middleware.OperLog(title, model.BusinessTypeGrant), handler.UserAuthRoleSave)
	// 新增弹窗：无 userId
	user.GET("", middleware.HasPermission("system:user:query"), handler.UserGet)
	user.GET("/", middleware.HasPermission("system:user:query"), handler.UserGet)
	user.GET("/:userId", middleware.HasPermission("system:user:query"), handler.UserGet)
	user.POST("", noRepeat(), middleware.HasPermission("system:user:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.UserAdd)
	user.PUT("", middleware.HasPermission("system:user:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.UserEdit)
	user.DELETE("/:userIds", middleware.HasPermission("system:user:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.UserRemove)
}

// registerMenu 菜单管理。
func registerMenu(g *gin.RouterGroup) {
	const title = "菜单管理"
	menu := g.Group("/system/menu")
	menu.GET("/list", middleware.HasPermission("system:menu:list"), handler.MenuList)
	// treeselect 与 Java 一致不挂权限：新增角色时要用，而那时用户未必有菜单管理权限
	menu.GET("/treeselect", handler.MenuTreeSelect)
	menu.GET("/roleMenuTreeselect/:roleId", handler.MenuRoleTreeSelect)
	menu.PUT("/updateSort", middleware.HasPermission("system:menu:edit"),
		middleware.OperLog("保存菜单排序", model.BusinessTypeUpdate), handler.MenuUpdateSort)
	menu.GET("/:menuId", middleware.HasPermission("system:menu:query"), handler.MenuGet)
	menu.POST("", noRepeat(), middleware.HasPermission("system:menu:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.MenuAdd)
	menu.PUT("", middleware.HasPermission("system:menu:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.MenuEdit)
	menu.DELETE("/:menuId", middleware.HasPermission("system:menu:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.MenuRemove)
}

// registerConfig 参数配置。
func registerConfig(g *gin.RouterGroup) {
	const title = "参数管理"
	cfg := g.Group("/system/config")
	cfg.GET("/list", middleware.HasPermission("system:config:list"), handler.ConfigList)
	cfg.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:config:export"),
		middleware.OperLog(title, model.BusinessTypeExport), handler.ConfigExport)
	// 参数值可能包含初始密码等敏感信息，按键名和调用方权限做最小授权。
	cfg.GET("/configKey/:configKey", middleware.CanReadConfigKey(), handler.ConfigGetByKey)
	cfg.DELETE("/refreshCache", middleware.HasPermission("system:config:remove"),
		middleware.OperLog(title, model.BusinessTypeClean), handler.ConfigRefreshCache)
	cfg.GET("/:configId", middleware.HasPermission("system:config:query"), handler.ConfigGet)
	cfg.POST("", noRepeat(), middleware.HasPermission("system:config:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.ConfigAdd)
	cfg.PUT("", middleware.HasPermission("system:config:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.ConfigEdit)
	cfg.DELETE("/:configIds", middleware.HasPermission("system:config:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.ConfigRemove)
}

// registerDict 字典管理（类型 + 数据）。
//
// 路由层级注意：/system/dict/data/... 和 /system/dict/type/... 是两组，
// data 下的 /type/:dictType 与 type 分组无关，别搞混。
func registerDict(g *gin.RouterGroup) {
	const (
		dataTitle = "字典数据"
		typeTitle = "字典类型"
	)

	data := g.Group("/system/dict/data")
	// 不挂权限：每个页面渲染字典标签都要调
	data.GET("/type/:dictType", handler.DictDataByType)
	data.GET("/list", middleware.HasPermission("system:dict:list"), handler.DictDataList)
	data.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:dict:export"),
		middleware.OperLog(dataTitle, model.BusinessTypeExport), handler.DictDataExport)
	data.GET("/:dictCode", middleware.HasPermission("system:dict:query"), handler.DictDataGet)
	data.POST("", noRepeat(), middleware.HasPermission("system:dict:add"),
		middleware.OperLog(dataTitle, model.BusinessTypeInsert), handler.DictDataAdd)
	data.PUT("", middleware.HasPermission("system:dict:edit"),
		middleware.OperLog(dataTitle, model.BusinessTypeUpdate), handler.DictDataEdit)
	data.DELETE("/:dictCodes", middleware.HasPermission("system:dict:remove"),
		middleware.OperLog(dataTitle, model.BusinessTypeDelete), handler.DictDataRemove)

	dictType := g.Group("/system/dict/type")
	dictType.GET("/list", middleware.HasPermission("system:dict:list"), handler.DictTypeList)
	dictType.POST("/export", middleware.ExportLimit(), middleware.HasPermission("system:dict:export"),
		middleware.OperLog(typeTitle, model.BusinessTypeExport), handler.DictTypeExport)
	dictType.GET("/optionselect", handler.DictTypeOptionSelect)
	dictType.DELETE("/refreshCache", middleware.HasPermission("system:dict:remove"),
		middleware.OperLog(typeTitle, model.BusinessTypeClean), handler.DictTypeRefreshCache)
	dictType.GET("/:dictId", middleware.HasPermission("system:dict:query"), handler.DictTypeGet)
	dictType.POST("", noRepeat(), middleware.HasPermission("system:dict:add"),
		middleware.OperLog(typeTitle, model.BusinessTypeInsert), handler.DictTypeAdd)
	dictType.PUT("", middleware.HasPermission("system:dict:edit"),
		middleware.OperLog(typeTitle, model.BusinessTypeUpdate), handler.DictTypeEdit)
	dictType.DELETE("/:dictIds", middleware.HasPermission("system:dict:remove"),
		middleware.OperLog(typeTitle, model.BusinessTypeDelete), handler.DictTypeRemove)
}

// registerNotice 通知公告。
//
// 权限严格照 SysNoticeController 的注解来，**逐个方法核对过**：
//
//	listTop / markRead / markReadAll / :noticeId  →  无 @PreAuthorize，仅需登录
//	readUsers/list                                →  @PreAuthorize('system:notice:list')
//
// 前四个是顶栏公告铃铛用的，所有登录用户都要能调
// （HeaderNotice/DetailView.vue 调的就是 getNotice(id)，
// 详情接口曾经错挂过 system:notice:query，导致普通员工点开就 403）。
//
// 【readUsers/list 必须挂权限】它返回的是登录名、昵称、部门、手机号 ——
// 谁读了哪条公告属于管理信息。这里曾经错误地跟着上面几个一起不挂，
// 注释还写着"与 Java 一致"，实际 Java 那行有 @PreAuthorize。
// **注释不是证据，要去看源码。**
//
// 标记已读不记操作日志 —— 每个用户每次进页面都会触发。
func registerNotice(g *gin.RouterGroup) {
	const title = "通知公告"
	notice := g.Group("/system/notice")
	notice.GET("/listTop", handler.NoticeListTop)
	notice.POST("/markRead", handler.NoticeMarkRead)
	notice.POST("/markReadAll", handler.NoticeMarkReadAll)
	notice.GET("/readUsers/list", middleware.HasPermission("system:notice:list"), handler.NoticeReadUsers)
	notice.GET("/list", middleware.HasPermission("system:notice:list"), handler.NoticeList)
	notice.GET("/:noticeId", handler.NoticeGet)
	notice.POST("", noRepeat(), middleware.HasPermission("system:notice:add"),
		middleware.OperLog(title, model.BusinessTypeInsert), handler.NoticeAdd)
	notice.PUT("", middleware.HasPermission("system:notice:edit"),
		middleware.OperLog(title, model.BusinessTypeUpdate), handler.NoticeEdit)
	notice.DELETE("/:noticeIds", middleware.HasPermission("system:notice:remove"),
		middleware.OperLog(title, model.BusinessTypeDelete), handler.NoticeRemove)
}

// registerLog 日志管理。
//
// 路径在 /monitor 下但菜单挂在系统管理里，权限标识也是 monitor:*，
// 这是 RuoYi 的既定安排，不要"纠正"成 system:*。
//
// clean 必须注册在 /:xxxIds 之前，否则会被当成 ID 匹配。
func registerLog(g *gin.RouterGroup) {
	logininfor := g.Group("/monitor/logininfor")
	logininfor.GET("/list", middleware.HasPermission("monitor:logininfor:list"), handler.LogininforList)
	logininfor.POST("/export", middleware.ExportLimit(), middleware.HasPermission("monitor:logininfor:export"),
		middleware.OperLog("登录日志", model.BusinessTypeExport), handler.LogininforExport)
	logininfor.DELETE("/clean", middleware.HasPermission("monitor:logininfor:remove"),
		middleware.OperLog("登录日志", model.BusinessTypeClean), handler.LogininforClean)
	logininfor.GET("/unlock/:userName", middleware.HasPermission("monitor:logininfor:unlock"),
		middleware.OperLog("账户解锁", model.BusinessTypeOther), handler.LogininforUnlock)
	logininfor.DELETE("/:infoIds", middleware.HasPermission("monitor:logininfor:remove"),
		middleware.OperLog("登录日志", model.BusinessTypeDelete), handler.LogininforRemove)

	operlog := g.Group("/monitor/operlog")
	operlog.GET("/list", middleware.HasPermission("monitor:operlog:list"), handler.OperLogList)
	operlog.POST("/export", middleware.ExportLimit(), middleware.HasPermission("monitor:operlog:export"),
		middleware.OperLog("操作日志", model.BusinessTypeExport), handler.OperLogExport)
	operlog.DELETE("/clean", middleware.HasPermission("monitor:operlog:remove"),
		middleware.OperLog("操作日志", model.BusinessTypeClean), handler.OperLogClean)
	operlog.DELETE("/:operIds", middleware.HasPermission("monitor:operlog:remove"),
		middleware.OperLog("操作日志", model.BusinessTypeDelete), handler.OperLogRemove)
}
