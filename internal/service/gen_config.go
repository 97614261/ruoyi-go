package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"ruoyi-go/internal/config"
)

var (
	generatorConfigMu sync.RWMutex
	generatorConfig   = config.GenConfig{
		Author: "ruoyi", PackageName: "ruoyi-go", AutoRemovePre: true,
		TablePrefixes: []string{"sys_"}, OutputRoot: ".",
	}
)

func InitGenerator(cfg config.GenConfig) error {
	root, err := filepath.Abs(cfg.OutputRoot)
	if err != nil {
		return fmt.Errorf("解析代码生成根目录失败: %w", err)
	}
	cfg.OutputRoot = filepath.Clean(root)
	generatorConfigMu.Lock()
	generatorConfig = cfg
	generatorConfigMu.Unlock()
	return nil
}

func currentGeneratorConfig() config.GenConfig {
	generatorConfigMu.RLock()
	defer generatorConfigMu.RUnlock()
	cfg := generatorConfig
	cfg.TablePrefixes = append([]string(nil), generatorConfig.TablePrefixes...)
	return cfg
}

func removeConfiguredTablePrefix(tableName string) string {
	cfg := currentGeneratorConfig()
	if !cfg.AutoRemovePre {
		return tableName
	}
	for _, raw := range cfg.TablePrefixes {
		prefix := strings.TrimSpace(raw)
		if prefix != "" && strings.HasPrefix(tableName, prefix) {
			return strings.TrimPrefix(tableName, prefix)
		}
	}
	return tableName
}
