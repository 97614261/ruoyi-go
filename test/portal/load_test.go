package portaltest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestPortalBoundedReadLoad(t *testing.T) {
	if os.Getenv("PORTAL_LOAD") != "1" {
		t.Skip("explicit PORTAL_LOAD=1; 160 reads with 8 workers, not a capacity claim")
	}
	c := cmsSetup(t)
	_ = c
	const count = 160
	const concurrency = 8
	queue := make(chan int, count)
	for i := 0; i < count; i++ {
		queue <- i
	}
	close(queue)
	times := make(chan float64, count)
	errs := make(chan string, count)
	var wg sync.WaitGroup
	start := time.Now()
	paths := []string{"/", "/news", "/products", "/search?q=test", "/sitemap.xml"}
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 15 * time.Second}
			for i := range queue {
				at := time.Now()
				resp, e := client.Get("http://127.0.0.1:8085" + paths[i%len(paths)])
				if e != nil {
					errs <- "transport"
					continue
				}
				_, e = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if e != nil || resp.StatusCode != 200 {
					errs <- fmt.Sprint(resp.StatusCode)
					continue
				}
				times <- float64(time.Since(at).Microseconds()) / 1000
			}
		}()
	}
	wg.Wait()
	close(times)
	close(errs)
	latency := []float64{}
	for n := range times {
		latency = append(latency, n)
	}
	failures := []string{}
	for e := range errs {
		failures = append(failures, e)
	}
	if len(failures) > 0 || len(latency) != count {
		t.Fatal("bounded read failures", len(failures))
	}
	sort.Float64s(latency)
	elapsed := time.Since(start).Seconds()
	report := map[string]any{"requests": count, "concurrency": concurrency, "failures": len(failures), "elapsedSeconds": elapsed, "requestsPerSecond": float64(count) / elapsed, "p50ms": latency[count/2], "p95ms": latency[count*95/100], "p99ms": latency[count*99/100], "environment": "local Windows, small fixture data; not Linux or production capacity"}
	b, _ := json.MarshalIndent(report, "", "  ")
	if e := os.WriteFile("../../logs/portal-step56/load.local.json", b, 0600); e != nil {
		t.Fatal(e)
	}
	t.Log(string(b))
}
