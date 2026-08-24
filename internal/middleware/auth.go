package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/jwtx"
	"ruoyi-go/pkg/response"
)

// ctxKeyLoginUser gin.Context 中存放会话的键。
//
// 这个键和下面的存取函数由 middleware 包持有，handler 包可以引用
// （同层依赖，方向固定为 handler -> middleware，不得反向）。
const ctxKeyLoginUser = "ruoyi:loginUser"

// Auth JWT 鉴权中间件。
//
// 流程：取 Authorization 头 -> 校验签名拿 uuid -> 用 uuid 去 Redis 取会话。
// 签名有效但 Redis 里没有，说明会话已过期或被强制下线，同样按未登录处理。
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := jwtx.StripPrefix(c.GetHeader("Authorization"))
		if token == "" {
			unauthorized(c)
			return
		}

		ctx := c.Request.Context()
		loginUser, err := service.GetLoginUser(ctx, token)
		if err != nil {
			slog.Debug("解析令牌失败", "path", c.Request.URL.Path, "err", err)
			unauthorized(c)
			return
		}
		if loginUser == nil {
			unauthorized(c)
			return
		}

		// 续期失败不影响本次请求，最坏情况是会话按原有 TTL 过期
		if err := service.VerifyToken(ctx, loginUser); err != nil {
			slog.Warn("刷新会话失败", "userId", loginUser.UserID, "err", err)
		}

		c.Set(ctxKeyLoginUser, loginUser)
		c.Next()
	}
}

// HasPermission 校验权限标识，配合 Auth 使用。
//
// 权限标识挂在路由上，不要写进 handler：
//
//	g.GET("/list", middleware.HasPermission("system:user:list"), handler.UserList)
func HasPermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		loginUser := CurrentUser(c)
		if loginUser == nil {
			unauthorized(c)
			return
		}
		if !loginUser.HasPermission(perm) {
			response.FailCode(c, response.CodeForbidden, "没有权限，请联系管理员授权")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CurrentUser 取当前会话，未登录返回 nil。
func CurrentUser(c *gin.Context) *model.LoginUser {
	value, ok := c.Get(ctxKeyLoginUser)
	if !ok {
		return nil
	}
	loginUser, ok := value.(*model.LoginUser)
	if !ok {
		return nil
	}
	return loginUser
}

func unauthorized(c *gin.Context) {
	response.FailCode(c, response.CodeUnauthorized, "认证失败，无法访问系统资源")
	c.Abort()
}
