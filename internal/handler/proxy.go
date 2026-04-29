package handler

import (
	"io"

	"GPTBridge/internal/domain/proxy/entity"
	"GPTBridge/internal/infra/logging"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// proxyOperation 表示当前支持的代理操作类型。
type proxyOperation string

const (
	operationChatCompletion proxyOperation = "chat_completion"
	operationResponse       proxyOperation = "response"
	operationImageGenerate  proxyOperation = "image_generation"
	operationImageEdit      proxyOperation = "image_edit"
	operationProxy          proxyOperation = "proxy"
)

// forwardToBridge 读取请求体后按操作类型转发给 Rust 桥接服务。
func (r *Router) forwardToBridge(c *gin.Context, operation proxyOperation) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, 400, err)
		return
	}
	defer c.Request.Body.Close()

	logger := logging.WithContext(r.logger, c.Request.Context())
	logger.Debug("开始转发请求",
		zap.String("operation", string(operation)),
		zap.Int("payload_bytes", len(payload)),
	)

	run, err := r.gateway.Start(c.Request.Context(), entity.ProxyRequest{
		Operation: string(operation),
		Method:    c.Request.Method,
		Path:      requestPath(c),
		Payload:   payload,
		Headers:   c.Request.Header,
	})
	if err != nil {
		logger.Error("转发请求失败",
			zap.String("operation", string(operation)),
			zap.Error(err),
		)
		writeGatewayError(c, err)
		return
	}
	defer run.Response.Body.Close()

	logger.Debug("转发请求成功",
		zap.String("operation", string(operation)),
		zap.Int("status", run.Response.StatusCode),
	)
	r.copyResponse(c, run)
}

// requestPath 返回客户端原始请求路径。
func requestPath(c *gin.Context) string {
	if c.Request.URL.RawQuery == "" {
		return c.Request.URL.Path
	}
	return c.Request.URL.Path + "?" + c.Request.URL.RawQuery
}
