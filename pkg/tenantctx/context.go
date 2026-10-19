package tenantctx

import (
	"context"
	"errors"
)

type key struct{}
type Scope struct {
	ID       int64
	Platform bool
	System   bool
}

var ErrMissing = errors.New("缺少有效的租户上下文")

func With(ctx context.Context, id int64, platform bool) context.Context {
	return context.WithValue(ctx, key{}, Scope{ID: id, Platform: platform})
}
func System(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, Scope{System: true})
}
func Get(ctx context.Context) Scope { s, _ := ctx.Value(key{}).(Scope); return s }
func ID(ctx context.Context) int64  { return Get(ctx).ID }
func Require(ctx context.Context) (Scope, error) {
	s := Get(ctx)
	if s.ID < 1 {
		return Scope{}, ErrMissing
	}
	return s, nil
}
