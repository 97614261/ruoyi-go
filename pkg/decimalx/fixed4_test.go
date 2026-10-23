package decimalx

import (
	"sync"
	"testing"
)

func TestFixed4(t *testing.T) {
	for _, s := range []string{"0", "0.0000", "0.1", "9999999999999999.9999"} {
		if _, e := Normalize(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"", "-1", "1e2", "+1", "01", "1.", ".1", "1.00001", "10000000000000000", "NaN", " 1", "1 ", "０"} {
		if _, e := Normalize(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
	if s, e := Multiply("0.1001", 3); e != nil || s != "0.3003" {
		t.Fatal(s, e)
	}
	if s, e := Add("0.1", "0.2"); e != nil || s != "0.3000" {
		t.Fatal(s, e)
	}
	if _, e := Multiply("9999999999999999.9999", 2); e == nil {
		t.Fatal("multiply overflow")
	}
	if _, e := Add("9999999999999999.9999", "0.0001"); e == nil {
		t.Fatal("sum overflow")
	}
	if s, e := Multiply("9999999999999999.9999", 0); e != nil || s != "0.0000" {
		t.Fatal(s, e)
	}
}
func TestFixed4MaximumNotMutated(t *testing.T) {
	var w sync.WaitGroup
	for i := 0; i < 20; i++ {
		w.Add(1)
		go func() {
			defer w.Done()
			for n := 0; n < 50; n++ {
				if _, e := Multiply("9999999999999999.9999", 2); e == nil {
					t.Error("overflow accepted")
				}
				if _, e := Normalize("9999999999999999.9999"); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	w.Wait()
}
