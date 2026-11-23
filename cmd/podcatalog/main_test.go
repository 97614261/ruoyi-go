package main

import "testing"

func TestReviewedPresetsAndExactMoney(t *testing.T) {
	rows, err := loadPresets("../../web/src/utils/pod-model-presets.json")
	if err != nil {
		t.Fatal(err)
	}
	free, unknown := 0, 0
	for _, p := range rows {
		if p.Cost == nil {
			unknown++
			continue
		}
		r, err := versionRequest(p)
		if err != nil {
			t.Fatal(err)
		}
		if r.CostRule.InputPerMillion == 0 && r.CostRule.OutputPerMillion == 0 && r.CostRule.PerImage == 0 {
			free++
		}
	}
	if free != 3 || unknown != 4 {
		t.Fatalf("free %d, unknown %d", free, unknown)
	}
	for s, want := range map[string]int64{"0": 0, "0.06": 60000, "0.8": 800000, "13": 13000000, "0.000001": 1} {
		if got, e := micros(s); e != nil || got != want {
			t.Fatalf("%s: %d %v", s, got, e)
		}
	}
	for _, s := range []string{"-1", "1e2", "0.0000001", "1000001", "1.0000001"} {
		if _, e := micros(s); e == nil {
			t.Fatal("invalid money accepted", s)
		}
	}
}
