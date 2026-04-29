package entity

import "time"

const (
	PeriodStatusActive  = "active"
	PeriodStatusPending = "pending"
	PeriodStatusExpired = "expired"

	ReservationStatusReserved  = "reserved"
	ReservationStatusCommitted = "committed"
	ReservationStatusReleased  = "released"
)

// BillingAccount 是平台客户帐号，平台 API Key 会解析到该帐号。
type BillingAccount struct {
	ID              uint   `gorm:"primaryKey"`
	AccountID       string `gorm:"uniqueIndex;size:64;not null"`
	Name            string `gorm:"size:128;not null"`
	Enabled         bool   `gorm:"not null;default:true"`
	CurrentPeriodID *uint
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BillingAPIKey 是客户访问 GPTBridge 的平台 API Key。
type BillingAPIKey struct {
	ID        uint   `gorm:"primaryKey"`
	AccountID uint   `gorm:"index;not null"`
	Key       string `gorm:"uniqueIndex;size:255;not null"`
	Name      string `gorm:"size:128;not null"`
	Enabled   bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AccountQuotaPeriod 记录客户一个充值周期的额度账本。
type AccountQuotaPeriod struct {
	ID                   uint      `gorm:"primaryKey"`
	AccountID            uint      `gorm:"index;not null"`
	QuotaMicroCredits    int64     `gorm:"not null"`
	UsedMicroCredits     int64     `gorm:"not null;default:0"`
	ReservedMicroCredits int64     `gorm:"not null;default:0"`
	PeriodStartAt        time.Time `gorm:"index;not null"`
	PeriodEndAt          time.Time `gorm:"index;not null"`
	Status               string    `gorm:"size:32;index;not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AccountRecharge 记录客户充值动作。
type AccountRecharge struct {
	ID                uint      `gorm:"primaryKey"`
	AccountID         uint      `gorm:"index;not null"`
	Mode              string    `gorm:"size:64;not null"`
	QuotaMicroCredits int64     `gorm:"not null"`
	PeriodDays        int       `gorm:"not null"`
	CreatedAt         time.Time `gorm:"index"`
}

// BillingReservation 是客户侧额度预占记录。
type BillingReservation struct {
	ID                    uint      `gorm:"primaryKey"`
	ReservationID         string    `gorm:"uniqueIndex;size:64;not null"`
	RequestID             string    `gorm:"index;size:128;not null"`
	TraceID               string    `gorm:"size:128"`
	AccountID             uint      `gorm:"index;not null"`
	PeriodID              uint      `gorm:"index;not null"`
	Endpoint              string    `gorm:"size:255;not null"`
	Model                 string    `gorm:"size:128"`
	SettlementPolicy      string    `gorm:"size:64;not null"`
	PolicyVersion         string    `gorm:"size:64;not null"`
	PriceSnapshot         string    `gorm:"type:text"`
	EstimatedMicroCredits int64     `gorm:"not null"`
	ReservedMicroCredits  int64     `gorm:"not null"`
	FinalMicroCredits     int64     `gorm:"not null;default:0"`
	Status                string    `gorm:"size:32;index;not null"`
	ReleaseReason         string    `gorm:"size:128"`
	ExpiresAt             time.Time `gorm:"index;not null"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// BillingLedgerRecord 是客户侧额度变动流水。
type BillingLedgerRecord struct {
	ID                   uint      `gorm:"primaryKey"`
	AccountID            uint      `gorm:"index;not null"`
	PeriodID             uint      `gorm:"index;not null"`
	ReservationID        string    `gorm:"index;size:64"`
	EntryType            string    `gorm:"size:32;not null"`
	DeltaReservedMicro   int64     `gorm:"not null;default:0"`
	DeltaUsedMicro       int64     `gorm:"not null;default:0"`
	BalanceReservedMicro int64     `gorm:"not null;default:0"`
	BalanceUsedMicro     int64     `gorm:"not null;default:0"`
	RequestID            string    `gorm:"index;size:128"`
	TraceID              string    `gorm:"size:128"`
	CreatedAt            time.Time `gorm:"index"`
}

// CustomerAccount 是 access 解析后的客户身份。
type CustomerAccount struct {
	ID        uint
	AccountID string
	Name      string
	Enabled   bool
	KeyID     uint
	Key       string
}
