package logging

import (
	"context"

	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
)

// NewLogger 创建 zap 日志实例。
func NewLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Encoding = "json"
	return config.Build()
}

// WithContext 将上下文中的 traceID 附加到日志中。
func WithContext(logger *zap.Logger, ctx context.Context) *zap.Logger {
	traceID := trace.TraceIDFromContext(ctx)
	if traceID == "" {
		return logger
	}
	return logger.With(zap.String("trace_id", traceID))
}
