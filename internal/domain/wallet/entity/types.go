package entity

import "time"

// WalletAccount 表示计费账号。
type WalletAccount struct {
	ID        uint    `gorm:"primaryKey"`
	AccountID string  `gorm:"uniqueIndex;size:64;not null"`
	Name      string  `gorm:"size:128;not null"`
	Balance   float64 `gorm:"not null"`
	Enabled   bool    `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WalletAPIKey 表示账号下的平台 API Key。
type WalletAPIKey struct {
	ID        uint   `gorm:"primaryKey"`
	AccountID uint   `gorm:"index;not null"`
	Key       string `gorm:"uniqueIndex;size:255;not null"`
	Name      string `gorm:"size:128;not null"`
	Enabled   bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WalletUsageRecord 表示一次请求用量和扣费记录。
type WalletUsageRecord struct {
	ID           uint   `gorm:"primaryKey"`
	AccountID    uint   `gorm:"index;not null"`
	APIKeyID     uint   `gorm:"index;not null"`
	Model        string `gorm:"size:128;not null"`
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Cost         float64 `gorm:"not null"`
	BalanceAfter float64 `gorm:"not null"`
	TraceID      string  `gorm:"size:128"`
	CreatedAt    time.Time
}

// APIKeyAccount 表示一个平台 API Key 账号。
type APIKeyAccount struct {
	AccountID string
	KeyID     uint
	Key       string
	Name      string
	Balance   float64
	Enabled   bool
}

// Usage 表示一次请求的用量。
type Usage struct {
	Model        string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// UsageRecord 表示一次扣费记录。
type UsageRecord struct {
	AccountID    string
	APIKeyID     uint
	AccountName  string
	Model        string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Cost         float64
	BalanceAfter float64
	CreatedAt    time.Time
}
