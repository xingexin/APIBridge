package normalapi

import "time"

// Config 定义正常 API 上游的连接配置。
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}
