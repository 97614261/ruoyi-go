package redisx

import (
	"context"
	"fmt"
)

const PODInstanceKey = "pod:instance"

// ClaimPODInstance is atomic and will not claim a non-empty unmarked database.
// No existing key is deleted or overwritten, and the marker has no expiration.
func ClaimPODInstance(ctx context.Context, instanceID string) error {
	if client == nil || instanceID == "" {
		return fmt.Errorf("POD Redis 未初始化")
	}
	const script = `
local current = redis.call('GET', KEYS[1])
if current then
  if current == ARGV[1] then return 1 else return 0 end
end
if redis.call('DBSIZE') ~= 0 then return 0 end
redis.call('SET', KEYS[1], ARGV[1])
return 1`
	result, err := client.Eval(ctx, script, []string{PODInstanceKey}, instanceID).Int()
	if err != nil {
		return fmt.Errorf("POD Redis 隔离检查失败: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("Redis 逻辑库已被其他数据或实例占用，禁止启动；请另选空库，不要清空已有数据")
	}
	return nil
}
