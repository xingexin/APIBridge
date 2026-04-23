package handler

import (
	"fmt"
	"io"
	"net/http"

	"GPTBridge/internal/infra/logging"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// forwardToBridge 读取请求体后按操作类型转发给 Rust 桥接服务。
func (r *Router) forwardToBridge(c *gin.Context, operation proxyOperation) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	defer c.Request.Body.Close()

	caller, ok := r.dispatcher[operation]
	if !ok {
		writeError(c, http.StatusInternalServerError, fmt.Errorf("未找到操作 %s 对应的处理函数", operation))
		return
	}

	logger := logging.WithContext(r.logger, c.Request.Context())
	logger.Debug("开始转发请求",
		zap.String("operation", string(operation)),
		zap.Int("payload_bytes", len(payload)),
	)

	resp, err := caller(c, payload, c.Request.Header)
	if err != nil {
		logger.Error("转发请求失败",
			zap.String("operation", string(operation)),
			zap.Error(err),
		)
		writeError(c, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

	logger.Debug("转发请求成功",
		zap.String("operation", string(operation)),
		zap.Int("status", resp.StatusCode),
	)
	copyResponse(c, resp)
}
