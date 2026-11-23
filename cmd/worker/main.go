package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"syscall"
	"time"
	_ "time/tzdata"
)

func main() {
	if err := run(); err != nil {
		slog.Error("POD Worker 停止", "reason", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "configs/pod.local.yml", "独立 POD 配置")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if err := config.ValidatePOD(cfg); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := repository.VerifyPODSchema(check, cfg.POD.InstanceID); err != nil {
		return err
	}
	if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB, PoolSize: cfg.Redis.PoolSize}); err != nil {
		return err
	}
	defer redisx.Close()
	if err := redisx.ClaimPODInstance(check, cfg.POD.InstanceID); err != nil {
		return err
	}
	if err := redisx.VerifyPODQueueRedis(check); err != nil {
		return err
	}
	if err := service.InitPODCatalog(cfg.POD.Catalog, cfg.JWT.Secret); err != nil {
		return err
	}
	if cfg.POD.Execution.Mode == "managed" {
		if err := repository.EnablePODManaged(ctx); err != nil {
			return err
		}
	}
	return service.RunPODWorker(ctx, cfg)
}
