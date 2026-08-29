package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// configCacheRevisionKey 是参数缓存的内部代数，不对应任何 sys_config.config_key。
// 它沿用 sys_config: 前缀，避免扩张部署清理契约。
const configCacheRevisionKey = KeySysConfig + "__ruoyi_go_revision__"

var configCacheGetScript = redis.NewScript(`
local value = redis.call('get', KEYS[1])
if value and redis.call('pttl', KEYS[1]) == -1 then
    redis.call('pexpire', KEYS[1], ARGV[1])
end
return value
`)

var configCacheStoreScript = redis.NewScript(`
local revision = redis.call('get', KEYS[2]) or '0'
if revision ~= ARGV[2] then
    return 0
end
if redis.call('exists', KEYS[1]) == 0 then
    redis.call('psetex', KEYS[1], ARGV[3], ARGV[1])
end
return 1
`)

var configCacheInvalidateScript = redis.NewScript(`
redis.call('incr', KEYS[1])
for i = 2, #KEYS do
    redis.call('del', KEYS[i])
end
return 1
`)

// GetConfigCache 读取参数缓存，并为升级前没有 TTL 的旧缓存补上有限生命周期。
func GetConfigCache(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return configCacheGetScript.Run(ctx, client, []string{key}, durationMilliseconds(ttl)).Text()
}

// ConfigCacheRevision 返回当前参数缓存代数。不存在表示尚未发生过失效，按 0 处理。
func ConfigCacheRevision(ctx context.Context) (int64, error) {
	revision, err := client.Get(ctx, configCacheRevisionKey).Int64()
	if IsNil(err) {
		return 0, nil
	}
	return revision, err
}

// StoreConfigCacheIfRevision 仅在回源期间没有发生参数更新时写入缓存。
// 返回 false 表示代数已变化，本次数据库快照不得写回。
func StoreConfigCacheIfRevision(
	ctx context.Context,
	key, value string,
	revision int64,
	ttl time.Duration,
) (bool, error) {
	result, err := configCacheStoreScript.Run(ctx, client, []string{key, configCacheRevisionKey},
		value, revision, durationMilliseconds(ttl)).Int64()
	return result == 1, err
}

// InvalidateConfigCache 先推进全局代数，再原子删除指定参数缓存。
// 即使并发读已经拿到旧数据库值，也无法在失效完成后把旧值重新写回。
func InvalidateConfigCache(ctx context.Context, keys ...string) error {
	scriptKeys := make([]string, 1, len(keys)+1)
	scriptKeys[0] = configCacheRevisionKey
	for _, key := range keys {
		if key != "" && !IsConfigCacheMetadataKey(key) {
			scriptKeys = append(scriptKeys, key)
		}
	}
	return configCacheInvalidateScript.Run(ctx, client, scriptKeys).Err()
}

// IsConfigCacheMetadataKey 判断 key 是否是 Go 内部的参数缓存一致性元数据。
func IsConfigCacheMetadataKey(key string) bool { return key == configCacheRevisionKey }

func durationMilliseconds(value time.Duration) int64 {
	milliseconds := value.Milliseconds()
	if milliseconds < 1 {
		return 1
	}
	return milliseconds
}
