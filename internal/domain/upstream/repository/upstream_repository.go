package repository

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"GPTBridge/internal/biz/contracts"
	upstreamentity "GPTBridge/internal/domain/upstream/entity"
	"GPTBridge/internal/infra/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const microCreditsPerCredit int64 = 1_000_000

var (
	ErrNoAvailablePool          = errors.New("没有可用上游帐号池")
	ErrStatefulRouteUnavailable = errors.New("stateful 资源原上游不可用")
)

type GormUpstreamRepository struct {
	db *gorm.DB
}

func NewGormUpstreamRepository(db *gorm.DB) *GormUpstreamRepository {
	return &GormUpstreamRepository{db: db}
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&upstreamentity.UpstreamPool{},
		&upstreamentity.UpstreamAPIAccount{},
		&upstreamentity.PoolQuotaCycle{},
		&upstreamentity.AccountPoolAssignment{},
		&upstreamentity.UpstreamCapacityReservation{},
		&upstreamentity.UpstreamResourceOwner{},
		&upstreamentity.QuotaReconcileRun{},
	)
}

func (r *GormUpstreamRepository) SeedDefault(ctx context.Context, cfg config.Config) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range upstreamPoolConfigs(cfg) {
			if item.PoolID == "" {
				item.PoolID = "default"
			}
			if item.Name == "" {
				item.Name = item.PoolID
			}
			if item.SourceType == "" {
				item.SourceType = cfg.Upstream.Mode
			}
			if item.BaseURL == "" {
				item.BaseURL = cfg.OpenAI.BaseURL
			}
			if item.RustGRPCAddr == "" {
				item.RustGRPCAddr = cfg.Rust.GRPCAddr
			}
			if len(item.APIAccounts) == 0 {
				apiKey := ""
				if sourceTypeFromMode(item.SourceType) == contracts.SourceTypeNormal {
					apiKey = cfg.OpenAI.APIKey
				}
				item.APIAccounts = []config.UpstreamAPIAccountConfig{{
					AccountRef:          "default",
					APIKey:              apiKey,
					MonthlyQuotaCredits: item.MonthlyQuotaCredits,
					Priority:            100,
				}}
			}
			pool := upstreamentity.UpstreamPool{}
			err := tx.Where("pool_id = ?", item.PoolID).First(&pool).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				pool = upstreamentity.UpstreamPool{
					PoolID:                   item.PoolID,
					Name:                     item.Name,
					SourceType:               sourceTypeFromMode(item.SourceType),
					BaseURL:                  item.BaseURL,
					RustGRPCAddr:             item.RustGRPCAddr,
					MonthlyQuotaMicroCredits: creditsToMicro(item.MonthlyQuotaCredits),
					OversellPercent:          item.OversellPercent,
					ExhaustThreshold:         item.ExhaustThreshold,
					DisabledByAdmin:          item.DisabledByAdmin,
				}
				if pool.MonthlyQuotaMicroCredits <= 0 {
					pool.MonthlyQuotaMicroCredits = 1_000_000 * microCreditsPerCredit
				}
				if pool.ExhaustThreshold <= 0 {
					pool.ExhaustThreshold = 0.98
				}
				if err := tx.Create(&pool).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			var cycleCount int64
			if err := tx.Model(&upstreamentity.PoolQuotaCycle{}).Where("pool_id = ?", pool.ID).Count(&cycleCount).Error; err != nil {
				return err
			}
			if cycleCount == 0 {
				now := time.Now()
				cycle := upstreamentity.PoolQuotaCycle{
					PoolID:            pool.ID,
					QuotaMicroCredits: pool.MonthlyQuotaMicroCredits,
					CycleStartAt:      now,
					CycleEndAt:        now.AddDate(0, 0, 30),
					Status:            upstreamentity.CycleStatusActive,
					ReconcileState:    "unknown",
				}
				if err := tx.Create(&cycle).Error; err != nil {
					return err
				}
				pool.ActiveCycleID = &cycle.ID
				if err := tx.Save(&pool).Error; err != nil {
					return err
				}
			}

			for _, accountItem := range item.APIAccounts {
				accountRef := accountItem.AccountRef
				if accountRef == "" {
					accountRef = "default"
				}
				var count int64
				if err := tx.Model(&upstreamentity.UpstreamAPIAccount{}).Where("pool_id = ? AND account_ref = ?", pool.ID, accountRef).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					continue
				}
				quota := creditsToMicro(accountItem.MonthlyQuotaCredits)
				if quota <= 0 {
					quota = pool.MonthlyQuotaMicroCredits
				}
				priority := accountItem.Priority
				if priority == 0 {
					priority = 100
				}
				account := upstreamentity.UpstreamAPIAccount{
					PoolID:                   pool.ID,
					AccountRef:               accountRef,
					APIKey:                   accountItem.APIKey,
					MonthlyQuotaMicroCredits: quota,
					Priority:                 priority,
					DisabledByAdmin:          accountItem.DisabledByAdmin,
				}
				if err := tx.Create(&account).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func upstreamPoolConfigs(cfg config.Config) []config.UpstreamPoolConfig {
	if len(cfg.Upstream.Pools) > 0 {
		return cfg.Upstream.Pools
	}
	sourceType := sourceTypeFromMode(cfg.Upstream.Mode)
	apiKey := ""
	if sourceType == contracts.SourceTypeNormal {
		apiKey = cfg.OpenAI.APIKey
	}
	return []config.UpstreamPoolConfig{
		{
			PoolID:              "default",
			Name:                "default",
			SourceType:          sourceType,
			BaseURL:             cfg.OpenAI.BaseURL,
			RustGRPCAddr:        cfg.Rust.GRPCAddr,
			MonthlyQuotaCredits: 1_000_000,
			ExhaustThreshold:    0.98,
			APIAccounts: []config.UpstreamAPIAccountConfig{
				{
					AccountRef:          "default",
					APIKey:              apiKey,
					MonthlyQuotaCredits: 1_000_000,
					Priority:            100,
				},
			},
		},
	}
}

func creditsToMicro(value float64) int64 {
	return int64(value * float64(microCreditsPerCredit))
}

type ResolveInput struct {
	ReservationID            string
	RequestID                string
	TraceID                  string
	CustomerAccountID        uint
	SoldCapacityMicroCredits int64
	EstimatedMicroCredits    int64
	StatefulRefs             []contracts.StatefulRef
	ExpiresAt                time.Time
	Now                      time.Time
}

type RouteLease struct {
	Route                contracts.Route
	ReservationID        string
	PoolID               uint
	PoolCycleID          uint
	UpstreamAccountID    uint
	ReservedMicroCredits int64
}

func (r *GormUpstreamRepository) ResolveAndReserve(ctx context.Context, input ResolveInput) (RouteLease, error) {
	if input.EstimatedMicroCredits <= 0 {
		input.EstimatedMicroCredits = 1
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}

	if owner, ok, err := r.findOwner(ctx, input.StatefulRefs, input.CustomerAccountID); err != nil {
		return RouteLease{}, err
	} else if ok {
		lease, err := r.reserveExact(ctx, input, owner.PoolID, owner.UpstreamAccountID)
		if err != nil {
			return RouteLease{}, ErrStatefulRouteUnavailable
		}
		return lease, nil
	}

	assignment, ok, err := r.activeAssignment(ctx, input.CustomerAccountID)
	if err != nil {
		return RouteLease{}, err
	}
	if ok {
		lease, err := r.reserveExact(ctx, input, assignment.PoolID, assignment.UpstreamAccountID)
		if err == nil {
			return lease, nil
		}
	}

	pool, account, err := r.findBestRoute(ctx, input)
	if err != nil {
		return RouteLease{}, err
	}
	if err := r.replaceAssignment(ctx, input.CustomerAccountID, pool.ID, account.ID, input.SoldCapacityMicroCredits, "auto_route"); err != nil {
		return RouteLease{}, err
	}
	return r.reserveExact(ctx, input, pool.ID, account.ID)
}

func (r *GormUpstreamRepository) Commit(ctx context.Context, reservationID string, finalMicroCredits int64) error {
	if finalMicroCredits < 0 {
		finalMicroCredits = 0
	}

	var reservation upstreamentity.UpstreamCapacityReservation
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("reservation_id = ?", reservationID).First(&reservation).Error; err != nil {
		return err
	}
	if reservation.Status != upstreamentity.ReservationStatusReserved {
		return nil
	}

	var cycle upstreamentity.PoolQuotaCycle
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.PoolCycleID).First(&cycle).Error; err != nil {
		return err
	}
	var account upstreamentity.UpstreamAPIAccount
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.UpstreamAccountID).First(&account).Error; err != nil {
		return err
	}

	cycle.ReservedMicroCredits = maxInt64(0, cycle.ReservedMicroCredits-reservation.ReservedMicroCredits)
	cycle.UsedMicroCredits += finalMicroCredits
	account.ReservedMicroCredits = maxInt64(0, account.ReservedMicroCredits-reservation.ReservedMicroCredits)
	account.UsedMicroCredits += finalMicroCredits
	if err := r.db.WithContext(ctx).Save(&cycle).Error; err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Save(&account).Error; err != nil {
		return err
	}

	reservation.FinalMicroCredits = finalMicroCredits
	reservation.Status = upstreamentity.ReservationStatusCommitted
	return r.db.WithContext(ctx).Save(&reservation).Error
}

