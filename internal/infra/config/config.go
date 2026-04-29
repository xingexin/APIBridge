package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Config 表示服务启动所需配置。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Upstream UpstreamConfig `mapstructure:"upstream"`
	Rust     RustConfig     `mapstructure:"rust"`
	OpenAI   OpenAIConfig   `mapstructure:"openai"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Billing  BillingConfig  `mapstructure:"billing"`
}

// ServerConfig 表示 HTTP 服务配置。
type ServerConfig struct {
	ListenAddr string `mapstructure:"listen_addr"`
}

// UpstreamConfig 表示上游选择配置。
type UpstreamConfig struct {
	Mode  string               `mapstructure:"mode"`
	Pools []UpstreamPoolConfig `mapstructure:"pools"`
}

// UpstreamPoolConfig 表示启动时初始化的上游帐号池。
type UpstreamPoolConfig struct {
	PoolID              string                     `mapstructure:"pool_id"`
	Name                string                     `mapstructure:"name"`
	SourceType          string                     `mapstructure:"source_type"`
	BaseURL             string                     `mapstructure:"base_url"`
	RustGRPCAddr        string                     `mapstructure:"rust_grpc_addr"`
	MonthlyQuotaCredits float64                    `mapstructure:"monthly_quota_credits"`
	OversellPercent     float64                    `mapstructure:"oversell_percent"`
	ExhaustThreshold    float64                    `mapstructure:"exhaust_threshold"`
	DisabledByAdmin     bool                       `mapstructure:"disabled_by_admin"`
	APIAccounts         []UpstreamAPIAccountConfig `mapstructure:"api_accounts"`
}

// UpstreamAPIAccountConfig 表示帐号池内的上游 API 帐号。
type UpstreamAPIAccountConfig struct {
	AccountRef          string  `mapstructure:"account_ref"`
	APIKey              string  `mapstructure:"api_key"`
	MonthlyQuotaCredits float64 `mapstructure:"monthly_quota_credits"`
	Priority            int     `mapstructure:"priority"`
	DisabledByAdmin     bool    `mapstructure:"disabled_by_admin"`
}

// RustConfig 表示 Rust RPC 配置。
type RustConfig struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
}

// OpenAIConfig 表示正常 API 上游配置。
type OpenAIConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
}

// DatabaseConfig 表示数据库配置。
type DatabaseConfig struct {
	DSN string `mapstructure:"dsn"`
}

// AuthConfig 表示登录和 session 配置。
type AuthConfig struct {
	SessionCookieName string           `mapstructure:"session_cookie_name"`
	SessionTTLHours   int              `mapstructure:"session_ttl_hours"`
	CookieSecure      bool             `mapstructure:"cookie_secure"`
	SeedUsers         []SeedUserConfig `mapstructure:"seed_users"`
}

// SeedUserConfig 表示启动时需要初始化的用户。
type SeedUserConfig struct {
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	DisplayName string `mapstructure:"display_name"`
	Role        string `mapstructure:"role"`
	Enabled     bool   `mapstructure:"enabled"`
}

// BillingConfig 表示计费配置。
type BillingConfig struct {
	Enabled                 bool                    `mapstructure:"enabled"`
	RequireAPIKey           bool                    `mapstructure:"require_api_key"`
	DefaultModel            string                  `mapstructure:"default_model"`
	DefaultInputPricePer1K  float64                 `mapstructure:"default_input_price_per_1k"`
	DefaultOutputPricePer1K float64                 `mapstructure:"default_output_price_per_1k"`
	RequestPrice            float64                 `mapstructure:"request_price"`
	DefaultPeriodDays       int                     `mapstructure:"default_period_days"`
	SeedAccounts            []BillingAccountConfig  `mapstructure:"seed_accounts"`
	Models                  map[string]ModelPricing `mapstructure:"models"`
}

// BillingAccountConfig 表示需要初始化的计费账号。
type BillingAccountConfig struct {
	AccountID string                `mapstructure:"account_id"`
	Name      string                `mapstructure:"name"`
	Balance   float64               `mapstructure:"balance"`
	Enabled   bool                  `mapstructure:"enabled"`
	APIKeys   []BillingAPIKeyConfig `mapstructure:"api_keys"`
}

// BillingAPIKeyConfig 表示账号下的平台 API Key。
type BillingAPIKeyConfig struct {
	Key     string `mapstructure:"key"`
	Name    string `mapstructure:"name"`
	Enabled bool   `mapstructure:"enabled"`
}

// ModelPricing 表示模型计费单价。
type ModelPricing struct {
	InputPricePer1K  float64 `mapstructure:"input_price_per_1k"`
	OutputPricePer1K float64 `mapstructure:"output_price_per_1k"`
	RequestPrice     float64 `mapstructure:"request_price"`
}

// Load 读取配置文件，并允许环境变量覆盖。
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./cfg")
	v.AddConfigPath(".")

	setDefaults(v)

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var cfg Config
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return cfg, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, err
	}
	cfg.Upstream.Mode = strings.ToLower(cfg.Upstream.Mode)
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.listen_addr", ":8080")
	v.SetDefault("upstream.mode", "rust")
	v.SetDefault("rust.grpc_addr", "127.0.0.1:50051")
	v.SetDefault("openai.base_url", "https://api.openai.com")
	v.SetDefault("openai.api_key", "")
	v.SetDefault("database.dsn", "gptbridge:password@tcp(127.0.0.1:3306)/gptbridge?charset=utf8mb4&parseTime=True&loc=Local")
	v.SetDefault("auth.session_cookie_name", "gptbridge_session")
	v.SetDefault("auth.session_ttl_hours", 168)
	v.SetDefault("auth.cookie_secure", false)
	v.SetDefault("billing.enabled", true)
	v.SetDefault("billing.require_api_key", true)
	v.SetDefault("billing.default_model", "default")
	v.SetDefault("billing.default_input_price_per_1k", 0.001)
	v.SetDefault("billing.default_output_price_per_1k", 0.002)
	v.SetDefault("billing.request_price", 0)
	v.SetDefault("billing.default_period_days", 30)
}
