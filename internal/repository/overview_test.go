package repository

import (
	"context"
	"gorm.io/gorm"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"testing"
)

func TestOverviewRequiresCustomerIdentityBeforeMetadata(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 9, false), erpctx.WithUser(tenantctx.With(context.Background(), 9, true), 12)} {
		called := false
		available, e := overviewRead(ctx, []string{"erp_order"}, func(*gorm.DB) error { called = true; return nil })
		if e == nil || available || called {
			t.Fatal("invalid identity reached database")
		}
	}
}