func (r *GormUpstreamRepository) Release(ctx context.Context, reservationID string, reason string) error {
	var reservation upstreamentity.UpstreamCapacityReservation
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("reservation_id = ?", reservationID).First(&reservation).Error; err != nil {
		return err
	}
	if reservation.Status != upstreamentity.ReservationStatusReserved {
		return nil
	}

	var cycle upstreamentity.PoolQuotaCycle
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.PoolCycleID).First(&cycle).Error; err != nil {
		return err
	}
	var account upstreamentity.UpstreamAPIAccount
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.UpstreamAccountID).First(&account).Error; err != nil {
		return err
	}

	cycle.ReservedMicroCredits = maxInt64(0, cycle.ReservedMicroCredits-reservation.ReservedMicroCredits)
	account.ReservedMicroCredits = maxInt64(0, account.ReservedMicroCredits-reservation.ReservedMicroCredits)
	if err := r.db.WithContext(ctx).Save(&cycle).Error; err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Save(&account).Error; err != nil {
		return err
	}

	reservation.Status = upstreamentity.ReservationStatusReleased
	reservation.ReleaseReason = reason
	return r.db.WithContext(ctx).Save(&reservation).Error
}

func (r *GormUpstreamRepository) ObserveFailure(ctx context.Context, route contracts.Route, failure contracts.UpstreamFailure) error {
	now := failure.OccurredAt
	if now.IsZero() {
		now = time.Now()
	}
	code := failure.ErrorCode
	if code == "" {
		code = failureCodeFromBody(failure.StatusCode, failure.Body)
	}

	updates := map[string]any{
		"last_error_code": code,
		"last_error_at":   now,
	}
	if failure.StatusCode == 429 {
		updates["cooldown_until"] = now.Add(time.Minute)
	} else if failure.StatusCode == httpStatusUnauthorized || failure.StatusCode == httpStatusForbidden || isQuotaFailure(code) {
		updates["frozen_by_error"] = true
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&upstreamentity.UpstreamAPIAccount{}).Where("id = ?", route.UpstreamAccountID).Updates(updates).Error
}

