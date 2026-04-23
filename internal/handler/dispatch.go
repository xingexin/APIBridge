package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// proxyOperation 表示当前支持的代理操作类型。
type proxyOperation string

const (
	operationChatCompletion proxyOperation = "chat_completion"
	operationResponse       proxyOperation = "response"
	operationImageGenerate  proxyOperation = "image_generation"
	operationImageEdit      proxyOperation = "image_edit"
)

// proxyCaller 表示具体的下游调用函数。
type proxyCaller func(*gin.Context, []byte, http.Header) (*http.Response, error)

// buildDispatchTable 构建操作和调用函数的映射关系。
func (r *Router) buildDispatchTable() map[proxyOperation]proxyCaller {
	return map[proxyOperation]proxyCaller{
		operationChatCompletion: func(c *gin.Context, payload []byte, headers http.Header) (*http.Response, error) {
			return r.proxy.ChatCompletion(c.Request.Context(), payload, headers)
		},
		operationResponse: func(c *gin.Context, payload []byte, headers http.Header) (*http.Response, error) {
			return r.proxy.Response(c.Request.Context(), payload, headers)
		},
		operationImageGenerate: func(c *gin.Context, payload []byte, headers http.Header) (*http.Response, error) {
			return r.proxy.ImageGeneration(c.Request.Context(), payload, headers)
		},
		operationImageEdit: func(c *gin.Context, payload []byte, headers http.Header) (*http.Response, error) {
			return r.proxy.ImageEdit(c.Request.Context(), payload, headers)
		},
	}
}
