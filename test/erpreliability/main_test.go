package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfilesAreBounded(t *testing.T) {
	for _, p := range []string{"quick", "standard"} {
		b, d, c, e := profileTimes(p)
		if e != nil || b <= 0 || c <= 0 || d < 2*time.Minute || d > 30*time.Minute {
			t.Fatalf("invalid %s profile", p)
		}
	}
	if _, _, _, e := profileTimes("unlimited"); e == nil {
		t.Fatal("unbounded profile accepted")
	}
}
func TestEvidencePathAndFixtureOwnership(t *testing.T) {
	previous := relRoot
	defer func() { relRoot = previous }()
	relRoot = filepath.Join("logs", "erp-verify-vtest-123")
	if ownedFixture(100, &relFixture{ID: 100, OwnerID: 1, Code: "vtest-123-0"}) {
		t.Fatal("old tenant accepted")
	}
	if ownedFixture(100, &relFixture{ID: 101, OwnerID: 1, Code: "vother-123-0"}) {
		t.Fatal("other run accepted")
	}
	if ownedFixture(100, &relFixture{ID: 101, OwnerID: 0, Code: "vtest-123-0"}) {
		t.Fatal("missing identity accepted")
	}
	if !ownedFixture(100, &relFixture{ID: 101, OwnerID: 1, Code: "vtest-123-2"}) {
		t.Fatal("current fixture refused")
	}
	if e := validateRoot(t.TempDir()); e == nil {
		t.Fatal("foreign directory accepted")
	}
	if e := validateRoot(filepath.Join("logs", "..", "other")); e == nil {
		t.Fatal("escaped path accepted")
	}
}
func TestNearestRankIncludesTailAndDoesNotMutate(t *testing.T) {
	rows := []time.Duration{10 * time.Millisecond, time.Millisecond, 3 * time.Millisecond, 2 * time.Millisecond}
	if got := relQuantile(rows, .95); got != 10 {
		t.Fatalf("tail lost: %v", got)
	}
	if rows[0] != 10*time.Millisecond {
		t.Fatal("input timing evidence reordered")
	}
	if relQuantile(nil, .95) != 0 {
		t.Fatal("empty series")
	}
}
func TestSyntheticCredentialsAreNotSerialized(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fixture.json")
	if e := save(p, &relFixture{ID: 2, Owner: "owner-secret", Reader: "reader-secret"}); e != nil {
		t.Fatal(e)
	}
	var row map[string]any
	if e := load(p, &row); e != nil {
		t.Fatal(e)
	}
	if _, ok := row["Owner"]; ok {
		t.Fatal("owner token leaked")
	}
	if _, ok := row["Reader"]; ok {
		t.Fatal("reader token leaked")
	}
	if b, e := os.ReadFile(p); e != nil || len(b) == 0 {
		t.Fatal("missing evidence")
	}
}
