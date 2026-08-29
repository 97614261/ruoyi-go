package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var repeatSubmitScript = redis.NewScript(`
local previous = redis.call('get', KEYS[1])
if previous == ARGV[1] then
    return 1
end
redis.call('psetex', KEYS[1], ARGV[2], ARGV[1])
return 0
`)

// CheckRepeatSubmit 原子完成“比较最近指纹、必要时覆盖并续期”。
// true 表示同一指纹仍在窗口内，调用方应拦截本次请求。
func CheckRepeatSubmit(ctx context.Context, key, fingerprint string, interval time.Duration) (bool, error) {
	result, err := repeatSubmitScript.Run(ctx, client, []string{key}, fingerprint,
		durationMilliseconds(interval)).Int64()
	return result == 1, err
}
