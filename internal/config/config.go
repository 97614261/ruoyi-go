// Package config 配置加载。[L0]
package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置总入口。
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	Redis   RedisConfig   `mapstructure:"redis"`
	JWT     JWTConfig     `mapstructure:"jwt"`
	Captcha CaptchaConfig `mapstructure:"captcha"`
	Upload  UploadConfig  `mapstructure:"upload"`
	Log     LogConfig     `mapstructure:"log"`
}

// UploadConfig 文件上传配置，对齐 Java 版的 ruoyi.profile。
type UploadConfig struct {
	// Path 本地存储根目录，头像放在 <Path>/avatar/ 下
	Path string `mapstructure:"path"`
	// URLPrefix 对外访问前缀，必须是 /profile —— 前端拼头像地址时写死了这个前缀
	URLPrefix string `mapstructure:"urlPrefix"`
	// MaxSizeMB 单文件大小上限
	MaxSizeMB int64 `mapstructure:"maxSizeMB"`
	// MaxRequestSizeMB multipart 请求总大小上限，必须不小于单文件上限。
	MaxRequestSizeMB int64 `mapstructure:"maxRequestSizeMB"`
}

// CaptchaConfig 验证码配置。
//
// 是否启用不在这里配，而是读 sys_config 的 sys.account.captchaEnabled，
// 与 Java 版保持一致（后台参数页面能直接改）。
type CaptchaConfig struct {
	// Type math 数字计算 / char 字符
	Type string `mapstructure:"type"`
}

type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	Mode            string        `mapstructure:"mode"`
	ReadTimeout     time.Duration `mapstructure:"readTimeout"`
	WriteTimeout    time.Duration `mapstructure:"writeTimeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdownTimeout"`
	// RequestTimeout 为请求上下文设置总 deadline，数据库查询继承该 deadline。
	RequestTimeout time.Duration `mapstructure:"requestTimeout"`
	// MaxRequestBodyMB 普通 JSON/form 请求体上限；multipart 使用 Upload 的单文件和总请求上限。
	MaxRequestBodyMB int64 `mapstructure:"maxRequestBodyMB"`
	// AllowedOrigins 跨域白名单，**默认为空即不放行任何跨域请求**。
	// 前端走 Vite 代理或 Nginx 反代时是同源的，根本用不到这个；
	// 真需要时在这里显式列出，不要图省事写 "*"。
	AllowedOrigins []string `mapstructure:"allowedOrigins"`
	// SlowRequestThreshold 超过该耗时的请求单独打 warn 日志。
	// 配得太小会把正常日志淹掉，太大就发现不了问题，500ms 是个务实的起点。
	SlowRequestThreshold time.Duration `mapstructure:"slowRequestThreshold"`
	// TrustedProxies 仅列反向代理地址或 CIDR；空列表表示不信任转发头。
	TrustedProxies []string `mapstructure:"trustedProxies"`
}

type MySQLConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
	SlowThreshold   time.Duration `mapstructure:"slowThreshold"`
	ConnectTimeout  time.Duration `mapstructure:"connectTimeout"`
	ReadTimeout     time.Duration `mapstructure:"readTimeout"`
	WriteTimeout    time.Duration `mapstructure:"writeTimeout"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"poolSize"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	// ExpireTime 会话有效期，即 Redis TTL
	ExpireTime time.Duration `mapstructure:"expireTime"`
	// RefreshWindow 剩余不足该值时自动续期
	RefreshWindow time.Duration `mapstructure:"refreshWindow"`
	Header        string        `mapstructure:"header"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Load 读取配置文件。
