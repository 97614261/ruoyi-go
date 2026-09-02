package redisx

import (
	"context"
	"time"
)

// dictCacheRevisionKey 沿用既有 sys_dict: 前缀，不扩张 Redis Key 前缀契约。
const dictCacheRevisionKey = KeySysDict + "__ruoyi_go_revision__"

// DictCacheRevision 返回当前字典缓存代数。不存在表示尚未失效过，按 0 处理。
func DictCacheRevision(ctx context.Context) (int64, error) {
	revision, err := client.Get(ctx, dictCacheRevisionKey).Int64()
	if IsNil(err) {
		return 0, nil
	}
	return revision, err
}

// StoreDictCacheIfRevision 仅在数据库回源期间没有字典更新时写入缓存。
func StoreDictCacheIfRevision(
	ctx context.Context,
	key string,
	value []byte,
	revision int64,
	ttl time.Duration,
) (bool, error) {
	result, err := configCacheStoreScript.Run(ctx, client, []string{key, dictCacheRevisionKey},
		value, revision, durationMilliseconds(ttl)).Int64()
	return result == 1, err
}

// InvalidateDictCache 先推进全局代数，再原子删除指定字典缓存。
func InvalidateDictCache(ctx context.Context, keys ...string) error {
	scriptKeys := make([]string, 1, len(keys)+1)
	scriptKeys[0] = dictCacheRevisionKey
	for _, key := range keys {
		if key != "" && !IsDictCacheMetadataKey(key) {
			scriptKeys = append(scriptKeys, key)
		}
	}
	return configCacheInvalidateScript.Run(ctx, client, scriptKeys).Err()
}

// IsDictCacheMetadataKey 判断 key 是否是 Go 内部的字典缓存一致性元数据。
func IsDictCacheMetadataKey(key string) bool { return key == dictCacheRevisionKey }

// IsCacheMetadataKey 判断 key 是否是不能从缓存监控中查看或单独删除的内部元数据。
func IsCacheMetadataKey(key string) bool {
	return IsConfigCacheMetadataKey(key) || IsDictCacheMetadataKey(key)
}
