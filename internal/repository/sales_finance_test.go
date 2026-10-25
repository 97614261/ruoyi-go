package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestSalesFinanceDueAndOvercollection(t *testing.T) {
	for _, x := range []struct{ gross, credit, paid, due, over string }{{"10.0000", "0.0000", "3.0000", "7.0000", "0.0000"}, {"10.0000", "8.0000", "3.0000", "0.0000", "1.0000"}, {"0.3000", "0.1000", "0.2000", "0.0000", "0.0000"}, {"0.0000", "0.0000", "0.0000", "0.0000", "0.0000"}} {
		p := model.SalesFinReceivable{GrossAmount: x.gross, CreditAmount: x.credit, ReceivedAmount: x.paid}
		if e := salesFinanceReceivableTotals(&p); e != nil || p.DueAmount != x.due || p.OverreceivedAmount != x.over {
			t.Fatal(p, e)
		}
	}
	if salesFinanceReceivableTotals(&model.SalesFinReceivable{GrossAmount: "1", CreditAmount: "2", ReceivedAmount: "0"}) == nil {
		t.Fatal("negative net accepted")
	}
}
func TestSalesFinanceCollectionScopeNoPartialTotal(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 23)
	q := salesFinanceCollectionScope(guardDB(t).WithContext(ctx).Model(&model.SalesFinCollection{})).Find(&[]model.SalesFinCollection{})
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	s := q.Statement.SQL.String()
	for _, x := range []string{"NOT EXISTS", "a.tenant_id", "r.tenant_id", "owner_user_id", "erp_warehouse_access", "erp_shop_access"} {
		if !strings.Contains(s, x) {
			t.Fatal(s)
		}
	}
	if salesFinanceCollectionScope(guardDB(t).Model(&model.SalesFinCollection{})).Find(&[]model.SalesFinCollection{}).Error == nil {
		t.Fatal("missing identity allowed")
	}

}
