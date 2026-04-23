package rustbridge

// 内部接口路径常量由 Go 与 Rust 共同约定。
const (
	pathChatCompletions  = "/internal/chat/completions"
	pathResponses        = "/internal/responses"
	pathImageGenerations = "/internal/images/generations"
	pathImageEdits       = "/internal/images/edits"
	pathFiles            = "/internal/files"
	pathModels           = "/internal/models"
	pathHealth           = "/internal/health"
)
