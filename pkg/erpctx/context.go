// Package erpctx carries server-authenticated actor identity, never client scope.
package erpctx

import "context"

type key struct{}

func WithUser(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, key{}, userID)
}
func UserID(ctx context.Context) int64 { id, _ := ctx.Value(key{}).(int64); return id }
