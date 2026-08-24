package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/response"
	"ruoyi-go/pkg/types"
	"ruoyi-go/pkg/upload"
)

// uploadCfg 由 router 在装配时注入。
var uploadCfg config.UploadConfig

// InitUpload 设置上传配置，必须在注册路由前调用。
func InitUpload(cfg config.UploadConfig) { uploadCfg = cfg }

// ProfileGet GET /system/user/profile
//
// 【混合形态】data 里是用户对象，同时平铺 roleGroup / postGroup。
func ProfileGet(c *gin.Context) {
	loginUser := middleware.CurrentUser(c)

	user, roleGroup, postGroup, err := service.GetProfile(c.Request.Context(), loginUser.UserID)
	if err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("data", user).
		Put("roleGroup", roleGroup).
		Put("postGroup", postGroup).
		JSON(c)
}

// ProfileUpdate PUT /system/user/profile
func ProfileUpdate(c *gin.Context) {
	var body model.ProfileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	loginUser := middleware.CurrentUser(c)

	if err := service.UpdateProfile(c.Request.Context(), loginUser.UserID, body); err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// ProfileUpdatePwd PUT /system/user/profile/updatePwd
//
// 【参数在 JSON body 里】Vue3 的 updateUserPwd 用的是 `data: data`，
// Java 是 `@RequestBody Map<String, String>`。
// 之前写成读查询串，改密码在真实前端上从来没成功过 —— 见 model.UpdatePwdBody 的注释。
func ProfileUpdatePwd(c *gin.Context) {
	var body model.UpdatePwdBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	loginUser := middleware.CurrentUser(c)

	err := service.UpdateProfilePwd(c.Request.Context(), loginUser.UserID,
		body.OldPassword, body.NewPassword)
	if err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// UnlockScreen POST /unlockscreen
//
// 头像下拉菜单里的"锁定屏幕"，解锁时用当前账号的密码校验。
// 锁定状态本身存在前端（store/modules/lock.js），后端不落库。
func UnlockScreen(c *gin.Context) {
	var body model.UnlockBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, bindMessage(err))
		return
	}
	loginUser := middleware.CurrentUser(c)

	if err := service.UnlockScreen(c.Request.Context(), loginUser.UserID, body.Password); err != nil {
		fail(c, err)
		return
	}
	response.OkMsg(c, "解锁成功")
}

// ProfileAvatar POST /system/user/profile/avatar
//
// 【平铺字段】imgUrl 在顶层。表单字段名是 avatarfile（前端写死的）。
func ProfileAvatar(c *gin.Context) {
	fileHeader, err := c.FormFile("avatarfile")
	if err != nil {
		response.Fail(c, "请选择要上传的头像")
		return
	}
	loginUser := middleware.CurrentUser(c)

	url, err := upload.Save(fileHeader, upload.Options{
		BaseDir:    uploadCfg.Path,
		SubDir:     "avatar",
		URLPrefix:  uploadCfg.URLPrefix,
		MaxSize:    uploadCfg.MaxSizeMB * 1024 * 1024,
		AllowedExt: upload.ImageExtensions,
		DatePath:   time.Now().In(types.Location).Format("2006/01/02"),
	})
	if err != nil {
		response.Fail(c, err.Error())
		return
	}

	if err := service.UpdateAvatar(c.Request.Context(), loginUser.UserID, url); err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).Put("imgUrl", url).JSON(c)
}
