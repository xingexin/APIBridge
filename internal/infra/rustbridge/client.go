package rustbridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

const streamProxyMethod = "/gptbridge.bridge.v1.BridgeService/StreamProxy"

const (
	operationChatCompletion = "chat_completion"
	operationResponse       = "response"
	operationImageGenerate  = "image_generation"
	operationImageEdit      = "image_edit"
	operationFileUpload     = "file_upload"
	operationModels         = "models"
	operationHealth         = "health"
)

// Client 封装对 Rust RPC 服务的调用。
type Client struct {
	addr   string
	conn   *grpc.ClientConn
	logger *zap.Logger
}

// NewClient 创建 Rust RPC 客户端。
func NewClient(cfg Config, logger *zap.Logger) *Client {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = "127.0.0.1:50051"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("connect rust rpc bridge %q failed: %v", addr, err))
	}

	return &Client{
		addr:   addr,
		conn:   conn,
		logger: logger,
	}
}

// Forward 调用 Rust RPC 服务。
func (c *Client) Forward(ctx context.Context, req entity.ProxyRequest) (*http.Response, error) {
	return c.stream(ctx, req.Operation, req.Payload, http.Header(req.Headers))
}

// UploadFile 调用 Rust 的文件上传接口。
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

	nextHeaders := headers.Clone()
	nextHeaders.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		nextHeaders.Set("X-File-Content-Type", contentType)
	}

	resp, err := c.stream(ctx, operationFileUpload, body.Bytes(), nextHeaders)
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

// Models 获取 Rust 返回的模型列表。
func (c *Client) Models(ctx context.Context, headers http.Header) (entity.ModelListResponse, error) {
	resp, err := c.stream(ctx, operationModels, nil, headers)
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

// Health 获取 Rust 服务的健康状态。
func (c *Client) Health(ctx context.Context, accountID string, headers http.Header) (entity.HealthResponse, error) {
	payload, err := json.Marshal(map[string]string{"account_id": accountID})
	if err != nil {
		return entity.HealthResponse{}, err
	}

	resp, err := c.stream(ctx, operationHealth, payload, headers)
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

// stream 调用 Rust 的 server-stream RPC，并包装成 http.Response 交给上层透传。
func (c *Client) stream(ctx context.Context, operation string, payload []byte, headers http.Header) (*http.Response, error) {
	request, err := c.buildRequest(operation, payload, headers)
	if err != nil {
		return nil, err
	}

	desc := &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: false,
	}
	stream, err := c.conn.NewStream(ctx, desc, streamProxyMethod)
	if err != nil {
		logging.WithContext(c.logger, ctx).Error("创建 Rust RPC 流失败",
			zap.String("operation", operation),
			zap.Error(err),
		)
		return nil, err
	}
	if err := stream.SendMsg(request); err != nil {
		return nil, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}

	first := &structpb.Struct{}
	if err := stream.RecvMsg(first); err != nil {
		return nil, err
	}

	statusCode := int(numberValue(first, "status_code", http.StatusOK))
	responseHeaders := headerFromStruct(first)
	reader, writer := io.Pipe()

	go c.copyStream(ctx, operation, stream, writer)

	logging.WithContext(c.logger, ctx).Debug("Rust RPC 流已建立",
		zap.String("operation", operation),
		zap.Int("status", statusCode),
	)

	return &http.Response{
		StatusCode: statusCode,
		Header:     responseHeaders,
		Body:       reader,
	}, nil
}

// copyStream 将 Rust RPC 返回的 body chunk 写入管道。
func (c *Client) copyStream(ctx context.Context, operation string, stream grpc.ClientStream, writer *io.PipeWriter) {
	defer writer.Close()

	for {
		chunk := &structpb.Struct{}
		if err := stream.RecvMsg(chunk); err != nil {
			if err == io.EOF {
				return
			}
			logging.WithContext(c.logger, ctx).Error("读取 Rust RPC 流失败",
				zap.String("operation", operation),
				zap.Error(err),
			)
			_ = writer.CloseWithError(err)
			return
		}

		data, err := decodeBody(chunk)
		if err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		if len(data) == 0 {
			continue
		}
		if _, err := writer.Write(data); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
	}
}

// buildRequest 构造通用 RPC 请求。
func (c *Client) buildRequest(operation string, payload []byte, headers http.Header) (*structpb.Struct, error) {
	values := map[string]any{
		"operation":   operation,
		"headers":     selectedHeaders(headers),
		"body_base64": base64.StdEncoding.EncodeToString(payload),
	}
	return structpb.NewStruct(values)
}

// selectedHeaders 选择允许传给 Rust 的请求头。
func selectedHeaders(headers http.Header) map[string]any {
	result := make(map[string]any)
	for _, key := range []string{
		"Authorization",
		trace.HeaderRequestID,
		trace.HeaderTraceID,
		"X-Account-Id",
		"X-Model-Override",
		"X-File-Content-Type",
		"Content-Type",
		"Accept",
	} {
		if value := headers.Get(key); value != "" {
			result[key] = value
		}
	}
	return result
}

// headerFromStruct 从首个 RPC chunk 中提取响应头。
func headerFromStruct(value *structpb.Struct) http.Header {
	header := make(http.Header)
	headersValue := value.GetFields()["headers"]
	if headersValue == nil || headersValue.GetStructValue() == nil {
		return header
	}
	for key, item := range headersValue.GetStructValue().GetFields() {
		header.Set(key, item.GetStringValue())
	}
	return header
}

// decodeBody 解码 RPC body chunk。
func decodeBody(value *structpb.Struct) ([]byte, error) {
	raw := value.GetFields()["body_base64"]
	if raw == nil || raw.GetStringValue() == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(raw.GetStringValue())
}

// numberValue 从 Struct 中读取数字。
func numberValue(value *structpb.Struct, key string, fallback float64) float64 {
	raw := value.GetFields()[key]
	if raw == nil {
		return fallback
	}
	return raw.GetNumberValue()
}
