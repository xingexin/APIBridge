package rustbridge

import "time"

// Config 定义 Rust 桥接客户端的连接配置。
type Config struct {
	BaseURL string
	Timeout time.Duration
}