func (r *GormUpstreamRepository) RecordResourceOwners(ctx context.Context, route contracts.Route, customerAccountID uint, requestID string, resources []contracts.ObservedResource) error {
	for _, resource := range resources {
		if resource.ResourceType == "" || resource.ResourceID == "" {
			continue
		}
		owner := upstreamentity.UpstreamResourceOwner{
			ResourceType:      resource.ResourceType,
			ResourceID:        resource.ResourceID,
			CustomerAccountID: customerAccountID,
			PoolID:            route.PoolID,
			UpstreamAccountID: route.UpstreamAccountID,
			SourceRequestID:   requestID,
		}
		err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "resource_type"}, {Name: "resource_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"customer_account_id", "pool_id", "upstream_account_id", "source_request_id", "updated_at"}),
		}).Create(&owner).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *GormUpstreamRepository) findOwner(ctx context.Context, refs []contracts.StatefulRef, customerAccountID uint) (upstreamentity.UpstreamResourceOwner, bool, error) {
	for _, ref := range refs {
		if ref.ResourceType == "" || ref.ResourceID == "" {
			continue
		}
		var owner upstreamentity.UpstreamResourceOwner
		err := r.db.WithContext(ctx).Where("resource_type = ? AND resource_id = ? AND customer_account_id = ?", ref.ResourceType, ref.ResourceID, customerAccountID).First(&owner).Error
		if err == nil {
			return owner, true, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return upstreamentity.UpstreamResourceOwner{}, false, err
		}
	}
	return upstreamentity.UpstreamResourceOwner{}, false, nil
}

