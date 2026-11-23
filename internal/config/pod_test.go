package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestPODConfigurationIsolation(t *testing.T) {
	root := t.TempDir()
	valid := func() Config {
		return Config{
			POD:   PODConfig{Enabled: true, InstanceID: "pod-local", DatabaseName: "pod_studio_test", RedisDB: 14, StorageRoot: filepath.Join(root, "private")},
			MySQL: MySQLConfig{DSN: "tester:secret@tcp(127.0.0.1:3306)/pod_studio_test"},
			Redis: RedisConfig{DB: 14}, JWT: JWTConfig{Secret: strings.Repeat("x", 48)},
			Upload: UploadConfig{Path: filepath.Join(root, "public")},
		}
	}
	if err := ValidatePOD(ptrConfig(valid())); err != nil {
		t.Fatal(err)
	}
	t.Run("managed_requires_budget_without_legacy_key", func(t *testing.T) {
		c := valid()
		c.POD.Execution.Mode = "managed"
		c.POD.Qwen = PODQwenConfig{AllowPaidCalls: true, BudgetFen: 100}
		if err := ValidatePOD(&c); err != nil {
			t.Fatal(err)
		}
		c.POD.Qwen.AllowPaidCalls = false
		if err := ValidatePOD(&c); err == nil {
			t.Fatal("managed accepted implicit paid calls")
		}
	})
	t.Run("qwen_requires_explicit_paid_budget_and_official_origin", func(t *testing.T) {
		qwen := valid()
		qwen.POD.Execution.Mode = "qwen"
		qwen.POD.Qwen = PODQwenConfig{BaseURL: "https://dashscope.aliyuncs.com", APIKeyEnv: "DASHSCOPE_API_KEY", AllowPaidCalls: true, BudgetFen: 100}
		if err := ValidatePOD(&qwen); err != nil {
			t.Fatal(err)
		}
		workspace := qwen
		workspace.POD.Qwen.BaseURL = "https://workspace-123.cn-beijing.maas.aliyuncs.com"
		if err := ValidatePOD(&workspace); err != nil {
			t.Fatal(err)
		}
		inline := qwen
		inline.POD.Qwen.APIKey = "private-local-fixture-key"
		inline.POD.Qwen.APIKeyEnv = ""
		if err := ValidatePOD(&inline); err != nil {
			t.Fatal("inline server key should not require an environment variable name")
		}
		for _, key := range []string{"short", strings.Repeat("x", 513), "secret-value\ninvalid", "secret-value invalid"} {
			inline.POD.Qwen.APIKey = key
			if err := ValidatePOD(&inline); err == nil || strings.Contains(err.Error(), key) {
				t.Fatal("invalid inline credential accepted or exposed")
			}
		}
		cases := []struct {
			name string
			edit func(*PODQwenConfig)
		}{
			{"paid switch missing", func(q *PODQwenConfig) { q.AllowPaidCalls = false }},
			{"budget missing", func(q *PODQwenConfig) { q.BudgetFen = 0 }},
			{"budget negative", func(q *PODQwenConfig) { q.BudgetFen = -1 }},
			{"budget excessive", func(q *PODQwenConfig) { q.BudgetFen = 5001 }},
			{"environment missing", func(q *PODQwenConfig) { q.APIKeyEnv = "" }},
			{"environment malformed", func(q *PODQwenConfig) { q.APIKeyEnv = "secret\nvalue" }},
			{"http", func(q *PODQwenConfig) { q.BaseURL = "http://dashscope.aliyuncs.com" }},
			{"custom host", func(q *PODQwenConfig) { q.BaseURL = "https://example.com" }},
			{"lookalike host", func(q *PODQwenConfig) { q.BaseURL = "https://dashscope.aliyuncs.com.example.com" }},
			{"userinfo", func(q *PODQwenConfig) { q.BaseURL = "https://secret-value@dashscope.aliyuncs.com" }},
			{"port", func(q *PODQwenConfig) { q.BaseURL = "https://dashscope.aliyuncs.com:443" }},
			{"path", func(q *PODQwenConfig) { q.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1" }},
			{"query", func(q *PODQwenConfig) { q.BaseURL = "https://dashscope.aliyuncs.com?key=secret-value" }},
			{"fragment", func(q *PODQwenConfig) { q.BaseURL = "https://dashscope.aliyuncs.com#secret-value" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := qwen
				tc.edit(&c.POD.Qwen)
				err := ValidatePOD(&c)
				if err == nil || strings.Contains(err.Error(), "secret-value") {
					t.Fatal("unsafe Qwen configuration accepted or exposed")
				}
			})
		}
	})
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"not enabled", func(c *Config) { c.POD.Enabled = false }},
		{"legacy database", func(c *Config) { c.MySQL.DSN = "tester:secret@tcp(127.0.0.1:3306)/ry-vue" }},
		{"unsafe declared database", func(c *Config) { c.POD.DatabaseName = "ry-vue" }},
		{"multi statement", func(c *Config) { c.MySQL.DSN += "?multiStatements=true" }},
		{"default redis", func(c *Config) { c.POD.RedisDB = 0; c.Redis.DB = 0 }},
		{"redis mismatch", func(c *Config) { c.Redis.DB = 13 }},
		{"too many nodes", func(c *Config) { c.POD.MaxNodes = 101 }},
		{"negative edges", func(c *Config) { c.POD.MaxEdges = -1 }},
		{"oversized graph limit", func(c *Config) { c.POD.MaxGraphBytes = 9 << 20 }},
		{"oversized upload limit", func(c *Config) { c.POD.MaxUploadBytes = 21 << 20 }},
		{"negative upload limit", func(c *Config) { c.POD.MaxUploadBytes = -1 }},
		{"oversized image pixels", func(c *Config) { c.POD.MaxImagePixels = 40_000_001 }},
		{"weak secret", func(c *Config) { c.JWT.Secret = "secret" }},
		{"public storage", func(c *Config) { c.POD.StorageRoot = filepath.Join(c.Upload.Path, "pod") }},
		{"public nested in private", func(c *Config) { c.Upload.Path = filepath.Join(c.POD.StorageRoot, "public") }},
		{"relative storage", func(c *Config) { c.POD.StorageRoot = "private" }},
		{"relative model keyring", func(c *Config) { c.POD.Catalog.KeyringFile = "private.json" }},
		{"public model keyring", func(c *Config) { c.POD.Catalog.KeyringFile = filepath.Join(c.Upload.Path, "keyring.json") }},
		{"private asset model keyring", func(c *Config) { c.POD.Catalog.KeyringFile = filepath.Join(c.POD.StorageRoot, "keyring.json") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			err := ValidatePOD(&c)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if strings.Contains(err.Error(), "tester:") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func ptrConfig(c Config) *Config { return &c }

func TestPODQwenInlineKeyLoadsWithoutJSONExposure(t *testing.T) {
	const key = "private-yaml-fixture-key"
	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)
	if err := v.ReadConfig(strings.NewReader("pod:\n  qwen:\n    apiKey: \"" + key + "\"\n")); err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil || c.POD.Qwen.APIKey != key {
		t.Fatal("inline key was not loaded from YAML")
	}
	data, err := json.Marshal(c)
	if err != nil || strings.Contains(string(data), key) {
		t.Fatal("inline key must not appear in JSON configuration output")
	}
}
