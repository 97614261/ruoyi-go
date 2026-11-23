package config

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// PODConfig is explicit: inherited RuoYi configuration cannot opt in by accident.
type PODConfig struct {
	Enabled        bool               `mapstructure:"enabled"`
	InstanceID     string             `mapstructure:"instanceId"`
	DatabaseName   string             `mapstructure:"databaseName"`
	RedisDB        int                `mapstructure:"redisDB"`
	StorageRoot    string             `mapstructure:"storageRoot"`
	MaxNodes       int                `mapstructure:"maxNodes"`
	MaxEdges       int                `mapstructure:"maxEdges"`
	MaxGraphBytes  int                `mapstructure:"maxGraphBytes"`
	MaxUploadBytes int64              `mapstructure:"maxUploadBytes"`
	MaxImagePixels int64              `mapstructure:"maxImagePixels"`
	Execution      PODExecutionConfig `mapstructure:"execution"`
	Qwen           PODQwenConfig      `mapstructure:"qwen"`
	Catalog        PODCatalogConfig   `mapstructure:"catalog"`
}

type PODCatalogConfig struct {
	KeyringFile    string   `mapstructure:"keyringFile"`
	CredentialEnvs []string `mapstructure:"credentialEnvs"`
}

type PODQwenConfig struct {
	BaseURL        string `mapstructure:"baseURL"`
	APIKey         string `mapstructure:"apiKey" json:"-"`
	APIKeyEnv      string `mapstructure:"apiKeyEnv"`
	AllowPaidCalls bool   `mapstructure:"allowPaidCalls"`
	BudgetFen      int64  `mapstructure:"budgetFen"`
}

type PODExecutionConfig struct {
	Mode                string `mapstructure:"mode"`
	GlobalConcurrency   int    `mapstructure:"globalConcurrency"`
	UserConcurrency     int    `mapstructure:"userConcurrency"`
	ProviderConcurrency int    `mapstructure:"providerConcurrency"`
	MaxPending          int    `mapstructure:"maxPending"`
	MaxUserPending      int    `mapstructure:"maxUserPending"`
	InitialCredits      int64  `mapstructure:"initialCredits"`
	ImageAIURL          string `mapstructure:"imageAIURL"`
}

var podIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
var podDatabase = regexp.MustCompile(`^pod_studio_[a-z0-9_]{1,40}$`)

// ValidatePOD is called before connecting to any backing service.
// It deliberately returns no connection strings or parsed secret values.
func ValidatePOD(c *Config) error {
	e := c.POD.Execution
	if e.ImageAIURL != "" {
		u, err := url.Parse(e.ImageAIURL)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("本地图片 AI 服务必须是带端口的 http://127.0.0.1 根地址")
		}
	}
	if (e.Mode != "" && e.Mode != "disabled" && e.Mode != "simulation" && e.Mode != "qwen" && e.Mode != "managed") || e.GlobalConcurrency < 0 || e.GlobalConcurrency > 16 || e.UserConcurrency < 0 || e.UserConcurrency > 8 || e.ProviderConcurrency < 0 || e.ProviderConcurrency > 16 || e.MaxPending < 0 || e.MaxPending > 1000 || e.MaxUserPending < 0 || e.MaxUserPending > 100 || e.InitialCredits < 0 || e.InitialCredits > 1_000_000 {
		return errors.New("POD 执行配置无效，仅允许 disabled/simulation/qwen/managed 及有界并发和测试额度")
	}
	q := c.POD.Qwen
	keySourceValid := regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`).MatchString(q.APIKeyEnv)
	if q.APIKey != "" {
		keySourceValid = len(q.APIKey) >= 8 && len(q.APIKey) <= 512 && !strings.ContainsAny(q.APIKey, "\r\n \t")
	}
	if e.Mode == "managed" && (!q.AllowPaidCalls || q.BudgetFen < 1 || q.BudgetFen > 5000) {
		return errors.New("managed 需要明确允许调用和 1—5000 分累计成本预算")
	}
	if e.Mode == "qwen" && (!q.AllowPaidCalls || q.BudgetFen < 1 || q.BudgetFen > 5000 || !keySourceValid) {
		return errors.New("千问模式须显式允许付费调用、设置 1—5000 分累计预算及有效的服务端密钥配置")
	}
	if e.Mode == "qwen" {
		u, err := url.Parse(c.POD.Qwen.BaseURL)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Host != "dashscope.aliyuncs.com" && !regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}\.cn-beijing\.maas\.aliyuncs\.com$`).MatchString(u.Host)) {
			return errors.New("千问须使用北京地域官方 HTTPS 根域名，不允许路径、凭据、参数或自定义端口")
		}
	}
	if !c.POD.Enabled {
		return errors.New("POD 服务需要独立配置，请使用 configs/pod.local.yml；未连接数据库")
	}
	if !podIdentifier.MatchString(c.POD.InstanceID) || !podDatabase.MatchString(c.POD.DatabaseName) {
		return errors.New("pod.instanceId 或 pod.databaseName 不合法，数据库名必须以 pod_studio_ 开头")
	}
	if c.POD.MaxNodes < 0 || c.POD.MaxNodes > 100 || c.POD.MaxEdges < 0 || c.POD.MaxEdges > 300 || c.POD.MaxGraphBytes < 0 || c.POD.MaxGraphBytes > 8<<20 {
		return errors.New("POD 图限制超出范围，最多 100 节点、300 连线、8 MiB")
	}
	dsn, err := mysql.ParseDSN(c.MySQL.DSN)
	if c.POD.MaxUploadBytes < 0 || c.POD.MaxUploadBytes > 20<<20 || c.POD.MaxImagePixels < 0 || c.POD.MaxImagePixels > 40_000_000 {
		return errors.New("POD 图片限制超出范围，最多 20 MiB、4000 万像素")
	}
	if err != nil || dsn.DBName != c.POD.DatabaseName || dsn.MultiStatements {
		return errors.New("MySQL 必须指向指定 POD 数据库，且不能开启 multiStatements")
	}
	if c.POD.RedisDB < 1 || c.Redis.DB != c.POD.RedisDB {
		return errors.New("Redis 必须使用 pod.redisDB 指定的独立逻辑库，不能使用 DB 0")
	}
	if len(c.JWT.Secret) < 32 || strings.Contains(strings.ToLower(c.JWT.Secret), "change-me") {
		return errors.New("POD JWT 密钥必须是至少 32 字节的独立随机密钥")
	}
	if strings.TrimSpace(c.POD.StorageRoot) == "" || !filepath.IsAbs(c.POD.StorageRoot) {
		return errors.New("pod.storageRoot 必须是专用私有素材目录的绝对路径")
	}
	publicRoot, err := filepath.Abs(c.Upload.Path)
	if err != nil {
		return errors.New("无法解析公共上传目录")
	}
	privateRoot := filepath.Clean(c.POD.StorageRoot)
	if keyFile := c.POD.Catalog.KeyringFile; keyFile != "" && (!filepath.IsAbs(keyFile) || pathContains(privateRoot, keyFile) || pathContains(publicRoot, keyFile)) {
		return errors.New("模型主密钥文件必须使用素材和公共上传目录之外的绝对路径")
	}
	if pathContains(publicRoot, privateRoot) || pathContains(privateRoot, publicRoot) {
		return errors.New("POD 私有素材目录不能与 /profile 公共上传目录相同或互相包含")
	}
	return nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
