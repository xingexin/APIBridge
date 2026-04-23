package entity

import "encoding/json"

// ChatCompletionRequest 表示聊天补全请求原始报文。
type ChatCompletionRequest json.RawMessage

// ChatCompletionResponse 表示聊天补全响应原始报文。
type ChatCompletionResponse json.RawMessage

// ResponseRequest 表示 responses 请求原始报文。
type ResponseRequest json.RawMessage

// ResponseEnvelope 表示 responses 响应原始报文。
type ResponseEnvelope json.RawMessage

// ImageGenerationRequest 表示图片生成请求原始报文。
type ImageGenerationRequest json.RawMessage

// ImageGenerationResponse 表示图片生成响应原始报文。
type ImageGenerationResponse json.RawMessage

// ImageEditRequest 表示图片编辑请求原始报文。
type ImageEditRequest json.RawMessage

// ImageEditResponse 表示图片编辑响应原始报文。
type ImageEditResponse json.RawMessage

// FileUploadResponse 表示文件上传后的返回结果。
type FileUploadResponse struct {
	ID       string `json:"id"`
	Object   string `json:"object,omitempty"`
	Filename string `json:"filename,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
}

// ModelListResponse 表示模型列表原始报文。
type ModelListResponse json.RawMessage

// HealthResponse 表示桥接服务健康检查结果。
type HealthResponse struct {
	Status    string `json:"status"`
	AccountID string `json:"account_id,omitempty"`
	Message   string `json:"message,omitempty"`
}
