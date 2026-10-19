package repository

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestLifecycleCannotBypassArchiveOrReviveSessions(t *testing.T) {
	for _, c := range []struct{ status, action, want string }{
		{"0", "requestCancellation", "3"}, {"1", "requestCancellation", "3"}, {"3", "archive", "4"}, {"4", "restore", "1"}, {"3", "rejectCancellation", "1"},
	} {
		got, ok := lifecycleNextState(c.status, c.action)
		if !ok || got != c.want {
			t.Fatalf("invalid transition %s %s", c.status, c.action)
		}
	}
	for _, c := range []struct{ status, action string }{{"0", "archive"}, {"4", "requestCancellation"}, {"4", "enable"}, {"3", "restore"}, {"2", "requestCancellation"}, {"1", "restore"}} {
		if _, ok := lifecycleNextState(c.status, c.action); ok {
			t.Fatal("accepted a lifecycle bypass")
		}
	}
	if customerMutable(&model.SysTenant{Status: "4"}) == nil || customerMutable(&model.SysTenant{Status: "3"}) == nil {
		t.Fatal("archived/pending tenant is still mutable")
	}
}
