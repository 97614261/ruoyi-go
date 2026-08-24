// Package redisx 封装 Redis 客户端与 key 常量。
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

// Options Redis 连接参数。
type Options struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// Init 建立连接并 ping 验证。
func Init(ctx context.Context, opt Options) error {
	if opt.PoolSize <= 0 {
		opt.PoolSize = 20
	}
	client = redis.NewClient(&redis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		DB:       opt.DB,
		PoolSize: opt.PoolSize,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("连接 Redis 失败(%s): %w", opt.Addr, err)
	}
	return nil
}

// C 返回客户端。调用前必须已 Init。
func C() *redis.Client { return client }

// Ping 探活，供健康检查用。
func Ping(ctx context.Context) error {
	if client == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	return client.Ping(ctx).Err()
}

// Close 关闭连接。
func Close() error {
	if client == nil {
		return nil
	}
	return client.Close()
}

// ScanKeys 按前缀迭代 key。
//
// 【重要】Java 版 TokenService.refreshPermissionByRoleId 用的是 KEYS 命令
// （见 TokenService.java:243），KEYS 会阻塞整个 Redis 实例，在线用户多时
// 会造成全站卡顿。Go 版一律用 SCAN 游标迭代，禁止使用 KEYS。
//
// fn 返回 error 时中止迭代。
func ScanKeys(ctx context.Context, prefix string, batch int64, fn func(key string) error) error {
	if batch <= 0 {
		batch = 100
	}
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, prefix+"*", batch).Result()
		if err != nil {
			return fmt.Errorf("SCAN %s* 失败: %w", prefix, err)
		}
		for _, k := range keys {
			if err := fn(k); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
