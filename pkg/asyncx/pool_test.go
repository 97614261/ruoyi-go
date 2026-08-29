package asyncx

import (
	"testing"
	"time"
)

func TestPoolAppliesBackpressure(t *testing.T) {
	pool := NewPool(1, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if !pool.Submit(func() { close(started); <-release }) {
		t.Fatal("first task should be accepted")
	}
	<-started
	if !pool.Submit(func() {}) {
		t.Fatal("one queued task should be accepted")
	}
	if pool.Submit(func() {}) {
		t.Fatal("full bounded queue must reject additional work")
	}
	close(release)
	select {
	case <-time.After(100 * time.Millisecond):
		// The worker has enough time to drain; the test only verifies bounded admission.
	}
}
