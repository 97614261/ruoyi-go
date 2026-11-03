package repository

import "context"

type portalFilterKey struct{}
type PortalFilter struct {
	TagId    int64
	Featured bool
}

func PortalFilteredContext(ctx context.Context, tag int64, featured bool) context.Context {
	return context.WithValue(ctx, portalFilterKey{}, PortalFilter{tag, featured})
}
func portalTag(ctx context.Context) int64 {
	v, _ := ctx.Value(portalFilterKey{}).(PortalFilter)
	return v.TagId
}
func featuredOnly(ctx context.Context) bool {
	v, _ := ctx.Value(portalFilterKey{}).(PortalFilter)
	return v.Featured
}
