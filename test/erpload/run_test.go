package erpload

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func options() Options {
	return Options{Concurrency: 8, Duration: time.Second, MaxRequests: 101, RequestTimeout: time.Second}
}
func target(url string) Target {
	return Target{Name: "read", Request: func(ctx context.Context, i int64) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/%d", url, i), nil)
	}, Check: JSON200}
}
func TestBoundsAndContentChecks(t *testing.T) {
	targets := []Target{target("http://localhost")}
	for _, mutate := range []func(*Options){func(o *Options) { o.Concurrency = 0 }, func(o *Options) { o.Concurrency = 257 }, func(o *Options) { o.Duration = 0 }, func(o *Options) { o.Duration = 3 * time.Minute }, func(o *Options) { o.MaxRequests = 0 }, func(o *Options) { o.MaxRequests = 250001 }, func(o *Options) { o.RequestTimeout = 0 }, func(o *Options) { o.RequestTimeout = 31 * time.Second }} {
		o := options()
		mutate(&o)
		if _, err := Run(context.Background(), http.DefaultClient, o, targets); err == nil {
			t.Fatal("unbounded configuration accepted")
		}
	}
	if _, e := Run(context.Background(), http.DefaultClient, options(), []Target{{Name: "missing-check", Request: targets[0].Request}}); e == nil {
		t.Fatal("missing content assertion accepted")
	}
	for _, raw := range []string{`{"code":401}`, `{"code":403}`, `{"code":500}`, `{}`, `not json`} {
		if JSON200([]byte(raw)) == nil {
			t.Fatal("HTTP200 business error counted as success")
		}
	}
	if JSON200([]byte(`{"code":200}`)) != nil {
		t.Fatal("success rejected")
	}
}
func TestConcurrentRequestsAreBoundedUniqueAndDrained(t *testing.T) {
	var active, peak atomic.Int64
	var mu sync.Mutex
	seen := map[string]bool{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		mu.Lock()
		if seen[r.URL.Path] {
			t.Error("duplicate request sequence")
		}
		seen[r.URL.Path] = true
		mu.Unlock()
		fmt.Fprint(w, `{"code":200}`)
	}))
	defer s.Close()
	r, e := Run(context.Background(), s.Client(), options(), []Target{target(s.URL)})
	if e != nil || r.Total.Requests != 101 || r.Total.Succeeded != 101 || len(seen) != 101 || peak.Load() > 8 || active.Load() != 0 || r.QPS <= 0 {
		t.Fatal("invalid bounded result", r, e)
	}
}
func TestErrorsAreCountedWithoutLeakingBodies(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/0":
			fmt.Fprint(w, `{"code":403,"secret":"token"}`)
		case "/1":
			w.WriteHeader(500)
		default:
			fmt.Fprint(w, `{"code":200}`)
		}
	}))
	defer s.Close()
	o := options()
	o.MaxRequests = 3
	r, e := Run(context.Background(), s.Client(), o, []Target{target(s.URL)})
	if e != nil || r.Total.Requests != 3 || r.Total.Succeeded != 1 || r.Total.Errors["content"] != 1 || r.Total.Errors["http_500"] != 1 {
		t.Fatal("errors miscounted", r, e)
	}
}
func TestDurationEndDrainsInsteadOfCancelling(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; fmt.Fprint(w, `{"code":200}`) }))
	defer s.Close()
	o := options()
	o.Concurrency = 1
	o.Duration = 20 * time.Millisecond
	done := make(chan Result, 1)
	go func() {
		r, e := Run(context.Background(), s.Client(), o, []Target{target(s.URL)})
		if e != nil {
			t.Error(e)
		}
		done <- r
	}()
	<-started
	time.Sleep(30 * time.Millisecond)
	close(release)
	r := <-done
	if r.Total.Requests != 1 || r.Total.Succeeded != 1 || r.Cancelled {
		t.Fatal("normal stage end cancelled the in-flight request", r)
	}
}
func TestExternalCancellationStopsScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Run(ctx, http.DefaultClient, options(), []Target{target("http://localhost")}); e == nil {
		t.Fatal("cancelled run was started")
	}
}
func TestNearestRankPercentilesIncludeFailedAttempts(t *testing.T) {
	s := summarize([]observation{{ms: 1}, {ms: 2}, {ms: 3}, {ms: 100, failure: "content"}})
	if s.P50 != 2 || s.P95 != 100 || s.P99 != 100 || s.Max != 100 || s.Succeeded != 3 {
		t.Fatal("percentiles or errors hidden", s)
	}
}
