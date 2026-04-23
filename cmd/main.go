package main

import (
	"context"
	"net/http"
	"time"

	"GPTBridge/internal/domain/proxy/repository"
	proxyservice "GPTBridge/internal/domain/proxy/service"
	userrepository "GPTBridge/internal/domain/user/repository"
	userservice "GPTBridge/internal/domain/user/service"
	walletrepository "GPTBridge/internal/domain/wallet/repository"
	walletservice "GPTBridge/internal/domain/wallet/service"
	"GPTBridge/internal/handler"
	"GPTBridge/internal/infra/config"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/normalapi"
	"GPTBridge/internal/infra/rustbridge"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// main 启动代理服务并挂载 Rust 桥接客户端。
func main() {
	logger, err := logging.NewLogger()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("读取配置失败", zap.Error(err))
	}

	db, err := gorm.Open(mysql.Open(cfg.Database.DSN), &gorm.Config{})
	if err != nil {
		logger.Fatal("打开数据库失败", zap.Error(err))
	}
	if err := walletrepository.AutoMigrate(db); err != nil {
		logger.Fatal("数据库迁移失败", zap.Error(err))
	}
	if err := userrepository.AutoMigrate(db); err != nil {
		logger.Fatal("用户表迁移失败", zap.Error(err))
	}
	userRepository := userrepository.NewGormUserRepository(db)
	if err := userRepository.SeedUsers(context.Background(), cfg.Auth.SeedUsers); err != nil {
		logger.Fatal("初始化用户失败", zap.Error(err))
	}
	walletRepository := walletrepository.NewGormWalletRepository(db)
	if err := walletRepository.SeedAccounts(context.Background(), cfg.Billing.SeedAccounts); err != nil {
		logger.Fatal("初始化计费账号失败", zap.Error(err))
	}

	bridgeClient := newBridge(cfg, logger)
	authService := userservice.NewAuthService(cfg.Auth, userRepository)
	billingService := walletservice.NewBillingService(cfg.Billing, walletRepository, logger)

	proxyService := proxyservice.NewProxyService(bridgeClient, logger)
	httpHandler := handler.NewRouter(proxyService, billingService, authService, logger)

	server := &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("代理服务启动",
		zap.String("listen_addr", cfg.Server.ListenAddr),
		zap.String("upstream_mode", cfg.Upstream.Mode),
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("代理服务启动失败", zap.Error(err))
	}
}

// newBridge 根据配置选择 Rust 逆向链路或正常 API 链路。
func newBridge(cfg config.Config, logger *zap.Logger) repository.Bridge {
	switch cfg.Upstream.Mode {
	case "api", "normal", "openai":
		logger.Info("使用正常 API 上游", zap.String("base_url", cfg.OpenAI.BaseURL))
		return normalapi.NewClient(normalapi.Config{
			BaseURL: cfg.OpenAI.BaseURL,
			APIKey:  cfg.OpenAI.APIKey,
			Timeout: 60 * time.Second,
		}, logger)
	default:
		logger.Info("使用 Rust RPC 桥接上游", zap.String("addr", cfg.Rust.GRPCAddr))
		return rustbridge.NewClient(rustbridge.Config{
			Addr:    cfg.Rust.GRPCAddr,
			Timeout: 60 * time.Second,
		}, logger)
	}
}
