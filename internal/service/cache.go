package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/redisx"
)

// maxCacheKeys 缓存监控单次列出的 key 上限，防止一次拉几十万个 key。
const maxCacheKeys = 500

// cacheNames 缓存监控里可浏览的缓存分类。
//
// 【刻意不包含 login_tokens】会话里有用户信息和权限，
// 让任何有缓存监控权限的人明文查看在线用户的会话内容是不合适的；
// 要看在线用户请用在线用户页面，那里只展示必要字段。
var cacheNames = []model.CacheItem{
	{CacheName: redisx.KeySysConfig, Remark: "配置信息"},
	{CacheName: redisx.KeySysDict, Remark: "数据字典"},
	{CacheName: redisx.KeyCaptchaCode, Remark: "验证码"},
	{CacheName: redisx.KeyRepeatSubmit, Remark: "防重提交"},
	{CacheName: redisx.KeyRateLimit, Remark: "限流处理"},
	{CacheName: redisx.KeyPwdErrCnt, Remark: "密码错误次数"},
}

// CacheOverview Redis 概览：info、dbSize、命令统计。
func CacheOverview(ctx context.Context) (map[string]any, error) {
	info, err := redisx.C().Info(ctx).Result()
	if err != nil {
		return nil, errs.Wrap(err, "获取 Redis 信息失败")
	}
	dbSize, err := redisx.C().DBSize(ctx).Result()
	if err != nil {
		return nil, errs.Wrap(err, "获取 Redis 键数量失败")
	}
	commandStats, err := redisx.C().Info(ctx, "commandstats").Result()
	if err != nil {
		return nil, errs.Wrap(err, "获取 Redis 命令统计失败")
	}

	return map[string]any{
		"info":         parseRedisInfo(info),
		"dbSize":       dbSize,
		"commandStats": parseCommandStats(commandStats),
	}, nil
}

// parseRedisInfo 把 INFO 的文本输出解析成键值对。
func parseRedisInfo(raw string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if found {
			result[key] = value
		}
	}
	return result
}

// parseCommandStats 把 commandstats 解析成前端饼图要的 [{name, value}]。
//
// 原始形如：cmdstat_get:calls=12,usec=100,usec_per_call=8.33
func parseCommandStats(raw string) []map[string]string {
	stats := make([]map[string]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "cmdstat_") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		calls := ""
		for _, part := range strings.Split(value, ",") {
			if k, v, ok := strings.Cut(part, "="); ok && k == "calls" {
				calls = v
				break
			}
		}
		stats = append(stats, map[string]string{
			"name":  strings.TrimPrefix(key, "cmdstat_"),
			"value": calls,
		})
	}
	return stats
}

// CacheNames 可浏览的缓存分类列表。
func CacheNames() []model.CacheItem {
	result := make([]model.CacheItem, len(cacheNames))
	copy(result, cacheNames)
	return result
}

// CacheKeys 列出某个缓存分类下的 key。
func CacheKeys(ctx context.Context, cacheName string) ([]string, error) {
	if !isAllowedCacheName(cacheName) {
		return nil, errs.New("不支持查看该缓存")
	}

	keys := make([]string, 0, 64)
	err := redisx.ScanKeys(ctx, cacheName, 200, func(key string) error {
		if redisx.IsConfigCacheMetadataKey(key) {
			return nil
		}
		if len(keys) >= maxCacheKeys {
			return nil
		}
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// CacheValue 查看某个 key 的值。
func CacheValue(ctx context.Context, cacheName, cacheKey string) (*model.CacheItem, error) {
	if !isAllowedCacheName(cacheName) {
		return nil, errs.New("不支持查看该缓存")
	}
	// 前端传的 cacheKey 是完整 key，兼容只传后缀的情况
	fullKey := cacheKey
	if !strings.HasPrefix(fullKey, cacheName) {
		fullKey = cacheName + cacheKey
	}

	value, err := redisx.C().Get(ctx, fullKey).Result()
	if errors.Is(err, redis.Nil) {
		value = ""
	} else if err != nil {
		return nil, errs.Wrap(err, "读取缓存失败")
	}

	return &model.CacheItem{
		CacheName:  cacheName,
		CacheKey:   cacheKey,
		CacheValue: value,
	}, nil
}

// ClearCacheByName 清空某个缓存分类。
func ClearCacheByName(ctx context.Context, cacheName string) error {
	if !isAllowedCacheName(cacheName) {
		return errs.New("不支持清理该缓存")
	}
	if cacheName == redisx.KeySysConfig {
		return ClearConfigCache(ctx, "")
	}
	return redisx.ScanKeys(ctx, cacheName, 200, func(key string) error {
		return redisx.C().Del(ctx, key).Err()
	})
}

// ClearCacheByKey 删除单个 key。
func ClearCacheByKey(ctx context.Context, cacheKey string) error {
	if cacheKey == "" {
		return errs.New("缓存键不能为空")
	}
	// 只允许删白名单前缀下的 key，避免误删（或恶意删）其它业务数据
	if !isAllowedCacheName(prefixOf(cacheKey)) {
		return errs.New("不支持清理该缓存")
	}
	if redisx.IsConfigCacheMetadataKey(cacheKey) {
		return errs.New("不能清理内部缓存元数据")
	}
	if strings.HasPrefix(cacheKey, redisx.KeySysConfig) {
		return ClearConfigCache(ctx, strings.TrimPrefix(cacheKey, redisx.KeySysConfig))
	}
	return redisx.C().Del(ctx, cacheKey).Err()
}

// ClearCacheAll 清空全部可管理的缓存分类。
//
// 【不是 FLUSHDB】只清白名单里的前缀。同一个 Redis 实例上可能还有
// 别的业务数据，FLUSHDB 会一起抹掉。
func ClearCacheAll(ctx context.Context) error {
	for _, item := range cacheNames {
		if err := ClearCacheByName(ctx, item.CacheName); err != nil {
			return fmt.Errorf("清理 %s 失败: %w", item.CacheName, err)
		}
	}
	return nil
}

func isAllowedCacheName(cacheName string) bool {
	for _, item := range cacheNames {
		if item.CacheName == cacheName {
			return true
		}
	}
	return false
}

// prefixOf 取 key 的前缀（含冒号）。
func prefixOf(key string) string {
	if idx := strings.Index(key, ":"); idx >= 0 {
		return key[:idx+1]
	}
	return key
}
