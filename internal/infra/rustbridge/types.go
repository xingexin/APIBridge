package rustbridge

import "time"

// Config 定义 Rust RPC 客户端的连接配置。
type Config struct {
	Addr    string
	Timeout time.Duration
}
