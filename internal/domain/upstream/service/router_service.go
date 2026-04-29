package service

import (
	"context"
	"errors"
	"time"

	"GPTBridge/internal/biz/contracts"
	upstreamrepository "GPTBridge/internal/domain/upstream/repository"
	"GPTBridge/internal/infra/logging"
	"go.uber.org/zap"
)

var (
	ErrNoAvailablePool          = upstreamrepository.ErrNoAvailablePool
	ErrStatefulRouteUnavailable = upstreamrepository.ErrStatefulRouteUnavailable
)

type RouterService struct {
	repository *upstreamrepository.GormUpstreamRepository
	logger     *zap.Logger
}

type ResolveInput struct {
	CustomerAccountID        uint
	SoldCapacityMicroCredits int64
	Features                 contracts.RequestFeatures
}

type RouteLease struct {
	Route                contracts.Route
	ReservationID        string
	ReservedMicroCredits int64
}

func NewRouterService(repository *upstreamrepository.GormUpstreamRepository, logger *zap.Logger) *RouterService {
	return &RouterService{repository: repository, logger: logger}
}

func (s *RouterService) ResolveAndReserve(ctx context.Context, input ResolveInput) (RouteLease, error) {
	lease, err := s.repository.ResolveAndReserve(ctx, upstreamrepository.ResolveInput{
		ReservationID:            "up_" + input.Features.RequestID,
		RequestID:                input.Features.RequestID,
		TraceID:                  input.Features.TraceID,
		CustomerAccountID:        input.CustomerAccountID,
		SoldCapacityMicroCredits: input.SoldCapacityMicroCredits,
		EstimatedMicroCredits:    input.Features.EstimatedMicroCredits,
		StatefulRefs:             input.Features.StatefulRefs,
		ExpiresAt:                input.Features.ExpiresAt,
		Now:                      time.Now(),
	})
	if err != nil {
		return RouteLease{}, err
	}
	logging.WithContext(s.logger, ctx).Debug("上游容量预占完成",
		zap.String("request_id", input.Features.RequestID),
		zap.Uint("pool_id", lease.Route.PoolID),
		zap.Uint("upstream_account_id", lease.Route.UpstreamAccountID),
		zap.Int64("reserved_micro_credits", lease.ReservedMicroCredits),
	)
	return RouteLease{
		Route:                lease.Route,
		ReservationID:        lease.ReservationID,
		ReservedMicroCredits: lease.ReservedMicroCredits,
	}, nil
}

func (s *RouterService) CommitCapacity(ctx context.Context, reservationID string, finalMicroCredits int64) error {
	if err := s.repository.Commit(ctx, reservationID, finalMicroCredits); err != nil {
		return err
	}
	logging.WithContext(s.logger, ctx).Debug("上游容量提交完成",
		zap.String("reservation_id", reservationID),
		zap.Int64("final_micro_credits", finalMicroCredits),
	)
	return nil
}

func (s *RouterService) ReleaseCapacity(ctx context.Context, reservationID string, reason string) error {
	if err := s.repository.Release(ctx, reservationID, reason); err != nil {
		return err
	}
	logging.WithContext(s.logger, ctx).Debug("上游容量释放完成",
		zap.String("reservation_id", reservationID),
		zap.String("reason", reason),
	)
	return nil
}

func (s *RouterService) ObserveFailure(ctx context.Context, route contracts.Route, failure contracts.UpstreamFailure) error {
	if route.UpstreamAccountID == 0 {
		return nil
	}
	if failure.OccurredAt.IsZero() {
		failure.OccurredAt = time.Now()
	}
	return s.repository.ObserveFailure(ctx, route, failure)
}

func (s *RouterService) RecordResourceOwners(ctx context.Context, route contracts.Route, customerAccountID uint, requestID string, resources []contracts.ObservedResource) error {
	return s.repository.RecordResourceOwners(ctx, route, customerAccountID, requestID, resources)
}

func IsNoCapacity(err error) bool {
	return errors.Is(err, ErrNoAvailablePool) || errors.Is(err, ErrStatefulRouteUnavailable)
}
