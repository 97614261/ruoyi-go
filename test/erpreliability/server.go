package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"gorm.io/gorm"
	"ruoyi-go/internal/middleware"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/validate"
)

type childInfo struct {
	PID        int    `json:"pid"`
	Base       string `json:"base"`
	Generation string `json:"generation"`
}
type faultControl struct {
	TenantID         int64 `json:"tenantId"`
	Key, Table, Mode string
}
type faultHit struct {
	TenantID         int64 `json:"tenantId"`
	Key, Table, Mode string
	ConnectionID     int64     `json:"connectionId"`
	At               time.Time `json:"atUtc"`
}

func appendEvidence(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	b = append(b, '\n')
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(b)
	return e
}

// This callback is linked only into the private verification executable. The
// shipped cmd/server never registers it or provides a fault/debug endpoint.
func installTransactionGate(root string) {
	var once sync.Mutex
	must(repository.DB(context.Background()).Callback().Create().Before("gorm:create").Register("reliability:pre-audit-gate", func(tx *gorm.DB) {
		var key, table string
		switch v := tx.Statement.Dest.(type) {
		case *model.ERPOrderEvent:
			if v.Action != "REVIEW" {
				return
			}
			key, table = v.RequestKey, "erp_order_event"
		case *model.FinAction:
			key, table = v.RequestKey, "erp_fin_action"
		default:
			return
		}
		var control faultControl
		if e := load(relRoot+"/fault-control.json", &control); e != nil {
			return
		}
		if key != control.Key || table != control.Table || tenantctx.ID(tx.Statement.Context) != control.TenantID {
			return
		}
		once.Lock()
		defer once.Unlock()
		var conn int64
		if e := tx.Statement.ConnPool.QueryRowContext(tx.Statement.Context, "SELECT CONNECTION_ID()").Scan(&conn); e != nil {
			tx.AddError(e)
			return
		}
		hit := faultHit{TenantID: control.TenantID, Key: key, Table: table, Mode: control.Mode, ConnectionID: conn, At: time.Now().UTC()}
		if e := save(root+"/fault-hit.json", hit); e != nil {
			tx.AddError(e)
			return
		}
		<-tx.Statement.Context.Done()
		tx.AddError(tx.Statement.Context.Err())
		_ = save(root+"/cancellation-observed.json", map[string]any{"atUtc": time.Now().UTC(), "key": key, "contextCancelled": true})
	}))
}

func serveReliability(generation string) {
	if generation == "" || filepath.Base(generation) != generation {
		panic(fail("invalid child generation"))
	}
	root := relRoot + "/children/" + generation
	if _, e := os.Stat(root); e != nil {
		panic(fail("parent-owned child directory required"))
	}
	cfg, _ := sourceConfig()
	if cfg.Redis.DB != 13 {
		panic(fail("local Redis13 required"))
	}
	initRepo(cfg)
	defer repository.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	must(redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB, PoolSize: cfg.Redis.PoolSize}))
	defer redisx.Close()
	service.InitToken(cfg.JWT)
	service.InitCaptcha(cfg.Captcha.Type)
	must(service.InitGenerator(cfg.Gen))
	must(validate.Register())
	// No scheduler/import worker: the helper may touch only its own test tenants.
	// Production routing, guards, services, repositories and pool settings remain.
	cfg.Server.Mode = "release"
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	installTransactionGate(root)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(e)
	srv := &http.Server{Handler: router.New(cfg), ReadTimeout: cfg.Server.ReadTimeout, WriteTimeout: cfg.Server.WriteTimeout}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(l) }()
	must(save(root+"/ready.json", childInfo{os.Getpid(), "http://" + l.Addr().String(), generation}))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		stats, e := repository.Stats(ctx)
		must(e)
		must(appendEvidence(root+"/runtime.jsonl", map[string]any{"atUtc": time.Now().UTC(), "pid": os.Getpid(), "goroutines": runtime.NumGoroutine(), "heapAlloc": mem.HeapAlloc, "heapInuse": mem.HeapInuse, "heapObjects": mem.HeapObjects, "totalAlloc": mem.TotalAlloc, "numGC": mem.NumGC, "db": stats}))
		select {
		case e := <-errCh:
			panic(fmt.Errorf("private listener stopped: %v", e))
		case <-ticker.C:
		}
		var gc struct {
			Label string `json:"label"`
		}
		if load(root+"/gc-request.json", &gc) == nil && (gc.Label == "baseline" || gc.Label == "cooldown") {
			if _, e := os.Stat(root + "/heap-" + gc.Label + ".json"); os.IsNotExist(e) {
				runtime.GC()
				var live runtime.MemStats
				runtime.ReadMemStats(&live)
				profile, e := os.Create(root + "/heap-" + gc.Label + ".pprof")
				must(e)
				must(pprof.WriteHeapProfile(profile))
				must(profile.Close())
				stats, e := repository.Stats(ctx)
				must(e)
				must(save(root+"/heap-"+gc.Label+".json", map[string]any{"atUtc": time.Now().UTC(), "diagnosticGC": true, "heapAlloc": live.HeapAlloc, "heapInuse": live.HeapInuse, "heapObjects": live.HeapObjects, "goroutines": runtime.NumGoroutine(), "numGC": live.NumGC, "db": stats}))
			}
		}
		if _, e := os.Stat(root + "/stop.json"); e == nil {
			stop, c := context.WithTimeout(context.Background(), 20*time.Second)
			_ = srv.Shutdown(stop)
			_ = middleware.ShutdownOperLogPool(stop)
			c()
			return
		}
	}
}
