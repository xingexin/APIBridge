package repository

import (
	"context"
	"io"
	"net/http"

	"GPTBridge/internal/domain/proxy/entity"
)

// Bridge 定义 Go 调用 Rust 桥接服务所需的能力接口。
type Bridge interface {
	ChatCompletion(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error)
	Response(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error)
	ImageGeneration(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error)
	ImageEdit(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error)
	UploadFile(ctx context.Context, filename string, contentType string, content io.Reader, purpose string, headers http.Header) (entity.FileUploadResponse, error)
	Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error)
	Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error)
}
