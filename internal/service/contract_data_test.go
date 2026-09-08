package service

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

type contractIdentifierManifest struct {
	SourceRevision  string   `json:"sourceRevision"`
	ConfigKeys      []string `json:"configKeys"`
	DictTypes       []string `json:"dictTypes"`
	MenuPermissions []string `json:"menuPermissions"`
}

func loadContractIdentifierManifest(t *testing.T) contractIdentifierManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(projectRoot(t), "internal", "service", "testdata", "contract_identifiers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest contractIdentifierManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("解析契约标识 manifest 失败: %v", err)
	}
	if manifest.SourceRevision == "" || len(manifest.ConfigKeys) == 0 ||
		len(manifest.DictTypes) == 0 || len(manifest.MenuPermissions) == 0 {
		t.Fatal("契约标识 manifest 缺少来源或集合为空")
	}
	return manifest
}

func TestContractManifestMatchesGoSets(t *testing.T) {
	manifest := loadContractIdentifierManifest(t)
	assertSameStringSet(t, "manifest 配置键", stringSet(manifest.ConfigKeys...), contractConfigKeys)
	assertSameStringSet(t, "manifest 字典类型", stringSet(manifest.DictTypes...), contractDictTypes)
	assertSameStringSet(t, "manifest 菜单权限", stringSet(manifest.MenuPermissions...), contractMenuPermissions)
}

func TestContractConfigIdentifiersCannotMoveOrLoseBuiltinFlag(t *testing.T) {
	existing := &model.SysConfig{
		ConfigKey:   ConfigKeyCaptchaEnabled,
		ConfigValue: "true",
		ConfigType:  model.ConfigTypeBuiltin,
	}

	renamed := *existing
	renamed.ConfigKey = "custom.captcha"
	if err := validateConfigUpdate(existing, &renamed); err == nil || !strings.Contains(err.Error(), "不能改名") {
		t.Fatalf("契约参数改名应被拒绝: %v", err)
	}

	demoted := *existing
	demoted.ConfigType = model.ConfigTypeCustom
	if err := validateConfigUpdate(existing, &demoted); err == nil || !strings.Contains(err.Error(), "必须标记为系统内置") {
		t.Fatalf("内置参数降级应被拒绝: %v", err)
	}

	// Historical bad data must be repairable, but cannot remain custom after edit.
	historical := *existing
	historical.ConfigType = model.ConfigTypeCustom
	if err := validateConfigUpdate(&historical, &historical); err == nil || !strings.Contains(err.Error(), "必须标记为系统内置") {
		t.Fatalf("历史降级的契约参数应要求修复内置标记: %v", err)
	}
	repaired := historical
	repaired.ConfigType = model.ConfigTypeBuiltin
	if err := validateConfigUpdate(&historical, &repaired); err != nil {
		t.Fatalf("契约参数恢复内置标记应允许: %v", err)
	}
}

func TestContractPermissionsCoverAllGoRoutes(t *testing.T) {
	root := projectRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "internal", "router", "router.go"))
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`HasPermission\("([^"]+)"\)`).FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("未从路由提取到任何权限标识")
	}
	for _, match := range matches {
		if _, exists := contractMenuPermissions[match[1]]; !exists {
			t.Errorf("路由权限 %q 未登记到契约集合", match[1])
		}
	}
}

func TestContractConfigKeysMatchGoConstants(t *testing.T) {
	serviceRoot := filepath.Join(projectRoot(t), "internal", "service")
	extracted := stringSet()
	pattern := regexp.MustCompile(`ConfigKey[A-Za-z0-9_]*\s*=\s*"([^"]+)"`)
	err := filepath.WalkDir(serviceRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(raw), -1) {
			extracted[match[1]] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSameStringSet(t, "Go 配置键常量", contractConfigKeys, extracted)
}

func TestContractPermissionsMatchJavaBaselineSQL(t *testing.T) {
	root := projectRoot(t)
	javaRoot := filepath.Join(filepath.Dir(root), "RuoYi-Vue-master")
	if _, err := os.Stat(javaRoot); os.IsNotExist(err) {
		t.Skip("兄弟 Java 仓库不存在；Go 内置 manifest 仍已执行")
	} else if err != nil {
		t.Fatal(err)
	}
	sqlPath := filepath.Join(javaRoot, "sql", "ry_20260417.sql")
	raw, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatalf("Java 仓库存在但基线 SQL 不可读，请核对并更新 manifest 来源路径: %v", err)
	}
	extracted := stringSet()
	for _, match := range regexp.MustCompile(`'(system|monitor|tool):[^']+'`).FindAllString(string(raw), -1) {
		extracted[strings.Trim(match, "'")] = struct{}{}
	}
	manifest := loadContractIdentifierManifest(t)
	assertSameStringSet(t, "Java SQL 权限", stringSet(manifest.MenuPermissions...), extracted)
}

func TestContractDictTypesMatchVueUseDict(t *testing.T) {
	root := projectRoot(t)
	vueRepository := filepath.Join(filepath.Dir(root), "RuoYi-Vue3-master")
	if _, err := os.Stat(vueRepository); os.IsNotExist(err) {
		t.Skip("兄弟 Vue 仓库不存在；Go 内置 manifest 仍已执行")
	} else if err != nil {
		t.Fatal(err)
	}
	vueRoot := filepath.Join(vueRepository, "src")
	extracted := stringSet()
	callPattern := regexp.MustCompile(`useDict\(([^)]*)\)`)
	argPattern := regexp.MustCompile(`["']([^"']+)["']`)
	err := filepath.WalkDir(vueRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".vue" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, call := range callPattern.FindAllStringSubmatch(string(raw), -1) {
			for _, arg := range argPattern.FindAllStringSubmatch(call[1], -1) {
				extracted[arg[1]] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(extracted) == 0 {
		t.Fatal("Vue 仓库存在但未提取到任何 useDict() 字典类型，请核对源码路径或提取规则")
	}
	manifest := loadContractIdentifierManifest(t)
	assertSameStringSet(t, "Vue useDict 字典类型", stringSet(manifest.DictTypes...), extracted)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func assertSameStringSet(t *testing.T, name string, want, got map[string]struct{}) {
	t.Helper()
	missing := make([]string, 0)
	extra := make([]string, 0)
	for value := range want {
		if _, exists := got[value]; !exists {
			missing = append(missing, value)
		}
	}
	for value := range got {
		if _, exists := want[value]; !exists {
			extra = append(extra, value)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("%s 与契约集合不一致: 缺少=%v 多余=%v", name, missing, extra)
	}
}

func TestContractConfigValuesAreValidated(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{ConfigKeyCaptchaEnabled, "ture"},
		{ConfigKeyRegisterUser, "yes"},
		{ConfigKeyInitPasswordModify, "2"},
		{ConfigKeyPasswordValidateDays, "365"},
		{ConfigKeyAccountChrtype, "5"},
		{ConfigKeyInitPassword, "1234"},
	}
	for _, tc := range tests {
		if err := validateContractConfigValue(tc.key, tc.value); err == nil {
			t.Errorf("非法契约参数值未被拒绝: key=%s value=%q", tc.key, tc.value)
		}
	}
}

func TestContractDictionaryAndPermissionSets(t *testing.T) {
	if !isContractDictType("sys_normal_disable") || isContractDictType("custom_status") {
		t.Fatal("契约字典类型集合不正确")
	}
	if !containsContractMenuPermission("custom:view, system:user:list") {
		t.Fatal("逗号分隔的契约权限未被识别")
	}
	if containsContractMenuPermission("custom:view") {
		t.Fatal("自定义权限不应被冻结")
	}
}
