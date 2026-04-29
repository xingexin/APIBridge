package main

import (
	"context"
	"net/http"
	"time"

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/biz/proxygateway"
	billingrepository "GPTBridge/internal/domain/billing/repository"
	proxyrepository "GPTBridge/internal/domain/proxy/repository"
	proxyservice "GPTBridge/internal/domain/proxy/service"
	upstreamrepository "GPTBridge/internal/domain/upstream/repository"
	userrepository "GPTBridge/internal/domain/user/repository"
	userservice "GPTBridge/internal/domain/user/service"
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
	if err := billingrepository.AutoMigrate(db); err != nil {
		logger.Fatal("计费表迁移失败", zap.Error(err))
	}
	if err := upstreamrepository.AutoMigrate(db); err != nil {
		logger.Fatal("上游池表迁移失败", zap.Error(err))
	}
	if err := userrepository.AutoMigrate(db); err != nil {
		logger.Fatal("用户表迁移失败", zap.Error(err))
	}
	userRepository := userrepository.NewGormUserRepository(db)
	if err := userRepository.SeedUsers(context.Background(), cfg.Auth.SeedUsers); err != nil {
		logger.Fatal("初始化用户失败", zap.Error(err))
	}
	billingRepository := billingrepository.NewGormBillingRepository(db)
	if err := billingRepository.SeedAccounts(context.Background(), cfg.Billing.SeedAccounts, cfg.Billing.DefaultPeriodDays); err != nil {
		logger.Fatal("初始化计费账号失败", zap.Error(err))
	}
	upstreamRepository := upstreamrepository.NewGormUpstreamRepository(db)
	if err := upstreamRepository.SeedDefault(context.Background(), cfg); err != nil {
		logger.Fatal("初始化上游帐号池失败", zap.Error(err))
	}

	proxyService := newProxyService(cfg, logger)
	authService := userservice.NewAuthService(cfg.Auth, userRepository)
	gateway := proxygateway.NewGateway(db, cfg, proxyService, logger)
	httpHandler := handler.NewRouter(gateway, authService, logger)

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

// newProxyService 注册 normal/rust provider，具体使用哪个由 upstream route 决定。
func newProxyService(cfg config.Config, logger *zap.Logger) *proxyservice.ProxyService {
	logger.Info("注册上游 provider",
		zap.String("normal_base_url", cfg.OpenAI.BaseURL),
		zap.String("rust_grpc_addr", cfg.Rust.GRPCAddr),
	)
	providers := map[string]proxyrepository.Forwarder{
		contracts.SourceTypeNormal: normalapi.NewClient(normalapi.Config{
			BaseURL: cfg.OpenAI.BaseURL,
			APIKey:  cfg.OpenAI.APIKey,
			Timeout: 60 * time.Second,
		}, logger),
		contracts.SourceTypeRust: rustbridge.NewClient(rustbridge.Config{
			Addr:    cfg.Rust.GRPCAddr,
			Timeout: 60 * time.Second,
		}, logger),
	}
	return proxyservice.NewProxyService(providers, logger)
}
