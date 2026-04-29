package entity

import "time"

const (
	CycleStatusActive  = "active"
	CycleStatusExpired = "expired"

	ReservationStatusReserved  = "reserved"
	ReservationStatusCommitted = "committed"
	ReservationStatusReleased  = "released"
)

// UpstreamPool 表示一组可超卖售卖、按真实容量路由的上游帐号池。
type UpstreamPool struct {
	ID                       uint    `gorm:"primaryKey"`
	PoolID                   string  `gorm:"uniqueIndex;size:64;not null"`
	Name                     string  `gorm:"size:128;not null"`
	SourceType               string  `gorm:"size:32;index;not null"`
	BaseURL                  string  `gorm:"size:512"`
	RustGRPCAddr             string  `gorm:"size:255"`
	MonthlyQuotaMicroCredits int64   `gorm:"not null"`
	OversellPercent          float64 `gorm:"not null;default:0"`
	ExhaustThreshold         float64 `gorm:"not null;default:0.98"`
	ActiveCycleID            *uint
	DisabledByAdmin          bool `gorm:"not null;default:false"`
	FrozenByError            bool `gorm:"not null;default:false"`
	CooldownUntil            *time.Time
	LastErrorCode            string `gorm:"size:128"`
	LastErrorAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// UpstreamAPIAccount 表示池内一个真实上游 API 帐号。
type UpstreamAPIAccount struct {
	ID                       uint   `gorm:"primaryKey"`
	PoolID                   uint   `gorm:"index;not null"`
	AccountRef               string `gorm:"size:128;not null"`
	APIKey                   string `gorm:"size:1024"`
	MonthlyQuotaMicroCredits int64  `gorm:"not null"`
	UsedMicroCredits         int64  `gorm:"not null;default:0"`
	ReservedMicroCredits     int64  `gorm:"not null;default:0"`
	Priority                 int    `gorm:"index;not null;default:100"`
	DisabledByAdmin          bool   `gorm:"not null;default:false"`
	FrozenByError            bool   `gorm:"not null;default:false"`
	CooldownUntil            *time.Time
	LastErrorCode            string `gorm:"size:128"`
	LastErrorAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// PoolQuotaCycle 是池的当前/历史额度周期账本。
type PoolQuotaCycle struct {
	ID                   uint      `gorm:"primaryKey"`
	PoolID               uint      `gorm:"index;not null"`
	QuotaMicroCredits    int64     `gorm:"not null"`
	UsedMicroCredits     int64     `gorm:"not null;default:0"`
	ReservedMicroCredits int64     `gorm:"not null;default:0"`
	CycleStartAt         time.Time `gorm:"index;not null"`
	CycleEndAt           time.Time `gorm:"index;not null"`
	Status               string    `gorm:"size:32;index;not null"`
	ReconcileState       string    `gorm:"size:32"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AccountPoolAssignment 记录客户当前绑定池和迁移历史。
type AccountPoolAssignment struct {
	ID                       uint   `gorm:"primaryKey"`
	CustomerAccountID        uint   `gorm:"index;not null"`
	PoolID                   uint   `gorm:"index;not null"`
	UpstreamAccountID        uint   `gorm:"index;not null"`
	SoldCapacityMicroCredits int64  `gorm:"not null;default:0"`
	Active                   bool   `gorm:"index;not null;default:true"`
	Reason                   string `gorm:"size:128"`
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// UpstreamCapacityReservation 是上游侧容量预占记录。
type UpstreamCapacityReservation struct {
	ID                    uint      `gorm:"primaryKey"`
	ReservationID         string    `gorm:"uniqueIndex;size:64;not null"`
	RequestID             string    `gorm:"index;size:128;not null"`
	TraceID               string    `gorm:"size:128"`
	CustomerAccountID     uint      `gorm:"index;not null"`
	PoolID                uint      `gorm:"index;not null"`
	PoolCycleID           uint      `gorm:"index;not null"`
	UpstreamAccountID     uint      `gorm:"index;not null"`
	EstimatedMicroCredits int64     `gorm:"not null"`
	ReservedMicroCredits  int64     `gorm:"not null"`
	FinalMicroCredits     int64     `gorm:"not null;default:0"`
	Status                string    `gorm:"size:32;index;not null"`
	ReleaseReason         string    `gorm:"size:128"`
	ExpiresAt             time.Time `gorm:"index;not null"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// UpstreamResourceOwner 记录上游 stateful 资源归属。
type UpstreamResourceOwner struct {
	ID                uint   `gorm:"primaryKey"`
	ResourceType      string `gorm:"size:64;not null;uniqueIndex:idx_resource_owner"`
	ResourceID        string `gorm:"size:255;not null;uniqueIndex:idx_resource_owner"`
	CustomerAccountID uint   `gorm:"index;not null"`
	PoolID            uint   `gorm:"index;not null"`
	UpstreamAccountID uint   `gorm:"index;not null"`
	SourceRequestID   string `gorm:"index;size:128"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// QuotaReconcileRun 记录上游额度校准观测。
type QuotaReconcileRun struct {
	ID                       uint  `gorm:"primaryKey"`
	PoolID                   uint  `gorm:"index;not null"`
	UpstreamAccountID        *uint `gorm:"index"`
	ObservedUsedMicroCredits int64
	ObservedCostMicroCredits int64
	ProviderState            string `gorm:"size:64"`
	Confidence               float64
	ObservedAt               time.Time `gorm:"index"`
	RawError                 string    `gorm:"type:text"`
	CreatedAt                time.Time
}