func (r *GormUpstreamRepository) activeAssignment(ctx context.Context, customerAccountID uint) (upstreamentity.AccountPoolAssignment, bool, error) {
	var assignment upstreamentity.AccountPoolAssignment
	err := r.db.WithContext(ctx).Where("customer_account_id = ? AND active = ?", customerAccountID, true).Order("id DESC").First(&assignment).Error
	if err == nil {
		return assignment, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return upstreamentity.AccountPoolAssignment{}, false, nil
	}
	return upstreamentity.AccountPoolAssignment{}, false, err
}

func (r *GormUpstreamRepository) findBestRoute(ctx context.Context, input ResolveInput) (upstreamentity.UpstreamPool, upstreamentity.UpstreamAPIAccount, error) {
	var pools []upstreamentity.UpstreamPool
	if err := r.db.WithContext(ctx).Find(&pools).Error; err != nil {
		return upstreamentity.UpstreamPool{}, upstreamentity.UpstreamAPIAccount{}, err
	}

	var bestPool upstreamentity.UpstreamPool
	var bestAccount upstreamentity.UpstreamAPIAccount
	var bestAvailable int64 = -1
	for _, pool := range pools {
		if !r.poolSelectable(pool, input.Now) {
			continue
		}
		if _, err := r.ensureActiveCycle(ctx, &pool, input.Now); err != nil {
			return upstreamentity.UpstreamPool{}, upstreamentity.UpstreamAPIAccount{}, err
		}
		if !r.hasSellableCapacity(ctx, pool, input.SoldCapacityMicroCredits) {
			continue
		}
		account, available, ok, err := r.bestAccountInPool(ctx, pool, input.EstimatedMicroCredits, input.Now)
		if err != nil {
			return upstreamentity.UpstreamPool{}, upstreamentity.UpstreamAPIAccount{}, err
		}
		if ok && available > bestAvailable {
			bestAvailable = available
			bestPool = pool
			bestAccount = account
		}
	}
	if bestAvailable < 0 {
		return upstreamentity.UpstreamPool{}, upstreamentity.UpstreamAPIAccount{}, ErrNoAvailablePool
	}
	return bestPool, bestAccount, nil
}

