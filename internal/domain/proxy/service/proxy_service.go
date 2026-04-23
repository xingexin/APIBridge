package biz

import (
	"context"
	"io"
	"net/http"

	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/domain/proxy/repository"
	"GPTBridge/internal/infra/logging"
	"go.uber.org/zap"
)

// ProxyService 负责调用 Rust 服务。
type ProxyService struct {
	bridge repository.Bridge
	logger *zap.Logger
}

// NewProxyService 创建 ProxyService。
func NewProxyService(bridge repository.Bridge, logger *zap.Logger) *ProxyService {
	return &ProxyService{bridge: bridge, logger: logger}
}

// Forward 调用当前配置的上游服务。
func (s *ProxyService) Forward(ctx context.Context, req entity.ProxyRequest) (*http.Response, error) {
	logging.WithContext(s.logger, ctx).Debug("调用上游服务",
		zap.String("operation", req.Operation),
		zap.String("method", req.Method),
		zap.String("path", req.Path),
		zap.Int("payload_bytes", len(req.Payload)),
	)
	return s.bridge.Forward(ctx, req)
}

// UploadFile 调用 Rust 的文件上传接口。
func (s *ProxyService) UploadFile(ctx context.Context, filename string, contentType string, content io.Reader, purpose string, headers http.Header) (entity.FileUploadResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("调用文件上传接口",
		zap.String("filename", filename),
		zap.String("content_type", contentType),
		zap.String("purpose", purpose),
	)
	return s.bridge.UploadFile(ctx, filename, contentType, content, purpose, headers)
}

// Models 获取 Rust 返回的模型列表。
func (s *ProxyService) Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("获取模型列表")
	return s.bridge.Models(ctx, headers)
}

// Health 获取 Rust 服务的健康状态。
func (s *ProxyService) Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error) {
	logging.WithContext(s.logger, ctx).Debug("获取健康状态", zap.String("account_id", accountID))
	return s.bridge.Health(ctx, accountID, headers)
}
