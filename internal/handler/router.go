package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"GPTBridge/internal/biz/proxygateway"
	billingservice "GPTBridge/internal/domain/billing/service"
	userservice "GPTBridge/internal/domain/user/service"
	"GPTBridge/internal/infra/logging"
	"GPTBridge/internal/infra/trace"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Router 负责注册对外 HTTP 路由并调用代理服务。
type Router struct {
	gateway *proxygateway.Gateway
	auth    *userservice.AuthService
	engine  *gin.Engine
	logger  *zap.Logger
}

// NewRouter 创建基于 Gin 的路由入口。
func NewRouter(gateway *proxygateway.Gateway, auth *userservice.AuthService, logger *zap.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)

	router := &Router{
		gateway: gateway,
		auth:    auth,
		engine:  gin.New(),
		logger:  logger,
	}
	router.engine.Use(router.traceMiddleware(), router.sessionMiddleware(), router.accessLogMiddleware(), gin.Recovery())
	router.registerRoutes()
	return router.engine
}

// registerRoutes 注册当前支持的对外接口。
func (r *Router) registerRoutes() {
	r.engine.GET("/health", r.handleHealth)
	r.engine.POST("/auth/login", r.handleLogin)
	r.engine.POST("/auth/logout", r.handleLogout)
	r.engine.GET("/auth/me", r.handleMe)
	r.engine.POST("/v1/chat/completions", r.handleChatCompletions)
	r.engine.POST("/v1/responses", r.handleResponses)
	r.engine.POST("/v1/images/generations", r.handleImageGenerations)
	r.engine.POST("/v1/images/edits", r.handleImageEdits)
	r.engine.GET("/v1/models", r.handleModels)
	r.engine.NoRoute(r.handleNoRoute)
}

// handleHealth 处理健康检查请求。
func (r *Router) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
	r.forwardToBridge(c, operationProxy)
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

func writeGatewayError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, billingservice.ErrMissingAPIKey), errors.Is(err, billingservice.ErrInvalidAPIKey):
		status = http.StatusUnauthorized
	case errors.Is(err, billingservice.ErrDisabledAPIKey):
		status = http.StatusForbidden
	case errors.Is(err, billingservice.ErrInsufficientCredits):
		status = http.StatusPaymentRequired
	case errors.Is(err, proxygateway.ErrUpstreamQuotaExhausted), errors.Is(err, proxygateway.ErrStatefulUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeError(c, status, err)
}

// copyResponse 原样写回上游响应，并在响应结束后执行计费。
func (r *Router) copyResponse(c *gin.Context, run *proxygateway.Run) {
	resp := run.Response
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(resp.StatusCode)
	if run.Features.RequestID != "" {
		c.Writer.Header().Set(trace.HeaderRequestID, run.Features.RequestID)
	}

	var captured bytes.Buffer
	_, copyErr := copyAndFlush(c.Writer, io.TeeReader(resp.Body, &captured))
	if err := r.gateway.Finalize(c.Request.Context(), run, resp.StatusCode, captured.Bytes(), copyErr); err != nil {
		r.logger.Error("请求结算失败", zap.Error(err))
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

// sessionMiddleware 从 cookie 中恢复当前登录用户。
func (r *Router) sessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if r.auth == nil {
			c.Next()
			return
		}
		token, err := c.Cookie(r.auth.CookieName())
		if err != nil || token == "" {
			c.Next()
			return
		}
		user, err := r.auth.AuthenticateSession(c.Request.Context(), token)
		if err != nil {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(userservice.WithCurrentUser(c.Request.Context(), user))
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