func (r *GormUpstreamRepository) reserveExact(ctx context.Context, input ResolveInput, poolID uint, upstreamAccountID uint) (RouteLease, error) {
	var pool upstreamentity.UpstreamPool
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", poolID).First(&pool).Error; err != nil {
		return RouteLease{}, err
	}
	if !r.poolSelectable(pool, input.Now) {
		return RouteLease{}, ErrNoAvailablePool
	}
	cycle, err := r.ensureActiveCycle(ctx, &pool, input.Now)
	if err != nil {
		return RouteLease{}, err
	}

	var account upstreamentity.UpstreamAPIAccount
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND pool_id = ?", upstreamAccountID, pool.ID).First(&account).Error; err != nil {
		return RouteLease{}, err
	}
	if !accountSelectable(account, input.Now) {
		return RouteLease{}, ErrNoAvailablePool
	}

	available := minInt64(routeCapacity(pool.ExhaustThreshold, cycle.QuotaMicroCredits)-cycle.UsedMicroCredits-cycle.ReservedMicroCredits, routeCapacity(pool.ExhaustThreshold, account.MonthlyQuotaMicroCredits)-account.UsedMicroCredits-account.ReservedMicroCredits)
	if available < input.EstimatedMicroCredits {
		return RouteLease{}, ErrNoAvailablePool
	}

	cycle.ReservedMicroCredits += input.EstimatedMicroCredits
	account.ReservedMicroCredits += input.EstimatedMicroCredits
	if err := r.db.WithContext(ctx).Save(&cycle).Error; err != nil {
		return RouteLease{}, err
	}
	if err := r.db.WithContext(ctx).Save(&account).Error; err != nil {
		return RouteLease{}, err
	}

	reservation := upstreamentity.UpstreamCapacityReservation{
		ReservationID:         input.ReservationID,
		RequestID:             input.RequestID,
		TraceID:               input.TraceID,
		CustomerAccountID:     input.CustomerAccountID,
		PoolID:                pool.ID,
		PoolCycleID:           cycle.ID,
		UpstreamAccountID:     account.ID,
		EstimatedMicroCredits: input.EstimatedMicroCredits,
		ReservedMicroCredits:  input.EstimatedMicroCredits,
		Status:                upstreamentity.ReservationStatusReserved,
		ExpiresAt:             input.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(&reservation).Error; err != nil {
		return RouteLease{}, err
	}

	return RouteLease{
		Route: contracts.Route{
			SourceType:        pool.SourceType,
			PoolID:            pool.ID,
			UpstreamAccountID: account.ID,
			BaseURL:           pool.BaseURL,
			APIKey:            account.APIKey,
			CredentialRef:     account.AccountRef,
			RustGRPCAddr:      pool.RustGRPCAddr,
		},
		ReservationID:        reservation.ReservationID,
		PoolID:               pool.ID,
		PoolCycleID:          cycle.ID,
		UpstreamAccountID:    account.ID,
		ReservedMicroCredits: reservation.ReservedMicroCredits,
	}, nil
}

func (r *GormUpstreamRepository) ensureActiveCycle(ctx context.Context, pool *upstreamentity.UpstreamPool, now time.Time) (upstreamentity.PoolQuotaCycle, error) {
	if pool.ActiveCycleID != nil {
		var cycle upstreamentity.PoolQuotaCycle
		err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", *pool.ActiveCycleID).First(&cycle).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return upstreamentity.PoolQuotaCycle{}, err
		}
		if err == nil && cycle.CycleEndAt.After(now) && cycle.Status == upstreamentity.CycleStatusActive {
			return cycle, nil
		}
		if err == nil {
			cycle.Status = upstreamentity.CycleStatusExpired
			if err := r.db.WithContext(ctx).Save(&cycle).Error; err != nil {
				return upstreamentity.PoolQuotaCycle{}, err
			}
		}
	}

	cycle := upstreamentity.PoolQuotaCycle{
		PoolID:            pool.ID,
		QuotaMicroCredits: pool.MonthlyQuotaMicroCredits,
		CycleStartAt:      now,
		CycleEndAt:        now.AddDate(0, 0, 30),
		Status:            upstreamentity.CycleStatusActive,
		ReconcileState:    "unknown",
	}
	if err := r.db.WithContext(ctx).Create(&cycle).Error; err != nil {
		return upstreamentity.PoolQuotaCycle{}, err
	}
	pool.ActiveCycleID = &cycle.ID
	if err := r.db.WithContext(ctx).Save(pool).Error; err != nil {
		return upstreamentity.PoolQuotaCycle{}, err
	}
	if err := r.db.WithContext(ctx).Model(&upstreamentity.UpstreamAPIAccount{}).Where("pool_id = ?", pool.ID).Updates(map[string]any{
		"used_micro_credits":     0,
		"reserved_micro_credits": 0,
	}).Error; err != nil {
		return upstreamentity.PoolQuotaCycle{}, err
	}
	return cycle, nil
}

