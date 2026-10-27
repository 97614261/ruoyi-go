// Package erpload implements bounded, closed-loop HTTP measurements for local
// ERP acceptance. Fixture ownership and loopback/database guards belong to the
// caller. Tokens, bodies and raw errors are never included in measurement output.
package erpload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Target struct {
	Name    string
	Request func(context.Context, int64) (*http.Request, error)
	Check   func([]byte) error
}
type Options struct {
	Concurrency    int           `json:"concurrency"`
	Duration       time.Duration `json:"durationNs"`
	MaxRequests    int64         `json:"maxRequests"`
	RequestTimeout time.Duration `json:"requestTimeoutNs"`
}
type Stats struct {
	Requests  int            `json:"requests"`
	Succeeded int            `json:"succeeded"`
	Errors    map[string]int `json:"errors"`
	P50       float64        `json:"p50Ms"`
	P95       float64        `json:"p95Ms"`
	P99       float64        `json:"p99Ms"`
	Max       float64        `json:"maxMs"`
}
type Result struct {
	Started   time.Time        `json:"startedAtUtc"`
	Options   Options          `json:"options"`
	Elapsed   float64          `json:"elapsedSeconds"`
	QPS       float64          `json:"qps"`
	Total     Stats            `json:"total"`
	Targets   map[string]Stats `json:"targets"`
	Cancelled bool             `json:"cancelled"`
}
type observation struct {
	name, failure string
	ms            float64
}

// JSON200 checks the business envelope as well as HTTP status. An HTTP200 login
// expiry, permission denial or business error must never become successful QPS.
func JSON200(body []byte) error {
	var v struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(body, &v) != nil || v.Code != 200 {
		return fmt.Errorf("business envelope is not successful")
	}
	return nil
}

func summarize(rows []observation) Stats {
	s := Stats{Requests: len(rows), Errors: map[string]int{}}
	latencies := make([]float64, 0, len(rows))
	for _, r := range rows {
		latencies = append(latencies, r.ms)
		if r.failure == "" {
			s.Succeeded++
		} else {
			s.Errors[r.failure]++
		}
	}
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(math.Ceil(p*float64(len(latencies))))-1]
	}
	s.P50, s.P95, s.P99, s.Max = percentile(.5), percentile(.95), percentile(.99), percentile(1)
	return s
}

// Run stops scheduling at the duration/request bound and drains in-flight work.
// Their completion is included in elapsed time. Normal stage end does not cancel
// valid in-flight requests; external cancellation does. Percentiles include all
// completed attempts, including failures. There is no simulated user think time.
func Run(ctx context.Context, client *http.Client, opts Options, targets []Target) (Result, error) {
	var out Result
	if opts.Concurrency < 1 || opts.Concurrency > 256 || opts.Duration <= 0 || opts.Duration > 2*time.Minute || opts.MaxRequests < 1 || opts.MaxRequests > 250000 || opts.RequestTimeout <= 0 || opts.RequestTimeout > 30*time.Second || len(targets) == 0 || client == nil {
		return out, fmt.Errorf("invalid or unbounded load configuration")
	}
	for _, t := range targets {
		if t.Name == "" || t.Request == nil || t.Check == nil {
			return out, fmt.Errorf("target requires name, request and content check")
		}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	out.Started = time.Now().UTC()
	out.Options = opts
	start := time.Now()
	end := start.Add(opts.Duration)
	var next atomic.Int64
	var mu sync.Mutex
	rows := make([]observation, 0, int(min(opts.MaxRequests, 10000)))
	var wg sync.WaitGroup
	for w := 0; w < opts.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil || !time.Now().Before(end) {
					return
				}
				i := next.Add(1) - 1
				if i >= opts.MaxRequests {
					return
				}
				t := targets[i%int64(len(targets))]
				requestCtx, cancel := context.WithTimeout(ctx, opts.RequestTimeout)
				begin := time.Now()
				failure := ""
				req, err := t.Request(requestCtx, i)
				if err != nil || req == nil {
					failure = "request"
				} else {
					req = req.WithContext(requestCtx)
					res, err := client.Do(req)
					if err != nil {
						failure = "transport"
					} else {
						body, readErr := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
						closeErr := res.Body.Close()
						if readErr != nil || closeErr != nil {
							failure = "read"
						} else if res.StatusCode != 200 {
							failure = fmt.Sprintf("http_%d", res.StatusCode)
						} else if len(body) > 2<<20 {
							failure = "body_limit"
						} else if t.Check(body) != nil {
							failure = "content"
						}
					}
				}
				cancel()
				row := observation{name: t.Name, failure: failure, ms: float64(time.Since(begin)) / float64(time.Millisecond)}
				mu.Lock()
				rows = append(rows, row)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	out.Elapsed = time.Since(start).Seconds()
	out.Total = summarize(rows)
	out.Cancelled = ctx.Err() != nil
	out.QPS = float64(out.Total.Requests) / out.Elapsed
	byName := map[string][]observation{}
	for _, row := range rows {
		byName[row.name] = append(byName[row.name], row)
	}
	out.Targets = map[string]Stats{}
	for name, rs := range byName {
		out.Targets[name] = summarize(rs)
	}
	return out, nil
}
