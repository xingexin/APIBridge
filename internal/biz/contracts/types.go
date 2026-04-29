package contracts

import "time"

const (
	SourceTypeNormal = "normal"
	SourceTypeRust   = "rust"
)

// Route 是 upstream 领域产出的可执行路由，proxy 只按它转发。
type Route struct {
	SourceType        string
	PoolID            uint
	UpstreamAccountID uint
	BaseURL           string
	APIKey            string
	CredentialRef     string
	RustGRPCAddr      string
	ExtraHeaders      map[string]string
}

// RequestFeatures 是 gateway 从 OpenAI 风格请求里解析出的结构化特征。
type RequestFeatures struct {
	RequestID             string
	TraceID               string
	Method                string
	Endpoint              string
	Model                 string
	EstimatedMicroCredits int64
	SettlementPolicy      string
	PolicyVersion         string
	PriceSnapshot         string
	StatefulRefs          []StatefulRef
	ExpectedResources     []string
	ExpiresAt             time.Time
}

// StatefulRef 表示请求里引用的上游有状态资源。
type StatefulRef struct {
	ResourceType string
	ResourceID   string
}

// ObservedResource 表示响应成功后新产生的上游资源。
type ObservedResource struct {
	ResourceType string
	ResourceID   string
}

// Usage 表示一次响应解析出的用量。
type Usage struct {
	Model        string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// UpstreamFailure 表示一次上游失败观测。
type UpstreamFailure struct {
	StatusCode int
	ErrorCode  string
	Body       string
	OccurredAt time.Time
}
