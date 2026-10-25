package repository

import (
	"context"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"testing"
)

func TestSetupRejectsUnscopedActorsBeforeDB(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 1, true), erpctx.WithUser(tenantctx.With(context.Background(), 0, false), 2)} {
		if _, e := SelectERPSetup(ctx, nil); e == nil {
			t.Fatal("unscoped setup accepted")
		}
	}
}
