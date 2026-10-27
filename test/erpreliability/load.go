package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
)

var loadPhase atomic.Value

func setPhase(p string) {
	loadPhase.Store(p)
	_ = save(relRoot+"/phase.json", map[string]any{"phase": p, "atUtc": time.Now().UTC()})
}
func phaseName() string {
	v := loadPhase.Load()
	if v == nil {
		return "setup"
	}
	return v.(string)
}

type processTarget struct {
	Name string `json:"name"`
	ID   int32  `json:"id"`
}
type cpuPoint struct {
	Seconds float64
	At      time.Time
}

func observeHost(ctx context.Context, child *privateChild, issues chan<- error) {
	var location struct {
		Path string `json:"path"`
	}
	must(load(relRoot+"/monitor-location.json", &location))
	absolute, e := filepath.Abs(location.Path)
	must(e)
	root, e := filepath.Abs(relRoot)
	must(e)
	if !strings.HasPrefix(absolute, root+string(os.PathSeparator)) {
		panic(fail("native monitor escaped private root"))
	}
	started := time.Now()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	high := 0
	for {
		now := time.Now()
		var sample map[string]any
		err := load(filepath.Join(absolute, "latest.json"), &sample)
		if err != nil {
			if time.Since(started) > 15*time.Second {
				select {
				case issues <- fail("native monitor did not become ready"):
				default:
				}
				return
			}
		} else {
			at, e := time.Parse(time.RFC3339Nano, fmt.Sprint(sample["atUtc"]))
			if e != nil || now.Sub(at) > 15*time.Second {
				select {
				case issues <- fail("native monitor stalled"):
				default:
				}
				return
			}
			if bad, ok := sample["errors"].([]any); ok && len(bad) > 0 {
				select {
				case issues <- fail("native process resource sample unavailable"):
				default:
				}
				return
			}
			total := float64(0)
			if processes, ok := sample["processes"].([]any); ok {
				for _, item := range processes {
					p := item.(map[string]any)
					if cpu, ok := p["cpuMachinePercent"].(float64); ok {
						total += cpu
					}
					if p["name"] == "test-server" && p["workingSetBytes"].(float64) > 512<<20 {
						select {
						case issues <- fail("test server working set protection triggered"):
						default:
						}
						return
					}
				}
			}
			vm, e := mem.VirtualMemory()
			if e != nil {
				select {
				case issues <- fail("host memory unavailable"):
				default:
				}
				return
			}
			sample["phase"] = phaseName()
			sample["availableBytes"] = vm.Available
			must(appendEvidence(relRoot+"/host-metrics.jsonl", sample))
			if total > 90 {
				high++
			} else {
				high = 0
			}
			if high >= 3 || vm.Available < 2<<30 {
				select {
				case issues <- fail("host resource protection triggered"):
				default:
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func idleObserve(duration time.Duration, issues <-chan error) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case e := <-issues:
		panic(e)
	case <-timer.C:
	}
}
func captureLiveHeap(c *privateChild, label string) {
	must(save(c.root+"/gc-request.json", map[string]string{"label": label}))
	for i := 0; i < 100; i++ {
		if _, e := os.Stat(c.root + "/heap-" + label + ".json"); e == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	panic(fail("live heap capture missing"))
}
func readerLoop(a api, f *relFixture, token string, end time.Time, stop *atomic.Bool, issues chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			stop.Store(true)
			select {
			case issues <- fail("read worker failed"):
			default:
			}
		}
	}()
	paths := []string{"/erp/order/list?pageSize=20", "/erp/order/workbench", "/erp/stock/list", "/erp/overview/orders"}
	for i := 0; time.Now().Before(end) && !stop.Load(); i++ {
		v := measured(a, "GET", paths[i%len(paths)], token, nil, "read_"+fmt.Sprint(i%len(paths)))
		if i%len(paths) == 0 {
			var prev int64
			for n, row := range v["rows"].([]any) {
				o := row.(map[string]any)
				id := field(o, "orderId")
				if field(o, "shopId") != f.Shop || id < 1 || (n > 0 && id >= prev) {
					panic(fail("reader tenant/order drift"))
				}
				if _, ok := o["receiverName"]; ok {
					panic(fail("private order data in summary"))
				}
				prev = id
			}
		} else if i%len(paths) == 2 {
			for _, row := range v["rows"].([]any) {
				o := row.(map[string]any)
				if field(o, "warehouseId") != f.Warehouse || field(o, "skuId") != f.SKU || field(o, "onHand") < field(o, "reserved") {
					panic(fail("stock reader scope/balance drift"))
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}
func sustainedRun(c *privateChild, s relState, duration time.Duration) {
	a := childAPI(c)
	defer a.cache.Close()
	reloadFixtures(a, &s)
	setPhase("warmup")
	next := map[int64]int64{}
	for _, r := range readCycles() {
		if r.Sequence >= next[r.TenantID] {
			next[r.TenantID] = r.Sequence + 1
		}
	}
	for _, f := range []*relFixture{s.Fixtures[0], s.Fixtures[2]} {
		realCycle(a, f, next[f.ID])
		next[f.ID]++
	}
	timings.mu.Lock()
	timings.Rows = map[string][]time.Duration{}
	timings.Errors = 0
	timings.Requests = 0
	timings.mu.Unlock()
	issues := make(chan error, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go observeHost(ctx, c, issues)
	setPhase("baseline")
	idleObserve(baselineDuration, issues)
	captureLiveHeap(c, "baseline")
	start := time.Now()
	end := start.Add(duration)
	setPhase("load")
	var stop atomic.Bool
	var workers sync.WaitGroup
	for _, f := range []*relFixture{s.Fixtures[0], s.Fixtures[2]} {
		workers.Add(1)
		go func(f *relFixture) {
			defer workers.Done()
			defer func() {
				if recover() != nil {
					stop.Store(true)
					select {
					case issues <- fail("business flow worker failed"):
					default:
					}
				}
			}()
			seq := next[f.ID]
			for time.Now().Before(end) && !stop.Load() {
				realCycle(a, f, seq)
				seq++
				time.Sleep(time.Second)
			}
		}(f)
	}
	for i := 0; i < 4; i++ {
		f, token := s.Fixtures[0], s.Fixtures[0].Reader
		if i >= 2 {
			f, token = s.Fixtures[2], s.Fixtures[2].Owner
		}
		workers.Add(1)
		go func(f *relFixture, token string) { defer workers.Done(); readerLoop(a, f, token, end, &stop, issues) }(f, token)
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	progress := time.NewTicker(30 * time.Second)
	defer progress.Stop()
run:
	for {
		select {
		case e := <-issues:
			stop.Store(true)
			<-done
			panic(e)
		case <-progress.C:
			timings.mu.Lock()
			requests, errors := timings.Requests, timings.Errors
			timings.mu.Unlock()
			must(save(relRoot+"/progress.json", map[string]any{"phase": "load", "atUtc": time.Now().UTC(), "elapsedSeconds": time.Since(start).Seconds(), "plannedSeconds": duration.Seconds(), "requests": requests, "errors": errors, "cycles": len(readCycles())}))
			fmt.Printf("LOAD %.0fs/%0.fs requests=%d errors=%d\n", time.Since(start).Seconds(), duration.Seconds(), requests, errors)
		case <-done:
			break run
		}
	}
	drained := time.Now()
	select {
	case e := <-issues:
		panic(e)
	default:
	}
	if timings.Errors != 0 {
		panic(fail("load recorded request errors"))
	}
	if drained.Before(end) {
		panic(fail("load did not complete requested duration"))
	}
	setPhase("cooldown")
	must(save(relRoot+"/load-complete.json", map[string]any{"startedAtUtc": start.UTC(), "schedulingEndedAtUtc": end.UTC(), "drainedAtUtc": drained.UTC(), "elapsedSeconds": drained.Sub(start).Seconds(), "scheduledSeconds": duration.Seconds(), "summary": timingSummary(), "concurrency": 6, "readers": 4, "writers": 2, "writerPauseSeconds": 1, "readerPauseMilliseconds": 250, "forcedGCDuringLoad": false}))
	verifyFlowFacts(s, "after-load")
	idleObserve(cooldownDuration, issues)
	captureLiveHeap(c, "cooldown")
	setPhase("finished")
	select {
	case e := <-issues:
		panic(e)
	default:
	}
	verifyFlowFacts(s, "after-cooldown")
	var before, after map[string]any
	must(load(c.root+"/heap-baseline.json", &before))
	must(load(c.root+"/heap-cooldown.json", &after))
	must(save(relRoot+"/resource-complete.json", map[string]any{"baselineSeconds": baselineDuration.Seconds(), "loadSeconds": duration.Seconds(), "cooldownSeconds": cooldownDuration.Seconds(), "baselineLiveHeap": before, "cooldownLiveHeap": after, "naturalRuntimeMetrics": c.root + "/runtime.jsonl", "hostMetrics": relRoot + "/host-metrics.jsonl", "gcOnlyAtDiagnosticBoundaries": true}))
	closeFixtures(a, s)
}

// Used only for a bounded private metrics view, never to carry credentials.
func readLastRuntime(path string) map[string]any {
	b, e := os.ReadFile(path)
	must(e)
	var out map[string]any
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		if len(line) > 0 {
			must(json.Unmarshal(line, &out))
		}
	}
	return out
}
