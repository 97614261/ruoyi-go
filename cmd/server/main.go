// Command server 是 ruoyi-go 的 HTTP 服务入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/logx"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/validate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("启动失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/application.yml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	initLogger(cfg.Log.Level)

	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer func() {
		if err := repository.Close(); err != nil {
			slog.Error("关闭数据库连接失败", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := redisx.Init(ctx, redisx.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	}); err != nil {
		return err
	}
	defer func() {
		if err := redisx.Close(); err != nil {
			slog.Error("关闭 Redis 连接失败", "err", err)
		}
	}()

	// 依赖数据库和 Redis 就绪，必须放在两者初始化之后
	service.InitToken(cfg.JWT)
	service.InitCaptcha(cfg.Captcha.Type)

	// 定时任务调度器：从 sys_job 装载状态正常的任务。
	// 单个任务配错只记日志跳过，不影响服务启动。
	if err := service.StartScheduler(ctx); err != nil {
		return err
	}

	// 自定义校验规则必须在构建路由前注册
	if err := validate.Register(); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router.New(cfg),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("服务已启动", "addr", srv.Addr, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("监听失败: %w", err)
	case <-ctx.Done():
		slog.Info("收到退出信号，开始优雅关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// 先停调度器再停 HTTP：反过来的话，正在跑的定时任务会在
	// 数据库连接已经关掉之后才发现自己没法写日志
	service.StopScheduler(shutdownCtx)

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭超时: %w", err)
	}
	slog.Info("服务已退出")
	return nil
}

func initLogger(level string) {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(time.Now().Format("2006-01-02 15:04:05.000"))
			}
			return a
		},
	})
	// 包一层，让 slog.XxxContext 自动带出 traceId。
	// 用 slog.Info 这类不带 ctx 的方法就没有，写业务日志时注意用 InfoContext。
	slog.SetDefault(slog.New(logx.ContextHandler{Handler: h}))
}
