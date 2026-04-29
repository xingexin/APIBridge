package biz

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/domain/proxy/repository"
	"GPTBridge/internal/infra/logging"
	"go.uber.org/zap"
)

// ProxyService 只负责按 route 调用对应 provider。
type ProxyService struct {
	providers map[string]repository.Forwarder
	logger    *zap.Logger
}

// NewProxyService 创建 ProxyService。
func NewProxyService(providers map[string]repository.Forwarder, logger *zap.Logger) *ProxyService {
	return &ProxyService{providers: providers, logger: logger}
}

// Forward 调用当前配置的上游服务。
func (s *ProxyService) Forward(ctx context.Context, route contracts.Route, req entity.ProxyRequest) (*http.Response, error) {
	logging.WithContext(s.logger, ctx).Debug("调用上游服务",
		zap.String("operation", req.Operation),
		zap.String("method", req.Method),
		zap.String("path", req.Path),
		zap.String("source_type", route.SourceType),
		zap.Uint("pool_id", route.PoolID),
		zap.Uint("upstream_account_id", route.UpstreamAccountID),
		zap.Int("payload_bytes", len(req.Payload)),
	)
	provider, ok := s.providers[route.SourceType]
	if !ok {
		return nil, fmt.Errorf("未找到上游 provider: %s", route.SourceType)
	}
	return provider.Forward(ctx, route, req)
}

// UploadFile 调用 Rust 的文件上传接口。
func (s *ProxyService) UploadFile(ctx context.Context, filename string, contentType string, content io.Reader, purpose string, headers http.Header) (entity.FileUploadResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("调用文件上传接口",
		zap.String("filename", filename),
		zap.String("content_type", contentType),
		zap.String("purpose", purpose),
	)
	return entity.FileUploadResponse{}, fmt.Errorf("文件上传应通过通用代理路由执行")
}

// Models 获取 Rust 返回的模型列表。
func (s *ProxyService) Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("获取模型列表")
	return nil, fmt.Errorf("模型列表应通过 gateway 按 route 转发")
}

// Health 获取 Rust 服务的健康状态。
func (s *ProxyService) Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("获取健康状态", zap.String("account_id", accountID))
	return entity.HealthResponse{Status: "ok", Message: "proxy service ready"}, nil
}
