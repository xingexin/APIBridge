package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	// HeaderTraceID 是服务内外统一使用的链路标识请求头。
	HeaderTraceID = "X-Trace-Id"
	// HeaderRequestID 兼容上游或客户端可能已传入的请求标识。
	HeaderRequestID = "X-Request-Id"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

// WithTraceID 将 traceID 写入上下文。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext 从上下文中读取 traceID。
func TraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// EnsureTraceID 从请求头提取 traceID，不存在时生成新的。
func EnsureTraceID(header http.Header) string {
	if traceID := header.Get(HeaderTraceID); traceID != "" {
		return traceID
	}
	if requestID := header.Get(HeaderRequestID); requestID != "" {
		return requestID
	}
	return NewTraceID()
}

// NewTraceID 生成新的 traceID。
func NewTraceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "trace-id-fallback"
	}
	return hex.EncodeToString(buf)
}
