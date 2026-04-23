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
}

// ServerConfig 表示 HTTP 服务配置。
type ServerConfig struct {
	ListenAddr string `mapstructure:"listen_addr"`
}

// UpstreamConfig 表示上游选择配置。
type UpstreamConfig struct {
	Mode string `mapstructure:"mode"`
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
}
