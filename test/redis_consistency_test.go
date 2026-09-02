package apitest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
)

func TestConfigCacheRejectsStaleWriteBack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cacheKey := redisx.SysConfigKey(fmt.Sprintf("%srevision_%d", testPrefix, time.Now().UnixNano()))
	trackRedisKey(cacheKey)
	if err := redisx.InvalidateConfigCache(ctx, cacheKey); err != nil {
		t.Fatalf("初始化参数缓存失败：%v", err)
	}

	oldRevision, err := redisx.ConfigCacheRevision(ctx)
	if err != nil {
		t.Fatalf("读取旧代数失败：%v", err)
	}
	if err := redisx.InvalidateConfigCache(ctx, cacheKey); err != nil {
		t.Fatalf("模拟并发参数更新失败：%v", err)
	}

	stored, err := redisx.StoreConfigCacheIfRevision(ctx, cacheKey, "stale", oldRevision, time.Minute)
	if err != nil {
		t.Fatalf("尝试写回旧值失败：%v", err)
	}
	if stored {
		t.Fatal("参数更新后，旧数据库快照不应重新写入缓存")
	}
	if _, err := redisx.C().Get(ctx, cacheKey).Result(); !redisx.IsNil(err) {
		t.Fatalf("旧值不应出现在缓存中，实际错误=%v", err)
	}

	currentRevision, err := redisx.ConfigCacheRevision(ctx)
	if err != nil {
		t.Fatalf("读取新代数失败：%v", err)
	}
	stored, err = redisx.StoreConfigCacheIfRevision(ctx, cacheKey, "fresh", currentRevision, time.Minute)
	if err != nil || !stored {
		t.Fatalf("当前代数的数据应允许写入，stored=%v err=%v", stored, err)
	}
}

func TestDictCacheRejectsStaleWriteBack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cacheKey := redisx.SysDictKey(fmt.Sprintf("%srevision_%d", testPrefix, time.Now().UnixNano()))
	trackRedisKey(cacheKey)
	if err := redisx.InvalidateDictCache(ctx, cacheKey); err != nil {
		t.Fatalf("初始化字典缓存失败：%v", err)
	}

	oldRevision, err := redisx.DictCacheRevision(ctx)
	if err != nil {
		t.Fatalf("读取旧字典代数失败：%v", err)
	}
	if err := redisx.InvalidateDictCache(ctx, cacheKey); err != nil {
		t.Fatalf("模拟并发字典更新失败：%v", err)
	}

	stored, err := redisx.StoreDictCacheIfRevision(ctx, cacheKey, []byte(`[{"dictValue":"stale"}]`),
		oldRevision, time.Minute)
	if err != nil {
		t.Fatalf("尝试写回旧字典值失败：%v", err)
	}
	if stored {
		t.Fatal("字典更新后，旧数据库快照不应重新写入缓存")
	}
	if _, err := redisx.C().Get(ctx, cacheKey).Result(); !redisx.IsNil(err) {
		t.Fatalf("旧字典值不应出现在缓存中，实际错误=%v", err)
	}

	currentRevision, err := redisx.DictCacheRevision(ctx)
	if err != nil {
		t.Fatalf("读取新字典代数失败：%v", err)
	}
	stored, err = redisx.StoreDictCacheIfRevision(ctx, cacheKey, []byte(`[{"dictValue":"fresh"}]`),
		currentRevision, time.Minute)
	if err != nil || !stored {
		t.Fatalf("当前字典代数的数据应允许写入，stored=%v err=%v", stored, err)
	}
}

func TestDictCacheMetadataIsNotManageableData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisx.InvalidateDictCache(ctx); err != nil {
		t.Fatalf("准备字典缓存代数失败：%v", err)
	}

	const metadataKey = redisx.KeySysDict + "__ruoyi_go_revision__"
	keys, err := service.CacheKeys(ctx, redisx.KeySysDict)
	if err != nil {
		t.Fatalf("列出字典缓存失败：%v", err)
	}
	for _, key := range keys {
		if key == metadataKey {
			t.Fatal("内部字典缓存代数不能出现在缓存监控列表")
		}
	}
	if _, err := service.CacheValue(ctx, redisx.KeySysDict, metadataKey); err == nil {
		t.Fatal("内部字典缓存代数不能通过缓存监控读取")
	}
	if err := service.ClearCacheByKey(ctx, metadataKey); err == nil {
		t.Fatal("内部字典缓存代数不能被单 key 清理")
	}
}

func TestRepeatSubmitCheckIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := redisx.RepeatSubmitKey(fmt.Sprintf("%satomic_%d", testPrefix, time.Now().UnixNano()))
	trackRedisKey(key)
	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var accepted atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			repeated, err := redisx.CheckRepeatSubmit(ctx, key, "same-fingerprint", time.Minute)
			if err != nil {
				errs <- err
				return
			}
			if !repeated {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("原子防重判定失败：%v", err)
	}
	if got := accepted.Load(); got != 1 {
		t.Fatalf("32 个相同并发请求应只放行 1 个，实际放行 %d 个", got)
	}
}
