package handler

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/jwtx"
	"ruoyi-go/pkg/response"
)

// Captcha GET /captchaImage
//
// 平铺字段：captchaEnabled、uuid、img（均不在 data 里）。
// 关闭验证码时只返回 captchaEnabled，不返回 uuid 和 img。
func Captcha(c *gin.Context) {
	ctx := c.Request.Context()

	enabled, err := service.CaptchaEnabled(ctx)
	if err != nil {
		fail(c, err)
		return
	}

	result := response.New(response.CodeSuccess, response.MsgSuccess).
		Put("captchaEnabled", enabled)
	if !enabled {
		result.JSON(c)
		return
	}

	id, img, err := service.GenerateCaptcha(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	// img 是不带 data URI 前缀的裸 base64，前端自己拼前缀
	result.Put("uuid", id).Put("img", img).JSON(c)
}

// Login POST /login
//
// 平铺字段：token。
func Login(c *gin.Context) {
	var body model.LoginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "用户名或密码不能为空")
		return
	}

	token, err := service.Login(c.Request.Context(), body, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		fail(c, err)
		return
	}
	response.New(response.CodeSuccess, response.MsgSuccess).Put("token", token).JSON(c)
}

// Register POST /register
//
// 是否开放注册由 sys_config 的 sys.account.registerUser 控制，默认关闭。
func Register(c *gin.Context) {
	var body model.RegisterBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, "注册信息不完整")
		return
	}

	err := service.Register(c.Request.Context(), body, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Ok(c)
}

// Logout POST /logout
//
// 无论令牌是否有效都返回成功 —— 前端点退出时会话可能已经过期，
// 这时候报错只会让用户卡在登录页。
func Logout(c *gin.Context) {
	token := jwtx.StripPrefix(c.GetHeader("Authorization"))
	if token != "" {
		if claims, err := service.ParseToken(token); err == nil {
			if err := service.Logout(c.Request.Context(), claims.LoginUserKey); err != nil {
				slog.Warn("退出登录时删除会话失败", "err", err)
			}
		}
	}
	response.OkMsg(c, "退出成功")
}

// GetInfo GET /getInfo
//
// 平铺字段：user、roles、permissions、pwdChrtype、
// isDefaultModifyPwd、isPasswordExpired（全部不在 data 里）。
func GetInfo(c *gin.Context) {
	ctx := c.Request.Context()
	loginUser := middleware.CurrentUser(c)
	user := loginUser.User

	roles := service.GetRolePermission(user)
	permissions, err := service.GetMenuPermission(ctx, user)
	if err != nil {
		fail(c, err)
		return
	}

	// 权限有变化时刷新会话，让改权限后立即生效（对齐 Java 版行为）
	if !sameStrings(permissions, loginUser.Permissions) {
		if err := service.RefreshLoginUserPermissions(ctx, loginUser); err != nil {
			slog.Warn("刷新权限缓存失败", "userId", loginUser.UserID, "err", err)
		} else {
			user = loginUser.User
			roles = service.GetRolePermission(user)
			permissions = loginUser.Permissions
		}
	}

	chrtype, err := service.PasswordChrtype(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	isDefaultModifyPwd, err := service.IsDefaultModifyPwd(ctx, user.PwdUpdateDate)
	if err != nil {
		fail(c, err)
		return
	}
	isPasswordExpired, err := service.IsPasswordExpired(ctx, user.PwdUpdateDate)
	if err != nil {
		fail(c, err)
		return
	}

	response.New(response.CodeSuccess, response.MsgSuccess).
		Put("user", contractLoginUser(*user)).
		Put("roles", roles).
		Put("permissions", permissions).
		Put("pwdChrtype", chrtype).
		Put("isDefaultModifyPwd", isDefaultModifyPwd).
		Put("isPasswordExpired", isPasswordExpired).
		JSON(c)
}

// GetRouters GET /getRouters
//
// 这个接口的路由树在 data 里，不是平铺 —— 和 deptTree 那类不一样，
// 不要想当然。
func GetRouters(c *gin.Context) {
	ctx := c.Request.Context()
	loginUser := middleware.CurrentUser(c)

	menus, err := service.SelectMenuTreeByUserID(ctx, loginUser.UserID)
	if err != nil {
		fail(c, err)
		return
	}
	response.OkData(c, service.BuildRouters(menus))
}

// sameStrings 比较两个已排序的字符串切片。
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
