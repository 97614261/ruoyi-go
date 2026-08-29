package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/response"
)

// DefaultRepeatInterval 默认的重复提交判定间隔，与 Java 版 @RepeatSubmit 的默认值一致。
const DefaultRepeatInterval = 5 * time.Second

// repeatSubmitTimeout Redis 操作的超时。
const repeatSubmitTimeout = time.Second

// RepeatSubmit 防重复提交。
//
// 判定条件与 Java 版 SameUrlDataInterceptor 一致：
// **同一个人 + 同一个 URL + 同样的参数 + 间隔之内** 才算重复。
// 参数不同就放行 —— 连续新增两条不同的记录是正常操作。
//
// 挂在新增/修改这类路由上，**不要挂查询和导出**：
// 导出走的是 POST，同样的查询条件连点两次导两份是合法的。
//
// 【比 Java 多做的一件事：参数存哈希，不存原文】
// Java 把请求体原样塞进 Redis 的 repeatParams 字段。它自己没在任何接口上
// 用这个注解，所以没暴露问题；但只要挂到"新增用户""重置密码"上，
// 明文密码就进 Redis 了，而且带 TTL 期间一直躺在那。
// 这里只存 sha256 前 16 字节：判重只需要"一样不一样"，不需要还原内容。
//
// 【Redis 挂了怎么办：放行】理由同限流 —— 防护降级好过全站不可用。
func RepeatSubmit(interval time.Duration) gin.HandlerFunc {
	if interval <= 0 {
		interval = DefaultRepeatInterval
	}

	return func(c *gin.Context) {
		// 只管写操作。GET 幂等，DELETE 重复执行的结果也一致
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut {
			c.Next()
			return
		}

		fingerprint, ok := requestFingerprint(c)
		if !ok {
			c.Next()
			return
		}

		// 身份用 token；没有 token（匿名接口）就退回 IP
		identity := c.GetHeader("Authorization")
		if identity == "" {
			identity = c.ClientIP()
		}
		key := redisx.RepeatSubmitKey(hash(identity + "|" + c.Request.Method + "|" + c.Request.URL.Path))

		ctx, cancel := context.WithTimeout(c.Request.Context(), repeatSubmitTimeout)
		defer cancel()

		// Lua 原子保持原语义：相同指纹拦截；不同指纹覆盖最近记录并重新计时。
		repeated, err := redisx.CheckRepeatSubmit(ctx, key, fingerprint, interval)
		if err != nil {
			slog.ErrorContext(ctx, "防重复提交原子判定失败，本次放行", "err", err)
			c.Next()
			return
		}

		if repeated {
			slog.WarnContext(ctx, "拦截重复提交",
				"method", c.Request.Method, "path", c.Request.URL.Path, "clientIP", c.ClientIP())
			// 文案与 Java 版 RepeatSubmitInterceptor 一致
			response.Fail(c, "不允许重复提交，请稍候再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// requestFingerprint 算请求参数的指纹。
//
// 返回 false 表示这次请求不参与判重（读不到 body、或者是文件上传）。
func requestFingerprint(c *gin.Context) (string, bool) {
	contentType := c.GetHeader("Content-Type")
	// 文件上传不判重：整个文件读进内存代价太大，而且同名文件重传通常是合法操作
	if isMultipartRequest(contentType) {
		return "", false
	}

	if c.Request.Body == nil {
		// 没有请求体时用查询串当指纹，否则所有无参 POST 会互相误判
		return hash(c.Request.URL.RawQuery), true
	}
	if body, ok := cachedRequestBody(c); ok {
		return hashRequest(c.Request.URL.RawQuery, body), true
	}

	// 没经过入口限制就不读取，避免独立装配时重新引入无上限 io.ReadAll。
	return "", false
}

func hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

func hashRequest(rawQuery string, body []byte) string {
	digest := sha256.New()
	_, _ = io.WriteString(digest, rawQuery)
	_, _ = digest.Write([]byte{'|'})
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil)[:16])
}
