package normalapi

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

	"GPTBridge/internal/biz/contracts"
	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
)

// Client 封装对正常 API 上游的 HTTP 调用。
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
}

type requestOptions struct {
	contentType string
}

// NewClient 创建正常 API 客户端。
func NewClient(cfg Config, logger *zap.Logger) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("invalid normal api base url %q: %v", cfg.BaseURL, err))
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	return &Client{
		baseURL: parsed,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// Forward 按 route 中的地址和凭证请求正常 API。
func (c *Client) Forward(ctx context.Context, route contracts.Route, req entity.ProxyRequest) (*http.Response, error) {
	headers := http.Header(req.Headers)
	return c.forward(ctx, route, req.Method, req.Path, bytes.NewReader(req.Payload), headers, requestOptions{
		contentType: headers.Get("Content-Type"),
	})
}

// UploadFile 调用正常 API 的文件上传接口。
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

	resp, err := c.forward(ctx, contracts.Route{}, http.MethodPost, "/v1/files", &body, headers, requestOptions{
		contentType: writer.FormDataContentType(),
	})
	if err != nil {
		return entity.FileUploadResponse{}, err
	}
	defer resp.Body.Close()

	var result entity.FileUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return entity.FileUploadResponse{}, err
	}
	return result, nil
}

// Models 获取正常 API 返回的模型列表。
func (c *Client) Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error) {
	resp, err := c.forward(ctx, contracts.Route{}, http.MethodGet, "/v1/models", nil, headers, requestOptions{})
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

// Health 返回正常 API 客户端的本地健康状态。
func (c *Client) Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error) {
	return entity.HealthResponse{
		Status:  "ok",
		Message: "normal api client ready",
	}, nil
}

// forward 将请求转发到正常 API 上游。
func (c *Client) forward(ctx context.Context, route contracts.Route, method string, endpoint string, body io.Reader, headers http.Header, opts requestOptions) (*http.Response, error) {
	req, err := c.newRequest(ctx, route, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if opts.contentType != "" {
		req.Header.Set("Content-Type", opts.contentType)
	}
	c.applyHeaders(req.Header, route, headers)

	logging.WithContext(c.logger, ctx).Debug("请求正常 API",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.WithContext(c.logger, ctx).Error("请求正常 API 失败",
			zap.String("method", method),
			zap.String("endpoint", endpoint),
			zap.Error(err),
		)
		return nil, err
	}
	return resp, nil
}

// newRequest 创建基础 HTTP 请求对象。
func (c *Client) newRequest(ctx context.Context, route contracts.Route, method string, endpoint string, body io.Reader) (*http.Request, error) {
	requestURL, err := c.buildURL(route, endpoint)
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, requestURL.String(), body)
}

// buildURL 组装正常 API 上游的完整请求地址。
func (c *Client) buildURL(route contracts.Route, endpoint string) (*url.URL, error) {
	base := c.baseURL
	if route.BaseURL != "" {
		parsed, err := url.Parse(strings.TrimRight(route.BaseURL, "/"))
		if err != nil {
			return nil, err
		}
		base = parsed
	}
	requestURL := *base
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	requestURL.Path = path.Join(base.Path, endpointURL.Path)
	requestURL.RawQuery = endpointURL.RawQuery
	return &requestURL, nil
}

// applyHeaders 设置上游请求需要的请求头。
func (c *Client) applyHeaders(dst http.Header, route contracts.Route, src http.Header) {
	if route.APIKey != "" {
		dst.Set("Authorization", "Bearer "+route.APIKey)
	} else if c.apiKey != "" {
		dst.Set("Authorization", "Bearer "+c.apiKey)
	} else if value := src.Get("Authorization"); value != "" {
		dst.Set("Authorization", value)
	}
	for key, value := range route.ExtraHeaders {
		if key != "" && value != "" {
			dst.Set(key, value)
		}
	}

	for _, key := range []string{
		trace.HeaderTraceID,
		trace.HeaderRequestID,
		"Accept",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

// apiError 表示正常 API 上游返回的错误。
type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("正常 API 返回状态码 %d: %s", e.StatusCode, e.Body)
}

// decodeAPIError 将正常 API 错误响应解码为本地错误。
func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return &apiError{
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}
