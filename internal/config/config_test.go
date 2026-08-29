package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadOperationalLimits(t *testing.T) {
	cfg, err := Load("../../configs/application.yml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.RequestTimeout <= 0 || cfg.Upload.MaxRequestSizeMB < cfg.Upload.MaxSizeMB ||
		cfg.MySQL.ConnectTimeout <= 0 || cfg.MySQL.ReadTimeout <= 0 || cfg.MySQL.WriteTimeout <= 0 {
		t.Fatalf("operational limits were not loaded: %#v", cfg)
	}
}

func TestValidateRejectsUnsafeLimitsAndProxy(t *testing.T) {
	valid := Config{
		Server: ServerConfig{ReadTimeout: time.Second, WriteTimeout: time.Second,
			ShutdownTimeout: time.Second, RequestTimeout: time.Second, MaxRequestBodyMB: 1,
			SlowRequestThreshold: time.Millisecond},
		MySQL: MySQLConfig{DSN: "u:p@tcp(localhost:3306)/db", MaxOpenConns: 2, MaxIdleConns: 1,
			ConnMaxLifetime: time.Minute, SlowThreshold: time.Millisecond, ConnectTimeout: time.Second,
			ReadTimeout: time.Second, WriteTimeout: time.Second},
		Redis:  RedisConfig{Addr: "localhost:6379", PoolSize: 1},
		JWT:    JWTConfig{Secret: "secret", ExpireTime: time.Hour, RefreshWindow: time.Minute},
		Upload: UploadConfig{Path: "uploads", URLPrefix: "/profile", MaxSizeMB: 10, MaxRequestSizeMB: 20},
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	invalidUpload := valid
	invalidUpload.Upload.MaxRequestSizeMB = 5
	if err := invalidUpload.validate(); err == nil || !strings.Contains(err.Error(), "maxRequestSizeMB") {
		t.Fatalf("unsafe multipart limits should fail: %v", err)
	}

	invalidProxy := valid
	invalidProxy.Server.TrustedProxies = []string{"not-an-ip"}
	if err := invalidProxy.validate(); err == nil || !strings.Contains(err.Error(), "trustedProxies") {
		t.Fatalf("invalid trusted proxy should fail: %v", err)
	}
}
