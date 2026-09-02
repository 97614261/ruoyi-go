package service

import (
	"context"
	"time"

	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
)

// healthCheckTimeout 单项探活的超时。
//
// 必须短：健康检查本身卡住比返回"不健康"更糟糕 ——
// 探活接口超时的话，负载均衡器往往按"未知"处理并继续打流量。
const healthCheckTimeout = time.Second

// HealthResult 健康检查结果。
type HealthResult struct {
	Status string `json:"status"` // up / down
	MySQL  string `json:"mysql"`
	Redis  string `json:"redis"`
	// DB 连接池状态，排查连接泄漏用
	DB        map[string]any `json:"db,omitempty"`
	RedisPool map[string]any `json:"redisPool,omitempty"`
}

// Health 依次探活 MySQL 和 Redis。
//
// 【为什么不能只返回固定的 "up"】
// 进程活着不等于服务可用。数据库连不上时，一个硬编码 up 的探活接口
// 会让负载均衡器一直把流量打进来，每个请求都在超时后报 500 ——
// 比直接摘掉这个实例糟糕得多。
//
// 【为什么不检查磁盘、不查业务表】
// 探活要便宜且稳定。跑业务 SQL 会因为一条脏数据把整个实例判死，
// 查磁盘会因为日志目录满了误报。只验"依赖的中间件通不通"。
func Health(ctx context.Context) HealthResult {
	result := HealthResult{Status: "up", MySQL: "up", Redis: "up"}

	dbCtx, cancelDB := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancelDB()
	if err := repository.Ping(dbCtx); err != nil {
		result.MySQL = "down"
		result.Status = "down"
	} else if stats, err := repository.Stats(dbCtx); err == nil {
		result.DB = stats
	}

	redisCtx, cancelRedis := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancelRedis()
	if err := redisx.Ping(redisCtx); err != nil {
		result.Redis = "down"
		result.Status = "down"
	} else if stats, err := redisx.Stats(); err == nil {
		result.RedisPool = stats
	}

	return result
}
