package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	billingentity "GPTBridge/internal/domain/billing/entity"
	"GPTBridge/internal/infra/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MicroCreditsPerCredit int64 = 1_000_000

var ErrQuotaExceeded = errors.New("客户额度不足")

// GormBillingRepository 使用 GORM 操作 billing/access 域数据。
type GormBillingRepository struct {
	db *gorm.DB
}

func NewGormBillingRepository(db *gorm.DB) *GormBillingRepository {
	return &GormBillingRepository{db: db}
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&billingentity.BillingAccount{},
		&billingentity.BillingAPIKey{},
		&billingentity.AccountQuotaPeriod{},
		&billingentity.AccountRecharge{},
		&billingentity.BillingReservation{},
		&billingentity.BillingLedgerRecord{},
	)
}

// SeedAccounts 从现有 billing.seed_accounts 初始化客户帐号、平台 Key 和当前周期额度。
func (r *GormBillingRepository) SeedAccounts(ctx context.Context, accounts []config.BillingAccountConfig, periodDays int) error {
	if periodDays <= 0 {
		periodDays = 30
	}
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := NewGormBillingRepository(tx)
		for _, item := range accounts {
			account := billingentity.BillingAccount{}
			err := tx.Where("account_id = ?", item.AccountID).First(&account).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				account = billingentity.BillingAccount{
					AccountID: item.AccountID,
					Name:      item.Name,
					Enabled:   item.Enabled,
				}
				if err := tx.Create(&account).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			for _, key := range item.APIKeys {
				apiKey := billingentity.BillingAPIKey{}
				err := tx.Where("`key` = ?", key.Key).First(&apiKey).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					apiKey = billingentity.BillingAPIKey{
						AccountID: account.ID,
						Key:       key.Key,
						Name:      key.Name,
						Enabled:   key.Enabled,
					}
					if err := tx.Create(&apiKey).Error; err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
			}

			var count int64
			if err := tx.Model(&billingentity.AccountQuotaPeriod{}).Where("account_id = ?", account.ID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				period := billingentity.AccountQuotaPeriod{
					AccountID:            account.ID,
					QuotaMicroCredits:    CreditsToMicro(item.Balance),
					UsedMicroCredits:     0,
					ReservedMicroCredits: 0,
					PeriodStartAt:        now,
					PeriodEndAt:          now.AddDate(0, 0, periodDays),
					Status:               billingentity.PeriodStatusActive,
				}
				if err := tx.Create(&period).Error; err != nil {
					return err
				}
				account.CurrentPeriodID = &period.ID
				if err := tx.Save(&account).Error; err != nil {
					return err
				}
				if err := repo.createLedger(ctx, account.ID, period.ID, "", "seed", 0, 0, "", ""); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func CreditsToMicro(value float64) int64 {
	return int64(value * float64(MicroCreditsPerCredit))
}

func (r *GormBillingRepository) FindAccountByAPIKey(ctx context.Context, key string) (billingentity.CustomerAccount, error) {
	var apiKey billingentity.BillingAPIKey
	if err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&apiKey).Error; err != nil {
		return billingentity.CustomerAccount{}, err
	}

	var account billingentity.BillingAccount
	if err := r.db.WithContext(ctx).Where("id = ?", apiKey.AccountID).First(&account).Error; err != nil {
		return billingentity.CustomerAccount{}, err
	}

	return billingentity.CustomerAccount{
		ID:        account.ID,
		AccountID: account.AccountID,
		Name:      account.Name,
		Enabled:   account.Enabled && apiKey.Enabled,
		KeyID:     apiKey.ID,
		Key:       apiKey.Key,
	}, nil
}

type ReserveInput struct {
	ReservationID         string
	RequestID             string
	TraceID               string
	AccountID             uint
	Endpoint              string
	Model                 string
	SettlementPolicy      string
	PolicyVersion         string
	PriceSnapshot         string
	EstimatedMicroCredits int64
	ExpiresAt             time.Time
	Now                   time.Time
}

func (r *GormBillingRepository) Reserve(ctx context.Context, input ReserveInput) (billingentity.BillingReservation, billingentity.AccountQuotaPeriod, error) {
	if input.EstimatedMicroCredits <= 0 {
		input.EstimatedMicroCredits = 1
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}

	period, err := r.activePeriod(ctx, input.AccountID, input.Now)
	if err != nil {
		return billingentity.BillingReservation{}, billingentity.AccountQuotaPeriod{}, err
	}

	available := period.QuotaMicroCredits - period.UsedMicroCredits - period.ReservedMicroCredits
	if available < input.EstimatedMicroCredits {
		return billingentity.BillingReservation{}, billingentity.AccountQuotaPeriod{}, ErrQuotaExceeded
	}
	period.ReservedMicroCredits += input.EstimatedMicroCredits
	if err := r.db.WithContext(ctx).Save(&period).Error; err != nil {
		return billingentity.BillingReservation{}, billingentity.AccountQuotaPeriod{}, err
	}

	reservation := billingentity.BillingReservation{
		ReservationID:         input.ReservationID,
		RequestID:             input.RequestID,
		TraceID:               input.TraceID,
		AccountID:             input.AccountID,
		PeriodID:              period.ID,
		Endpoint:              input.Endpoint,
		Model:                 input.Model,
		SettlementPolicy:      input.SettlementPolicy,
		PolicyVersion:         input.PolicyVersion,
		PriceSnapshot:         input.PriceSnapshot,
		EstimatedMicroCredits: input.EstimatedMicroCredits,
		ReservedMicroCredits:  input.EstimatedMicroCredits,
		Status:                billingentity.ReservationStatusReserved,
		ExpiresAt:             input.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(&reservation).Error; err != nil {
		return billingentity.BillingReservation{}, billingentity.AccountQuotaPeriod{}, err
	}
	if err := r.createLedger(ctx, input.AccountID, period.ID, input.ReservationID, "reserve", input.EstimatedMicroCredits, 0, input.RequestID, input.TraceID); err != nil {
		return billingentity.BillingReservation{}, billingentity.AccountQuotaPeriod{}, err
	}
	return reservation, period, nil
}

func (r *GormBillingRepository) Commit(ctx context.Context, reservationID string, finalMicroCredits int64) error {
	if finalMicroCredits < 0 {
		finalMicroCredits = 0
	}

	var reservation billingentity.BillingReservation
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("reservation_id = ?", reservationID).First(&reservation).Error; err != nil {
		return err
	}
	if reservation.Status != billingentity.ReservationStatusReserved {
		return nil
	}

	var period billingentity.AccountQuotaPeriod
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.PeriodID).First(&period).Error; err != nil {
		return err
	}

	nextReserved := period.ReservedMicroCredits - reservation.ReservedMicroCredits
	if nextReserved < 0 {
		nextReserved = 0
	}
	if period.UsedMicroCredits+nextReserved+finalMicroCredits > period.QuotaMicroCredits {
		return fmt.Errorf("%w: commit would exceed period quota", ErrQuotaExceeded)
	}

	period.ReservedMicroCredits = nextReserved
	period.UsedMicroCredits += finalMicroCredits
	if err := r.db.WithContext(ctx).Save(&period).Error; err != nil {
		return err
	}

	reservation.FinalMicroCredits = finalMicroCredits
	reservation.Status = billingentity.ReservationStatusCommitted
	if err := r.db.WithContext(ctx).Save(&reservation).Error; err != nil {
		return err
	}
	return r.createLedger(ctx, reservation.AccountID, period.ID, reservation.ReservationID, "commit", -reservation.ReservedMicroCredits, finalMicroCredits, reservation.RequestID, reservation.TraceID)
}

func (r *GormBillingRepository) Release(ctx context.Context, reservationID string, reason string) error {
	var reservation billingentity.BillingReservation
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("reservation_id = ?", reservationID).First(&reservation).Error; err != nil {
		return err
	}
	if reservation.Status != billingentity.ReservationStatusReserved {
		return nil
	}

	var period billingentity.AccountQuotaPeriod
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.PeriodID).First(&period).Error; err != nil {
		return err
	}
	period.ReservedMicroCredits -= reservation.ReservedMicroCredits
	if period.ReservedMicroCredits < 0 {
		period.ReservedMicroCredits = 0
	}
	if err := r.db.WithContext(ctx).Save(&period).Error; err != nil {
		return err
	}

	reservation.Status = billingentity.ReservationStatusReleased
	reservation.ReleaseReason = reason
	if err := r.db.WithContext(ctx).Save(&reservation).Error; err != nil {
		return err
	}
	return r.createLedger(ctx, reservation.AccountID, period.ID, reservation.ReservationID, "release", -reservation.ReservedMicroCredits, 0, reservation.RequestID, reservation.TraceID)
}

func (r *GormBillingRepository) activePeriod(ctx context.Context, accountID uint, now time.Time) (billingentity.AccountQuotaPeriod, error) {
	var period billingentity.AccountQuotaPeriod
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND status = ? AND period_start_at <= ? AND period_end_at > ?", accountID, billingentity.PeriodStatusActive, now, now).
		Order("period_end_at ASC").
		First(&period).Error
	if err == nil {
		return period, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return billingentity.AccountQuotaPeriod{}, err
	}

	if err := r.db.WithContext(ctx).Model(&billingentity.AccountQuotaPeriod{}).
		Where("account_id = ? AND status = ? AND period_end_at <= ?", accountID, billingentity.PeriodStatusActive, now).
		Update("status", billingentity.PeriodStatusExpired).Error; err != nil {
		return billingentity.AccountQuotaPeriod{}, err
	}

	err = r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND status = ? AND period_start_at <= ? AND period_end_at > ?", accountID, billingentity.PeriodStatusPending, now, now).
		Order("period_start_at ASC").
		First(&period).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return billingentity.AccountQuotaPeriod{}, ErrQuotaExceeded
		}
		return billingentity.AccountQuotaPeriod{}, err
	}
	period.Status = billingentity.PeriodStatusActive
	if err := r.db.WithContext(ctx).Save(&period).Error; err != nil {
		return billingentity.AccountQuotaPeriod{}, err
	}
	return period, nil
}

func (r *GormBillingRepository) createLedger(ctx context.Context, accountID uint, periodID uint, reservationID string, entryType string, deltaReserved int64, deltaUsed int64, requestID string, traceID string) error {
	var period billingentity.AccountQuotaPeriod
	if err := r.db.WithContext(ctx).Where("id = ?", periodID).First(&period).Error; err != nil {
		return err
	}
	record := billingentity.BillingLedgerRecord{
		AccountID:            accountID,
		PeriodID:             periodID,
		ReservationID:        reservationID,
		EntryType:            entryType,
		DeltaReservedMicro:   deltaReserved,
		DeltaUsedMicro:       deltaUsed,
		BalanceReservedMicro: period.ReservedMicroCredits,
		BalanceUsedMicro:     period.UsedMicroCredits,
		RequestID:            requestID,
		TraceID:              traceID,
	}
	return r.db.WithContext(ctx).Create(&record).Error
}
