package handler

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	proxyservice "GPTBridge/internal/domain/proxy/service"
	walletservice "GPTBridge/internal/domain/wallet/service"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Router 负责注册对外 HTTP 路由并调用代理服务。
type Router struct {
	proxy      *proxyservice.ProxyService
	billing    *walletservice.BillingService
	engine     *gin.Engine
	dispatcher map[proxyOperation]proxyCaller
	logger     *zap.Logger
}

// NewRouter 创建基于 Gin 的路由入口。
func NewRouter(proxy *proxyservice.ProxyService, billing *walletservice.BillingService, logger *zap.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := &Router{
		proxy:   proxy,
		billing: billing,
		engine:  gin.New(),
		logger:  logger,
	}
	router.dispatcher = router.buildDispatchTable()
	router.engine.Use(router.traceMiddleware(), router.billingMiddleware(), router.accessLogMiddleware(), gin.Recovery())
	router.registerRoutes()
	return router.engine
}

// registerRoutes 注册当前支持的对外接口。
func (r *Router) registerRoutes() {
	r.engine.GET("/health", r.handleHealth)
	r.engine.POST("/v1/chat/completions", r.handleChatCompletions)
	r.engine.POST("/v1/responses", r.handleResponses)
	r.engine.POST("/v1/images/generations", r.handleImageGenerations)
	r.engine.POST("/v1/images/edits", r.handleImageEdits)
	r.engine.GET("/v1/models", r.handleModels)
	r.engine.NoRoute(r.handleNoRoute)
}

// handleHealth 处理健康检查请求。
func (r *Router) handleHealth(c *gin.Context) {
	resp, err := r.proxy.Health(c.Request.Context(), c.Query("account_id"), c.Request.Header)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// handleChatCompletions 处理聊天补全请求。
func (r *Router) handleChatCompletions(c *gin.Context) {
	r.forwardToBridge(c, operationChatCompletion)
}

// handleResponses 处理 responses 请求。
func (r *Router) handleResponses(c *gin.Context) {
	r.forwardToBridge(c, operationResponse)
}

// handleImageGenerations 处理图片生成请求。
func (r *Router) handleImageGenerations(c *gin.Context) {
	r.forwardToBridge(c, operationImageGenerate)
}

// handleImageEdits 处理图片编辑请求。
func (r *Router) handleImageEdits(c *gin.Context) {
	r.forwardToBridge(c, operationImageEdit)
}

// handleModels 处理模型列表请求。
func (r *Router) handleModels(c *gin.Context) {
	models, err := r.proxy.Models(c.Request.Context(), c.Request.Header)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}

	c.Data(http.StatusOK, "application/json", models)
}

// handleNoRoute 将未显式注册的 /v1 请求走通用代理。
func (r *Router) handleNoRoute(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		r.forwardToBridge(c, operationProxy)
		return
	}
	writeError(c, http.StatusNotFound, http.ErrNoLocation)
}

// writeError 输出统一的桥接错误响应。
func writeError(c *gin.Context, status int, err error) {
	c.JSON(status, map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "bridge_error",
		},
	})
}

// copyResponse 原样写回上游响应，并在响应结束后执行计费。
func (r *Router) copyResponse(c *gin.Context, resp *http.Response, requestBody []byte) {
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(resp.StatusCode)

	var captured bytes.Buffer
	_, _ = copyAndFlush(c.Writer, io.TeeReader(resp.Body, &captured))

	if resp.StatusCode >= http.StatusBadRequest || r.billing == nil || !r.billing.Enabled() {
		return
	}
	account, ok := walletservice.AccountFromContext(c.Request.Context())
	if !ok {
		return
	}
	if _, err := r.billing.Charge(c.Request.Context(), account, requestBody, captured.Bytes()); err != nil {
		logging.WithContext(r.logger, c.Request.Context()).Error("请求计费失败", zap.Error(err))
	}
}

// traceMiddleware 为每个请求准备 traceID 并写入上下文。
func (r *Router) traceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := trace.EnsureTraceID(c.Request.Header)
		c.Request.Header.Set(trace.HeaderTraceID, traceID)
		c.Writer.Header().Set(trace.HeaderTraceID, traceID)
		c.Request = c.Request.WithContext(trace.WithTraceID(c.Request.Context(), traceID))
		c.Next()
	}
}

// billingMiddleware 校验平台 API Key，并把账号写入上下文。
func (r *Router) billingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if r.billing == nil || !r.billing.RequireAPIKey() || !strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			c.Next()
			return
		}

		account, err := r.billing.Authenticate(c.Request.Header)
		if err != nil {
			status := http.StatusUnauthorized
			if err == walletservice.ErrInsufficientBalance {
				status = http.StatusPaymentRequired
			}
			writeError(c, status, err)
			c.Abort()
			return
		}

		c.Writer.Header().Set("X-Billing-Account", account.Name)
		c.Request = c.Request.WithContext(walletservice.WithAccount(c.Request.Context(), account))
		c.Next()
	}
}

// accessLogMiddleware 记录基础访问日志。
func (r *Router) accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		logging.WithContext(r.logger, c.Request.Context()).Info("请求完成",
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.String("client_ip", c.ClientIP()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func copyAndFlush(writer gin.ResponseWriter, reader io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			writeN, writeErr := writer.Write(buf[:n])
			written += int64(writeN)
			writer.Flush()
			if writeErr != nil {
				return written, writeErr
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
