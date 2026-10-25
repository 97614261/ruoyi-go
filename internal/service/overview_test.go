package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"testing"
	"time"
)

func TestOverviewShanghaiWindows(t *testing.T) {
	for _, tc := range []struct{ utc, today, seven, through string }{
		{"2026-01-01T15:59:59Z", "2026-01-01 00:00:00", "2025-12-26 00:00:00", "2026-01-02 00:00:00"},
		{"2026-01-01T16:00:00Z", "2026-01-02 00:00:00", "2025-12-27 00:00:00", "2026-01-03 00:00:00"},
		{"2028-02-29T16:00:00Z", "2028-03-01 00:00:00", "2028-02-24 00:00:00", "2028-03-02 00:00:00"},
	} {
		n, e := time.Parse(time.RFC3339, tc.utc)
		if e != nil {
			t.Fatal(e)
		}
		a, b, c := overviewWindow(n)
		if a != tc.today || b != tc.seven || c != tc.through {
			t.Fatalf("%s => %s %s %s", tc.utc, a, b, c)
		}
	}
}
func TestOverviewRejectsCurrencyBeforeReading(t *testing.T) {
	for _, s := range []string{"cny", " CNY", "CNY ", "CNY' OR 1=1", "人民币", "US", "USDD", "CN\n"} {
		if _, e := OverviewMoney(context.Background(), model.OverviewMoneyQuery{Currency: s}, page.Query{}, false); e == nil {
			t.Fatalf("accepted currency %q", s)
		}
	}
}
