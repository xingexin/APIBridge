package handler

import (
	"net/http"

	"GPTBridge/internal/domain/proxy/entity"
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
type proxyCaller func(*gin.Context, entity.ProxyRequest) (*http.Response, error)

// buildDispatchTable 构建操作和调用函数的映射关系。
func (r *Router) buildDispatchTable() map[proxyOperation]proxyCaller {
	return map[proxyOperation]proxyCaller{
		operationChatCompletion: r.callChatCompletion,
		operationResponse:       r.callResponse,
		operationImageGenerate:  r.callImageGeneration,
		operationImageEdit:      r.callImageEdit,
	}
}

// callChatCompletion 调用聊天补全处理函数。
func (r *Router) callChatCompletion(c *gin.Context, req entity.ProxyRequest) (*http.Response, error) {
	return r.proxy.Forward(c.Request.Context(), req)
}

// callResponse 调用 responses 处理函数。
func (r *Router) callResponse(c *gin.Context, req entity.ProxyRequest) (*http.Response, error) {
	return r.proxy.Forward(c.Request.Context(), req)
}

// callImageGeneration 调用图片生成处理函数。
func (r *Router) callImageGeneration(c *gin.Context, req entity.ProxyRequest) (*http.Response, error) {
	return r.proxy.Forward(c.Request.Context(), req)
}

// callImageEdit 调用图片编辑处理函数。
func (r *Router) callImageEdit(c *gin.Context, req entity.ProxyRequest) (*http.Response, error) {
	return r.proxy.Forward(c.Request.Context(), req)
}
