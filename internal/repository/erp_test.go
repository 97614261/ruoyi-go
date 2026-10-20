package repository

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestERPStateTransitions(t *testing.T) {
	for _, c := range []struct{ k, s, a, n, e string }{{"IN", "DRAFT", "confirm", "POSTED", "IN"}, {"SALE", "DRAFT", "confirm", "RESERVED", "RESERVE"}, {"SALE", "RESERVED", "ship", "SHIPPED", "OUT"}, {"SALE", "RESERVED", "cancel", "CANCELLED", "RELEASE"}, {"SALE", "DRAFT", "cancel", "CANCELLED", ""}, {"IN", "POSTED", "confirm", "POSTED", ""}, {"SALE", "SHIPPED", "ship", "SHIPPED", ""}, {"SALE", "CANCELLED", "cancel", "CANCELLED", ""}} {
		n, e, err := erpNextState(c.k, c.s, c.a)
		if err != nil || n != c.n || e != c.e {
			t.Fatalf("%+v got %s/%s err=%v", c, n, e, err)
		}
	}
	for _, c := range []struct{ k, s, a string }{{"IN", "POSTED", "cancel"}, {"SALE", "SHIPPED", "cancel"}, {"SALE", "DRAFT", "ship"}, {"SALE", "CANCELLED", "confirm"}, {"IN", "DRAFT", "ship"}} {
		if _, _, e := erpNextState(c.k, c.s, c.a); e == nil {
			t.Fatalf("illegal transition %+v", c)
		}
	}
}
func TestERPCountAndCorrectionArithmetic(t *testing.T) {
	for _, kind := range []string{"SR", "PR", "COUNT"} {
		n, event, err := erpNextState(kind, "DRAFT", "confirm")
		if err != nil || n != "POSTED" || event != kind {
			t.Fatal("new posting state")
		}
		if _, _, err = erpNextState(kind, "POSTED", "cancel"); err == nil {
			t.Fatal("posted correction cancellable")
		}
	}
	if h, r, err := erpApplyCount(10, 2, 2); err != nil || h != 2 || r != 2 {
		t.Fatal("count reservation")
	}
	for _, target := range []int64{-1, 1, 1000000001} {
		if _, _, err := erpApplyCount(10, 2, target); err == nil {
			t.Fatal("unsafe count accepted")
		}
	}
	if h, r, err := erpApplyBalance(5, 2, 3, "PR"); err != nil || h != 2 || r != 2 {
		t.Fatal("available-only purchase return")
	}
	if _, _, err := erpApplyBalance(5, 2, 4, "PR"); err == nil {
		t.Fatal("returned reserved stock")
	}
}
func TestERPStockArithmetic(t *testing.T) {
	h, r, e := erpApplyBalance(0, 0, 100, "IN")
	if e != nil || h != 100 || r != 0 {
		t.Fatal("in")
	}
	h, r, e = erpApplyBalance(h, r, 3, "RESERVE")
	if e != nil || h != 100 || r != 3 {
		t.Fatal("reserve")
	}
	h, r, e = erpApplyBalance(h, r, 3, "OUT")
	if e != nil || h != 97 || r != 0 {
		t.Fatal("out")
	}
	for _, c := range []struct {
		h, r, q int64
		e       string
	}{{0, 0, 1, "RESERVE"}, {1, 0, 1, "OUT"}, {1, 2, 1, "RELEASE"}, {model.ERPMaxStock, 0, 1, "IN"}, {1, 1, -1, "OUT"}, {1, 1, 1000000001, "IN"}} {
		if _, _, err := erpApplyBalance(c.h, c.r, c.q, c.e); err == nil {
			t.Fatalf("invalid arithmetic %+v", c)
		}
	}
}
