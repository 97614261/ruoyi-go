package service

import (
	"strings"
	"testing"

	"ruoyi-go/internal/config"
)

func TestPODQwenServerCredentialsAndPaidGuard(t *testing.T) {
	const env = "POD_QWEN_TEST_KEY"
	const key = "private-local-fixture-key"
	c := &config.Config{POD: config.PODConfig{
		StorageRoot: t.TempDir(),
		Execution:   config.PODExecutionConfig{Mode: "qwen"},
		Qwen:        config.PODQwenConfig{BaseURL: "https://dashscope.aliyuncs.com", APIKeyEnv: env, AllowPaidCalls: true, BudgetFen: 100},
	}}
	t.Setenv(env, "")
	if _, err := NewPODQwenExecutor(c); err == nil {
		t.Fatal("missing server key accepted")
	}
	t.Setenv(env, key)
	p, err := NewPODQwenExecutor(c)
	if err != nil {
		t.Fatal(err)
	}
	p.Images.CloseIdleConnections()
	c.POD.Qwen.APIKey = key
	t.Setenv(env, "bad")
	inline, err := NewPODQwenExecutor(c)
	if err != nil {
		t.Fatal("inline key should take priority over the environment")
	}
	inline.Images.CloseIdleConnections()
	c.POD.Qwen.APIKey = "bad"
	t.Setenv(env, key)
	if _, err := NewPODQwenExecutor(c); err == nil || strings.Contains(err.Error(), key) {
		t.Fatal("invalid inline key must fail rather than silently use the environment")
	}
	c.POD.Qwen.APIKey = ""
	for _, mutate := range []func(*config.Config){
		func(c *config.Config) { c.POD.Execution.Mode = "simulation" },
		func(c *config.Config) { c.POD.Qwen.AllowPaidCalls = false },
		func(c *config.Config) { c.POD.Qwen.BudgetFen = 0 },
		func(c *config.Config) { c.POD.Qwen.BudgetFen = 5001 },
		func(c *config.Config) { c.POD.Qwen.BaseURL = "https://example.com" },
	} {
		copy := *c
		mutate(&copy)
		_, err := NewPODQwenExecutor(&copy)
		if err == nil || strings.Contains(err.Error(), key) {
			t.Fatal("unsafe paid configuration accepted or credential exposed")
		}
	}
}