func (r *GormUpstreamRepository) bestAccountInPool(ctx context.Context, pool upstreamentity.UpstreamPool, estimated int64, now time.Time) (upstreamentity.UpstreamAPIAccount, int64, bool, error) {
	cycle, err := r.ensureActiveCycle(ctx, &pool, now)
	if err != nil {
		return upstreamentity.UpstreamAPIAccount{}, 0, false, err
	}
	poolAvailable := routeCapacity(pool.ExhaustThreshold, cycle.QuotaMicroCredits) - cycle.UsedMicroCredits - cycle.ReservedMicroCredits
	if poolAvailable < estimated {
		return upstreamentity.UpstreamAPIAccount{}, 0, false, nil
	}

	var accounts []upstreamentity.UpstreamAPIAccount
	if err := r.db.WithContext(ctx).Where("pool_id = ?", pool.ID).Order("priority ASC, id ASC").Find(&accounts).Error; err != nil {
		return upstreamentity.UpstreamAPIAccount{}, 0, false, err
	}
	var best upstreamentity.UpstreamAPIAccount
	var bestAvailable int64 = -1
	for _, account := range accounts {
		if !accountSelectable(account, now) {
			continue
		}
		accountAvailable := routeCapacity(pool.ExhaustThreshold, account.MonthlyQuotaMicroCredits) - account.UsedMicroCredits - account.ReservedMicroCredits
		available := minInt64(poolAvailable, accountAvailable)
		if available >= estimated && available > bestAvailable {
			best = account
			bestAvailable = available
		}
	}
	return best, bestAvailable, bestAvailable >= estimated, nil
}

func (r *GormUpstreamRepository) hasSellableCapacity(ctx context.Context, pool upstreamentity.UpstreamPool, soldCapacity int64) bool {
	var assigned int64
	_ = r.db.WithContext(ctx).Model(&upstreamentity.AccountPoolAssignment{}).
		Where("pool_id = ? AND active = ?", pool.ID, true).
		Select("COALESCE(SUM(sold_capacity_micro_credits), 0)").
		Scan(&assigned).Error
	sellable := int64(math.Round(float64(pool.MonthlyQuotaMicroCredits) * (1 + pool.OversellPercent)))
	return sellable-assigned >= soldCapacity
}

func (r *GormUpstreamRepository) replaceAssignment(ctx context.Context, customerAccountID uint, poolID uint, upstreamAccountID uint, soldCapacity int64, reason string) error {
	if err := r.db.WithContext(ctx).Model(&upstreamentity.AccountPoolAssignment{}).
		Where("customer_account_id = ? AND active = ?", customerAccountID, true).
		Update("active", false).Error; err != nil {
		return err
	}
	assignment := upstreamentity.AccountPoolAssignment{
		CustomerAccountID:        customerAccountID,
		PoolID:                   poolID,
		UpstreamAccountID:        upstreamAccountID,
		SoldCapacityMicroCredits: soldCapacity,
		Active:                   true,
		Reason:                   reason,
	}
	return r.db.WithContext(ctx).Create(&assignment).Error
}

func (r *GormUpstreamRepository) poolSelectable(pool upstreamentity.UpstreamPool, now time.Time) bool {
	if pool.DisabledByAdmin || pool.FrozenByError {
		return false
	}
	if pool.CooldownUntil != nil && pool.CooldownUntil.After(now) {
		return false
	}
	return true
}

func accountSelectable(account upstreamentity.UpstreamAPIAccount, now time.Time) bool {
	if account.DisabledByAdmin || account.FrozenByError {
		return false
	}
	if account.CooldownUntil != nil && account.CooldownUntil.After(now) {
		return false
	}
	return true
}

func sourceTypeFromMode(mode string) string {
	switch strings.ToLower(mode) {
	case "api", "normal", "openai":
		return contracts.SourceTypeNormal
	default:
		return contracts.SourceTypeRust
	}
}

func routeCapacity(threshold float64, quota int64) int64 {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.98
	}
	return int64(math.Floor(float64(quota) * threshold))
}

const (
	httpStatusUnauthorized = 401
	httpStatusForbidden    = 403
)

func failureCodeFromBody(status int, body string) string {
	lower := strings.ToLower(body)
	for _, code := range []string{"insufficient_quota", "billing_hard_limit_reached", "quota_exceeded", "invalid_api_key"} {
		if strings.Contains(lower, code) {
			return code
		}
	}
	if status == 429 {
		return "rate_limited"
	}
	return ""
}

func isQuotaFailure(code string) bool {
	switch strings.ToLower(code) {
	case "insufficient_quota", "billing_hard_limit_reached", "quota_exceeded":
		return true
	default:
		return false
	}
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
