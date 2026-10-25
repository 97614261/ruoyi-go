package decimalx

import "testing"

func TestCompareSubtractExact(t *testing.T) {
	for _, x := range []struct {
		a, b, d string
		c       int
	}{{"10", "2.0001", "7.9999", 1}, {"0.3", "0.2", "0.1000", 1}, {"9999999999999999.9999", "0.0001", "9999999999999999.9998", 1}, {"0", "0", "0.0000", 0}} {
		n, e := Compare(x.a, x.b)
		if e != nil || n != x.c {
			t.Fatal(n, e)
		}
		s, e := Subtract(x.a, x.b)
		if e != nil || s != x.d {
			t.Fatal(s, e)
		}
	}
	if _, e := Subtract("0", "0.0001"); e == nil {
		t.Fatal("negative accepted")
	}
	for _, s := range []string{"-1", "1e2", "1.00001", "01", "", "NaN"} {
		if _, e := Compare(s, "0"); e == nil {
			t.Fatal(s)
		}
		if _, e := Subtract("1", s); e == nil {
			t.Fatal(s)
		}
	}
}
