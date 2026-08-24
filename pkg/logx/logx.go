// Package logx 给 slog 补上 traceId。
//
// 【为什么要有 traceId】
// 没有它的时候，一次请求产生的日志散落在几十条别的请求中间，
// 只能靠时间戳猜哪几行是同一次请求的。用户报"我刚才保存失败了"，
// 你连是哪一次请求都定位不到。
//
// 有了它，grep 一个 ID 就能把这次请求从进来到出去的所有日志拉全。
package logx

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// TraceIDKey 日志里的字段名，以及响应头 X-Trace-Id 的取值来源。
const TraceIDKey = "traceId"

// HeaderTraceID 响应头名称。
//
// 出错时前端可以把它显示出来，用户截个图就带着 ID，
// 省掉"你大概几点几分操作的"这轮来回。
const HeaderTraceID = "X-Trace-Id"

// traceCtxKey 用私有类型做 key，避免和别的包撞车。
type traceCtxKey struct{}

// NewTraceID 生成一个 traceId。
//
// 去掉了 uuid 的连字符：日志里靠双击选中复制，带连字符的会被断开。
func NewTraceID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// WithTraceID 把 traceId 放进 ctx。
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceCtxKey{}, id)
}

// TraceID 取出 traceId，没有则返回空串。
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceCtxKey{}).(string)
	return id
}

// ContextHandler 包一层 slog.Handler，自动把 ctx 里的 traceId 补进每条日志。
//
// 【为什么用 Handler 而不是到处传 logger】
// 换成"业务层自己 slog.With("traceId", ...)"的话，几百个调用点都要改，
// 而且漏一个就断一次链，靠 review 是保不住的。
// 包 Handler 只要调用方用了 XxxContext 系列方法就自动带上。
//
// 代价：必须用 slog.InfoContext(ctx, ...) 而不是 slog.Info(...)。
// 后者拿不到 ctx，traceId 就补不上 —— 这是唯一要记住的规矩。
type ContextHandler struct {
	slog.Handler
}

// Handle 实现 slog.Handler。
func (h ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := TraceID(ctx); id != "" {
		record.AddAttrs(slog.String(TraceIDKey, id))
	}
	return h.Handler.Handle(ctx, record)
}

// WithAttrs / WithGroup 必须重新包一层，否则 slog.With(...) 之后
// 拿到的是裸 Handler，traceId 就丢了。
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{Handler: h.Handler.WithGroup(name)}
}
