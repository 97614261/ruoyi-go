package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorDefaultsRejectUnavailableAndWrongKind(t *testing.T) {
	usable := map[string]string{"1": "text", "2": "image"}
	for _, tt := range []struct{ kind, id, want string }{{"text", "1", "passed"}, {"image", "2", "passed"}, {"text", "2", "failed"}, {"image", "1", "failed"}, {"image", "99", "failed"}, {"image", "", "warning"}} {
		if got, _ := doctorDefault(tt.kind, tt.id, usable); got != tt.want {
			t.Fatalf("%+v: %s", tt, got)
		}
	}
}

func TestDoctorReportKeepsFailureAndDoesNotPrintConfigContents(t *testing.T) {
	r := doctorReport{Status: "passed"}
	r.add("first", "failed", "failed safely")
	r.add("second", "warning", "not a vendor probe")
	var out bytes.Buffer
	if r.write(&out) == nil || !strings.Contains(out.String(), `"status": "failed"`) || !strings.Contains(out.String(), `"vendorCalls": false`) {
		t.Fatal(out.String())
	}
	path := filepath.Join(t.TempDir(), "bad.yml")
	const marker = "SECRET-MUST-NOT-APPEAR"
	if err := os.WriteFile(path, []byte("pod: [\nsecret: "+marker+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := doctorFile(path, &out); err == nil {
		t.Fatal("invalid config accepted")
	}
	if strings.Contains(out.String(), marker) || strings.Contains(out.String(), path) {
		t.Fatal("configuration leaked")
	}
}