//
// 环境变量可覆盖，命名规则为大写下划线，如 MYSQL_DSN、JWT_SECRET。
// 生产环境的密钥和数据库密码应走环境变量，不要写进 yml。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()
	// 必须设置替换器，否则 AutomaticEnv 会去找名为 "MYSQL.DSN" 的环境变量，
	// 而这不是合法的变量名，等于环境变量覆盖完全失效。
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.readTimeout", "30s")
	v.SetDefault("server.writeTimeout", "60s")
	v.SetDefault("server.shutdownTimeout", "10s")
	v.SetDefault("server.requestTimeout", "60s")
	v.SetDefault("server.maxRequestBodyMB", 2)
	v.SetDefault("server.slowRequestThreshold", "500ms")

	v.SetDefault("mysql.maxOpenConns", 50)
	v.SetDefault("mysql.maxIdleConns", 10)
	v.SetDefault("mysql.connMaxLifetime", "1h")
	v.SetDefault("mysql.slowThreshold", "200ms")
	v.SetDefault("mysql.connectTimeout", "5s")
	v.SetDefault("mysql.readTimeout", "30s")
	v.SetDefault("mysql.writeTimeout", "30s")

	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.poolSize", 20)

	v.SetDefault("jwt.expireTime", "30m")
	v.SetDefault("jwt.refreshWindow", "20m")
	v.SetDefault("jwt.header", "Authorization")

	v.SetDefault("captcha.type", "math")

	v.SetDefault("upload.path", "./uploadPath")
	v.SetDefault("upload.urlPrefix", "/profile")
	v.SetDefault("upload.maxSizeMB", 10)
	v.SetDefault("upload.maxRequestSizeMB", 20)

	v.SetDefault("log.level", "info")
}

func (c *Config) validate() error {
	if c.Server.MaxRequestBodyMB <= 0 {
		return fmt.Errorf("server.maxRequestBodyMB 必须大于 0")
	}
	if c.Server.ReadTimeout <= 0 || c.Server.WriteTimeout <= 0 || c.Server.ShutdownTimeout <= 0 ||
		c.Server.RequestTimeout <= 0 || c.Server.SlowRequestThreshold <= 0 {
		return fmt.Errorf("server 的 readTimeout、writeTimeout、shutdownTimeout、requestTimeout 和 slowRequestThreshold 必须大于 0")
	}
	for _, proxy := range c.Server.TrustedProxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return fmt.Errorf("server.trustedProxies 包含非法地址 %q", proxy)
			}
		}
	}
	if c.MySQL.DSN == "" {
		return fmt.Errorf("mysql.dsn 不能为空")
	}
	if c.MySQL.MaxOpenConns <= 0 || c.MySQL.MaxIdleConns < 0 || c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns ||
		c.MySQL.ConnMaxLifetime <= 0 || c.MySQL.SlowThreshold <= 0 || c.MySQL.ConnectTimeout <= 0 ||
		c.MySQL.ReadTimeout <= 0 || c.MySQL.WriteTimeout <= 0 {
		return fmt.Errorf("mysql 连接池和超时配置不合法")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr 不能为空")
	}
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis.poolSize 必须大于 0")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.JWT.ExpireTime <= 0 || c.JWT.RefreshWindow <= 0 {
		return fmt.Errorf("jwt.expireTime 和 jwt.refreshWindow 必须大于 0")
	}
	if c.JWT.RefreshWindow >= c.JWT.ExpireTime {
		return fmt.Errorf("jwt.refreshWindow(%s) 必须小于 jwt.expireTime(%s)，否则每次请求都会续期",
			c.JWT.RefreshWindow, c.JWT.ExpireTime)
	}
	if c.Upload.MaxSizeMB <= 0 || c.Upload.MaxRequestSizeMB <= 0 {
		return fmt.Errorf("upload.maxSizeMB 和 upload.maxRequestSizeMB 必须大于 0")
	}
	if c.Upload.MaxRequestSizeMB < c.Upload.MaxSizeMB {
		return fmt.Errorf("upload.maxRequestSizeMB 不能小于 upload.maxSizeMB")
	}
	if strings.TrimSpace(c.Upload.Path) == "" || c.Upload.URLPrefix != "/profile" {
		return fmt.Errorf("upload.path 不能为空且 upload.urlPrefix 必须为 /profile")
	}
	return nil
}
