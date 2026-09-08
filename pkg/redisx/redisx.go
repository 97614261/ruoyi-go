// Package redisx 封装 Redis 客户端与 key 常量。
package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

// ErrStopScan lets a callback finish a SCAN successfully without walking the
// remaining keyspace. It is useful for bounded list endpoints.
var ErrStopScan = errors.New("停止扫描")

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

// Stats returns Redis client pool counters for health checks and load tests.
func Stats() (map[string]any, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	stats := client.PoolStats()
	return map[string]any{
		"hits":            stats.Hits,
		"misses":          stats.Misses,
		"timeouts":        stats.Timeouts,
		"waitCount":       stats.WaitCount,
		"waitMs":          stats.WaitDurationNs / int64(time.Millisecond),
		"unusable":        stats.Unusable,
		"totalConns":      stats.TotalConns,
		"idleConns":       stats.IdleConns,
		"staleConns":      stats.StaleConns,
		"pendingRequests": stats.PendingRequests,
	}, nil
}

// Close 关闭连接。
func Close() error {
	if client == nil {
		return nil
	}
	return client.Close()
}

// ScanKeys 按前缀逐个迭代 key。
//
// 【重要】Java 版 TokenService.refreshPermissionByRoleId 用的是 KEYS 命令
// （见 TokenService.java:243），KEYS 会阻塞整个 Redis 实例，在线用户多时
// 会造成全站卡顿。Go 版一律用 SCAN 游标迭代，禁止使用 KEYS。
//
// fn 返回 error 时中止迭代。
func ScanKeys(ctx context.Context, prefix string, batch int64, fn func(key string) error) error {
	return ScanKeyBatches(ctx, prefix, batch, func(keys []string) error {
		for _, key := range keys {
			if err := fn(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// ScanKeyBatches 按 SCAN 返回的批次迭代 key，供 MGET/Pipeline 批量读取使用。
func ScanKeyBatches(ctx context.Context, prefix string, batch int64, fn func(keys []string) error) error {
	return scanKeyBatches(ctx, prefix, batch, func(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
		return client.Scan(ctx, cursor, pattern, count).Result()
	}, fn)
}

// ScanKeyBatchesBounded 最多把 limit 个 key 交给回调，并准确报告 Redis 中
// 是否还有未处理 key。最后一个 SCAN 批次会在调用回调前裁剪。
func ScanKeyBatchesBounded(ctx context.Context, prefix string, batch int64, limit int,
	fn func(keys []string) error) (bool, error) {
	return scanKeyBatchesBounded(ctx, prefix, batch, limit,
		func(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
			return client.Scan(ctx, cursor, pattern, count).Result()
		}, fn)
}

func scanKeyBatchesBounded(
	ctx context.Context,
	prefix string,
	batch int64,
	limit int,
	scan func(context.Context, uint64, string, int64) ([]string, uint64, error),
	fn func(keys []string) error,
) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	if batch <= 0 {
		batch = 100
	}
	processed := 0
	var cursor uint64
	for {
		keys, next, err := scan(ctx, cursor, prefix+"*", batch)
		if err != nil {
			return false, fmt.Errorf("SCAN %s* 失败: %w", prefix, err)
		}
		remaining := limit - processed
		truncatedBatch := len(keys) > remaining
		if truncatedBatch {
			keys = keys[:remaining]
		}
		if len(keys) > 0 {
			if err := fn(keys); err != nil {
				if errors.Is(err, ErrStopScan) {
					return false, nil
				}
				return false, err
			}
			processed += len(keys)
		}
		if truncatedBatch || (processed >= limit && next != 0) {
			return true, nil
		}
		if next == 0 {
			return false, nil
		}
		cursor = next
	}
}

func scanKeyBatches(
	ctx context.Context,
	prefix string,
	batch int64,
	scan func(context.Context, uint64, string, int64) ([]string, uint64, error),
	fn func(keys []string) error,
) error {
	if batch <= 0 {
		batch = 100
	}
	var cursor uint64
	for {
		keys, next, err := scan(ctx, cursor, prefix+"*", batch)
		if err != nil {
			return fmt.Errorf("SCAN %s* 失败: %w", prefix, err)
		}
		if len(keys) > 0 {
			if err := fn(keys); err != nil {
				if errors.Is(err, ErrStopScan) {
					return nil
				}
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
