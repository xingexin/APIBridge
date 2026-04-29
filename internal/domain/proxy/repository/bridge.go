package repository

import (
	"context"
	"io"
	"net/http"

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/domain/proxy/entity"
)

// Forwarder 定义一个可执行上游路由的代理 provider。
type Forwarder interface {
	Forward(ctx context.Context, route contracts.Route, req entity.ProxyRequest) (*http.Response, error)
	UploadFile(ctx context.Context, filename string, contentType string, content io.Reader, purpose string, headers http.Header) (entity.FileUploadResponse, error)
	Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error)
	Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error)
}
