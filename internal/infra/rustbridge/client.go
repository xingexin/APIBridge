package rustbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
)

// Client 封装对 Rust 桥接服务的 HTTP 调用。
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient 创建 Rust 桥接客户端。
func NewClient(cfg Config, logger *zap.Logger) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("invalid rust bridge base url %q: %v", cfg.BaseURL, err))
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &Client{
		baseURL: parsed,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// ChatCompletion 调用聊天补全内部接口。
func (c *Client) ChatCompletion(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, pathChatCompletions, payload, headers)
}

// Response 调用 responses 内部接口。
func (c *Client) Response(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, pathResponses, payload, headers)
}

// ImageGeneration 调用图片生成内部接口。
func (c *Client) ImageGeneration(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, pathImageGenerations, payload, headers)
}

// ImageEdit 调用图片编辑内部接口。
func (c *Client) ImageEdit(ctx context.Context, payload []byte, headers http.Header) (*http.Response, error) {
	return c.doJSON(ctx, http.MethodPost, pathImageEdits, payload, headers)
}

// UploadFile 调用文件上传内部接口。
func (c *Client) UploadFile(ctx context.Context, filename string, contentType string, content io.Reader, purpose string, headers http.Header) (entity.FileUploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("purpose", purpose); err != nil {
		return entity.FileUploadResponse{}, err
	}

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return entity.FileUploadResponse{}, err
	}
	if _, err := io.Copy(part, content); err != nil {
		return entity.FileUploadResponse{}, err
	}
	if err := writer.Close(); err != nil {
		return entity.FileUploadResponse{}, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, pathFiles, &body)
	if err != nil {
		return entity.FileUploadResponse{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		req.Header.Set("X-File-Content-Type", contentType)
	}
	copyAllowedHeaders(req.Header, headers)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.WithContext(c.logger, ctx).Error("调用 Rust 文件上传接口失败", zap.Error(err))
		return entity.FileUploadResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return entity.FileUploadResponse{}, decodeBridgeError(resp)
	}

	var result entity.FileUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.FileUploadResponse{}, err
	}
	logging.WithContext(c.logger, ctx).Debug("Rust 文件上传接口调用成功",
		zap.String("filename", filename),
		zap.String("file_id", result.ID),
	)
	return result, nil
}

// Models 获取模型列表。
func (c *Client) Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, pathModels, nil, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return entity.ModelListResponse(raw), nil
}

// Health 获取桥接服务健康状态。
func (c *Client) Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error) {
	requestPath := pathHealth
	if accountID != "" {
		requestPath = requestPath + "?account_id=" + url.QueryEscape(accountID)
	}

	resp, err := c.doJSON(ctx, http.MethodGet, requestPath, nil, headers)
	if err != nil {
		return entity.HealthResponse{}, err
	}
	defer resp.Body.Close()

	var result entity.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.HealthResponse{}, err
	}
	return result, nil
}

// doJSON 发送常规 JSON 请求到 Rust 桥接服务。
func (c *Client) doJSON(ctx context.Context, method string, endpoint string, payload []byte, headers http.Header) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := c.newRequest(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	copyAllowedHeaders(req.Header, headers)
	if traceID := trace.TraceIDFromContext(ctx); traceID != "" {
		req.Header.Set(trace.HeaderTraceID, traceID)
	}

	logging.WithContext(c.logger, ctx).Debug("请求 Rust 接口",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.WithContext(c.logger, ctx).Error("请求 Rust 接口失败",
			zap.String("method", method),
			zap.String("endpoint", endpoint),
			zap.Error(err),
		)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		logging.WithContext(c.logger, ctx).Warn("Rust 接口返回错误状态",
			zap.String("method", method),
			zap.String("endpoint", endpoint),
			zap.Int("status", resp.StatusCode),
		)
		return nil, decodeBridgeError(resp)
	}
	logging.WithContext(c.logger, ctx).Debug("Rust 接口请求成功",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.Int("status", resp.StatusCode),
	)
	return resp, nil
}

// newRequest 创建基础 HTTP 请求对象。
func (c *Client) newRequest(ctx context.Context, method string, endpoint string, body io.Reader) (*http.Request, error) {
	requestURL, err := c.buildURL(endpoint)
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, requestURL.String(), body)
}

// buildURL 组装 Rust 桥接服务的完整请求地址。
func (c *Client) buildURL(endpoint string) (*url.URL, error) {
	requestURL := *c.baseURL

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	requestURL.Path = path.Join(c.baseURL.Path, endpointURL.Path)
	requestURL.RawQuery = endpointURL.RawQuery
	return &requestURL, nil
}

// copyAllowedHeaders 拷贝允许透传的请求头。
func copyAllowedHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{
		"Authorization",
		"X-Request-Id",
		trace.HeaderTraceID,
		"X-Account-Id",
		"X-Model-Override",
		"Accept",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

// bridgeError 表示 Rust 桥接服务返回的错误。
type bridgeError struct {
	StatusCode int
	Body       string
}

func (e *bridgeError) Error() string {
	return fmt.Sprintf("Rust 桥接服务返回状态码 %d: %s", e.StatusCode, e.Body)
}

// decodeBridgeError 将 Rust 桥接服务错误响应解码为本地错误。
func decodeBridgeError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return &bridgeError{
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}
