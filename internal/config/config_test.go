package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
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

func TestMySQLIdlePoolDefaultMatchesDeploymentConfig(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	const want = 20
	if got := v.GetInt("mysql.maxIdleConns"); got != want {
		t.Fatalf("mysql.maxIdleConns 默认值=%d，期望 %d", got, want)
	}

	deployment := viper.New()
	deployment.SetConfigFile("../../configs/application.yml")
	if err := deployment.ReadInConfig(); err != nil {
		t.Fatalf("读取部署配置失败：%v", err)
	}
	if got := deployment.GetInt("mysql.maxIdleConns"); got != want {
		t.Fatalf("部署配置 mysql.maxIdleConns=%d，期望与默认值一致为 %d", got, want)
	}
}

func TestRedisPoolDefaultMatchesDeploymentConfig(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	const want = 32
	if got := v.GetInt("redis.poolSize"); got != want {
		t.Fatalf("redis.poolSize 默认值=%d，期望 %d", got, want)
	}

	deployment := viper.New()
	deployment.SetConfigFile("../../configs/application.yml")
	if err := deployment.ReadInConfig(); err != nil {
		t.Fatalf("读取部署配置失败：%v", err)
	}
	if got := deployment.GetInt("redis.poolSize"); got != want {
		t.Fatalf("部署配置 redis.poolSize=%d，期望与默认值一致为 %d", got, want)
	}
}

func TestValidateRejectsUnsafeLimitsAndProxy(t *testing.T) {
	valid := Config{
		Server: ServerConfig{Port: 8080, Mode: "release", ReadTimeout: time.Second, WriteTimeout: time.Second,
			ShutdownTimeout: time.Second, RequestTimeout: time.Second, MaxRequestBodyMB: 1,
			SlowRequestThreshold: time.Millisecond},
		MySQL: MySQLConfig{DSN: "u:p@tcp(localhost:3306)/db", MaxOpenConns: 2, MaxIdleConns: 1,
			ConnMaxLifetime: time.Minute, SlowThreshold: time.Millisecond, ConnectTimeout: time.Second,
			ReadTimeout: time.Second, WriteTimeout: time.Second},
		Redis: RedisConfig{Addr: "localhost:6379", PoolSize: 1},
		JWT: JWTConfig{Secret: "secret", ExpireTime: time.Hour, RefreshWindow: time.Minute,
			Header: "Authorization"},
		Captcha: CaptchaConfig{Type: "math"},
		Upload:  UploadConfig{Path: "uploads", URLPrefix: "/profile", MaxSizeMB: 10, MaxRequestSizeMB: 20},
		Log:     LogConfig{Level: "info"},
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

	invalidPort := valid
	invalidPort.Server.Port = 70000
	if err := invalidPort.validate(); err == nil || !strings.Contains(err.Error(), "server.port") {
		t.Fatalf("invalid port should fail: %v", err)
	}

	invalidMode := valid
	invalidMode.Server.Mode = "production"
	if err := invalidMode.validate(); err == nil || !strings.Contains(err.Error(), "server.mode") {
		t.Fatalf("invalid mode should fail: %v", err)
	}

	invalidRedisDB := valid
	invalidRedisDB.Redis.DB = -1
	if err := invalidRedisDB.validate(); err == nil || !strings.Contains(err.Error(), "redis.db") {
		t.Fatalf("negative Redis DB should fail: %v", err)
	}

	mixedOrigins := valid
	mixedOrigins.Server.AllowedOrigins = []string{"*", "https://example.test"}
	if err := mixedOrigins.validate(); err == nil || !strings.Contains(err.Error(), "不能与具体来源混用") {
		t.Fatalf("mixed wildcard origins should fail: %v", err)
	}

	invalidOrigin := valid
	invalidOrigin.Server.AllowedOrigins = []string{"https://example.test/path"}
	if err := invalidOrigin.validate(); err == nil || !strings.Contains(err.Error(), "allowedOrigins") {
		t.Fatalf("origin with path should fail: %v", err)
	}

	invalidHeader := valid
	invalidHeader.JWT.Header = "X-Token"
	if err := invalidHeader.validate(); err == nil || !strings.Contains(err.Error(), "jwt.header") {
		t.Fatalf("未使用的 JWT header 应被拒绝: %v", err)
	}

	invalidCaptcha := valid
	invalidCaptcha.Captcha.Type = "text"
	if err := invalidCaptcha.validate(); err == nil || !strings.Contains(err.Error(), "captcha.type") {
		t.Fatalf("非法验证码类型应被拒绝: %v", err)
	}
}
