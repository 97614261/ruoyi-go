package redisx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func VerifyPODQueueRedis(ctx context.Context) error {
	if client == nil {
		return fmt.Errorf("Redis 未初始化")
	}
	info, err := client.Info(ctx, "server").Result()
	if err != nil {
		return fmt.Errorf("无法确认队列 Redis 版本")
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "redis_version:")), ".")
			major, err := strconv.Atoi(parts[0])
			if err == nil && major >= 4 {
				return nil
			}
		}
	}
	return fmt.Errorf("Asynq 需要 Redis 4.0 或以上；请为 POD 使用独立兼容实例，不替换其他项目的 Redis")
}
