package main

import (
	"net/http"
	"os"
	"time"

	proxyservice "GPTBridge/internal/domain/proxy/service"
	"GPTBridge/internal/handler"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/rustbridge"

	"go.uber.org/zap"
)

// main 启动代理服务并挂载 Rust 桥接客户端。
func main() {
	bridgeBaseURL := getenv("RUST_BRIDGE_BASE_URL", "http://127.0.0.1:8081")
	listenAddr := getenv("LISTEN_ADDR", ":8080")

	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	bridgeClient := rustbridge.NewClient(rustbridge.Config{
		BaseURL: bridgeBaseURL,
		Timeout: 60 * time.Second,
	}, logger)

	proxyService := proxyservice.NewProxyService(bridgeClient, logger)
	httpHandler := handler.NewRouter(proxyService, logger)

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("代理服务启动",
		zap.String("listen_addr", listenAddr),
		zap.String("rust_bridge", bridgeBaseURL),
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("代理服务启动失败", zap.Error(err))
	}
}

// getenv 优先读取环境变量，缺省时返回兜底值。
func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
