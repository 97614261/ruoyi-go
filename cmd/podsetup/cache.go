package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"io"
	"net"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
	"strings"
	"time"
)

// This convenience command only stops the dedicated portable development instance.
func stopCache(ctx context.Context, cfg *config.Config) error {
	host, port, err := net.SplitHostPort(cfg.Redis.Addr)
	if err != nil || port != "6387" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("stop-cache 仅接受专用本地 6387 实例")
	}
	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	if err := repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		return err
	}
	if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		return err
	}
	defer redisx.Close()
	owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
	if err != nil || owner != cfg.POD.InstanceID {
		return errors.New("Redis 归属不匹配，拒绝停止")
	}
	if online, err := redisx.C().Exists(ctx, "pod:worker:heartbeat:"+cfg.POD.InstanceID).Result(); err != nil || online != 0 {
		return errors.New("请先停止 POD Worker 并等待心跳过期，再停止 Redis")
	}
	info, err := redisx.C().Info(ctx, "keyspace").Result()
	if err != nil {
		return errors.New("无法核对 Redis 数据库范围")
	}
	ownPrefix := fmt.Sprintf("db%d:", cfg.Redis.DB)
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "db") && !strings.HasPrefix(line, ownPrefix) {
			return errors.New("Redis 还有其他逻辑库数据，拒绝停止共享实例")
		}
	}
	// SHUTDOWN replies with EOF. Retrying it converts a normal exit into a dial error.
	shutdownClient := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB, MaxRetries: -1, PoolSize: 1, DialTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 3 * time.Second})
	defer shutdownClient.Close()
	if err := shutdownClient.ShutdownSave(ctx).Err(); err != nil {
		var networkError net.Error
		if !errors.As(err, &networkError) && !errors.Is(err, io.EOF) {
			return errors.New("Redis 拒绝保存退出，请检查专用实例")
		}
		fmt.Println("已发送 Redis 保存退出请求；连接关闭，请由脚本核对记录的进程是否退出。")
		return nil
	}
	fmt.Println("POD 专用 Redis 已保存并正常退出，未操作旧 Redis 服务。")
	return nil
}
