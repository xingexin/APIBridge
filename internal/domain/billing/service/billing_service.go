package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"GPTBridge/internal/biz/contracts"
	billingentity "GPTBridge/internal/domain/billing/entity"
	billingrepository "GPTBridge/internal/domain/billing/repository"
	"GPTBridge/internal/infra/config"
	"GPTBridge/internal/infra/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrMissingAPIKey       = errors.New("缺少 API Key")
	ErrInvalidAPIKey       = errors.New("API Key 无效")
	ErrDisabledAPIKey      = errors.New("API Key 已禁用")
	ErrInsufficientCredits = errors.New("客户额度不足")
)

type Service struct {
	enabled       bool
	requireAPIKey bool
	repository    *billingrepository.GormBillingRepository
	logger        *zap.Logger
}

type Reservation struct {
	ReservationID         string
	AccountID             uint
	PeriodID              uint
	EstimatedMicroCredits int64
	ReservedMicroCredits  int64
}

func NewService(cfg config.BillingConfig, repository *billingrepository.GormBillingRepository, logger *zap.Logger) *Service {
	return &Service{
		enabled:       cfg.Enabled,
		requireAPIKey: cfg.RequireAPIKey,
		repository:    repository,
		logger:        logger,
	}
}

func (s *Service) AuthenticateHeader(ctx context.Context, header http.Header) (billingentity.CustomerAccount, error) {
	if !s.enabled || !s.requireAPIKey {
		return billingentity.CustomerAccount{}, nil
	}
	key := bearerToken(header.Get("Authorization"))
	if key == "" {
		return billingentity.CustomerAccount{}, ErrMissingAPIKey
	}
	return s.AuthenticateAPIKey(ctx, key)
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, key string) (billingentity.CustomerAccount, error) {
	account, err := s.repository.FindAccountByAPIKey(ctx, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return billingentity.CustomerAccount{}, ErrInvalidAPIKey
	}
	if err != nil {
		return billingentity.CustomerAccount{}, err
	}
	if !account.Enabled {
		return billingentity.CustomerAccount{}, ErrDisabledAPIKey
	}
	return account, nil
}

func (s *Service) ResolveAndReserve(ctx context.Context, account billingentity.CustomerAccount, features contracts.RequestFeatures) (Reservation, int64, error) {
	dbReservation, period, err := s.repository.Reserve(ctx, billingrepository.ReserveInput{
		ReservationID:         "bill_" + features.RequestID,
		RequestID:             features.RequestID,
		TraceID:               features.TraceID,
		AccountID:             account.ID,
		Endpoint:              features.Endpoint,
		Model:                 features.Model,
		SettlementPolicy:      features.SettlementPolicy,
		PolicyVersion:         features.PolicyVersion,
		PriceSnapshot:         features.PriceSnapshot,
		EstimatedMicroCredits: features.EstimatedMicroCredits,
		ExpiresAt:             features.ExpiresAt,
		Now:                   time.Now(),
	})
	if errors.Is(err, billingrepository.ErrQuotaExceeded) {
		return Reservation{}, 0, ErrInsufficientCredits
	}
	if err != nil {
		return Reservation{}, 0, err
	}
	logging.WithContext(s.logger, ctx).Debug("客户额度预占完成",
		zap.String("request_id", features.RequestID),
		zap.Uint("account_id", account.ID),
		zap.Int64("reserved_micro_credits", dbReservation.ReservedMicroCredits),
	)
	return Reservation{
		ReservationID:         dbReservation.ReservationID,
		AccountID:             account.ID,
		PeriodID:              period.ID,
		EstimatedMicroCredits: dbReservation.EstimatedMicroCredits,
		ReservedMicroCredits:  dbReservation.ReservedMicroCredits,
	}, period.QuotaMicroCredits, nil
}

func (s *Service) CommitUsage(ctx context.Context, reservationID string, finalMicroCredits int64) error {
	if err := s.repository.Commit(ctx, reservationID, finalMicroCredits); err != nil {
		if errors.Is(err, billingrepository.ErrQuotaExceeded) {
			return ErrInsufficientCredits
		}
		return err
	}
	logging.WithContext(s.logger, ctx).Debug("客户额度提交完成",
		zap.String("reservation_id", reservationID),
		zap.Int64("final_micro_credits", finalMicroCredits),
	)
	return nil
}

func (s *Service) ReleaseReservation(ctx context.Context, reservationID string, reason string) error {
	if err := s.repository.Release(ctx, reservationID, reason); err != nil {
		return err
	}
	logging.WithContext(s.logger, ctx).Debug("客户额度释放完成",
		zap.String("reservation_id", reservationID),
		zap.String("reason", reason),
	)
	return nil
}

func bearerToken(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}
