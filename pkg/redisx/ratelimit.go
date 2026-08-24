package redisx

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// IsNil 判断错误是不是"key 不存在"。
//
// 【不要写成 err.Error() == "redis: nil"】那是在拿错误的文案当契约，
// 库升级改一个字就静默失效 —— 而且失效的方向是"把 key 不存在当成 Redis 故障"，
// 于是每次查不到都走降级分支。
func IsNil(err error) bool {
	return errors.Is(err, redis.Nil)
}

// limitScript 固定窗口计数器，与 Java 版 RedisConfig 里的 limitScript 逐行对应。
//
// 【为什么必须是 Lua 而不是 INCR + EXPIRE 两条命令】
// 分开发的话，两条命令之间进程崩了、或者 EXPIRE 因为网络抖动没执行成功，
// 这个 key 就永远不过期 —— 计数只增不减，该 IP 从此被永久封死。
// Lua 脚本在 Redis 里是原子执行的，不存在中间状态。
var limitScript = redis.NewScript(`
local key = KEYS[1]
local count = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local current = redis.call('get', key)
if current and tonumber(current) > count then
    return tonumber(current)
end
current = redis.call('incr', key)
if tonumber(current) == 1 then
    redis.call('expire', key, window)
end
return tonumber(current)
`)

// RateLimit 对 key 计数并返回当前窗口内的累计次数。
//
// 返回值大于 limit 表示已经超限。窗口是固定窗口：第一次请求时设置 TTL，
// 到期后计数清零重来。
//
// 【固定窗口的已知缺陷】窗口边界上可以打出双倍流量（窗口末尾 N 次 +
// 下个窗口开头 N 次）。滑动窗口能解决，代价是要存每次请求的时间戳。
// 对"防暴力破解"这个目的，固定窗口足够，也与 Java 版行为一致。
func RateLimit(ctx context.Context, key string, limit int, window time.Duration) (int64, error) {
	seconds := int(window.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return limitScript.Run(ctx, client, []string{key}, limit, seconds).Int64()
}
