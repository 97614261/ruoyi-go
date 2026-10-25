package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestFinanceDueAndOverpayment(t *testing.T) {
	for _, x := range []struct{ gross, credit, paid, due, over string }{{"10.0000", "0.0000", "3.0000", "7.0000", "0.0000"}, {"10.0000", "8.0000", "3.0000", "0.0000", "1.0000"}, {"0.3000", "0.1000", "0.2000", "0.0000", "0.0000"}, {"0.0000", "0.0000", "0.0000", "0.0000", "0.0000"}} {
		p := model.FinPayable{GrossAmount: x.gross, CreditAmount: x.credit, PaidAmount: x.paid}
		if e := financePayableTotals(&p); e != nil || p.DueAmount != x.due || p.OverpaidAmount != x.over {
			t.Fatal(p, e)
		}
	}
	if financePayableTotals(&model.FinPayable{GrossAmount: "1", CreditAmount: "2", PaidAmount: "0"}) == nil {
		t.Fatal("negative net accepted")
	}
}
func TestFinancePaymentScopeNoPartialTotal(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 23)
	q := financePaymentScope(guardDB(t).WithContext(ctx).Model(&model.FinPayment{})).Find(&[]model.FinPayment{})
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	s := q.Statement.SQL.String()
	for _, x := range []string{"NOT EXISTS", "fa.tenant_id", "fp.tenant_id", "owner_user_id", "erp_warehouse_access"} {
		if !strings.Contains(s, x) {
			t.Fatal(s)
		}
	}
	if financePaymentScope(guardDB(t).Model(&model.FinPayment{})).Find(&[]model.FinPayment{}).Error == nil {
		t.Fatal("missing identity allowed")
	}
	if ERPFinanceSHA256() != "6ba0c311f8783e687fd35492867cc2bf0acf22b7216cebfa29b4b928c04f495d" {
		t.Fatal("published008 changed")
	}
}
