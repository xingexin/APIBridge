package repository

import (
	"context"
	"errors"

	walletentity "GPTBridge/internal/domain/wallet/entity"
	"GPTBridge/internal/infra/config"
	"gorm.io/gorm"
)

// WalletRepository 定义 wallet 域需要的仓库能力。
type WalletRepository interface {
	FindAccountByAPIKey(ctx context.Context, key string) (walletentity.APIKeyAccount, error)
	ChargeAccount(ctx context.Context, account walletentity.APIKeyAccount, usage walletentity.Usage, cost float64, traceID string) (walletentity.UsageRecord, error)
}

// GormWalletRepository 使用 GORM 操作 wallet 域数据。
type GormWalletRepository struct {
	db *gorm.DB
}

// NewGormWalletRepository 创建 GORM 钱包仓库。
func NewGormWalletRepository(db *gorm.DB) *GormWalletRepository {
	return &GormWalletRepository{db: db}
}

// AutoMigrate 自动迁移 wallet 域表结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&walletentity.WalletAccount{}, &walletentity.WalletAPIKey{}, &walletentity.WalletUsageRecord{})
}

// SeedAccounts 从配置初始化账号和 API Key。
func (r *GormWalletRepository) SeedAccounts(ctx context.Context, accounts []config.BillingAccountConfig) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range accounts {
			account := walletentity.WalletAccount{}
			err := tx.Where("account_id = ?", item.AccountID).First(&account).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				account = walletentity.WalletAccount{
					AccountID: item.AccountID,
					Name:      item.Name,
					Balance:   item.Balance,
					Enabled:   item.Enabled,
				}
				if err := tx.Create(&account).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			for _, key := range item.APIKeys {
				apiKey := walletentity.WalletAPIKey{}
				err := tx.Where("key = ?", key.Key).First(&apiKey).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					apiKey = walletentity.WalletAPIKey{
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
		}
		return nil
	})
}

// FindAccountByAPIKey 根据 API Key 查找账号。
func (r *GormWalletRepository) FindAccountByAPIKey(ctx context.Context, key string) (walletentity.APIKeyAccount, error) {
	var apiKey walletentity.WalletAPIKey
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&apiKey).Error; err != nil {
		return walletentity.APIKeyAccount{}, err
	}

	var account walletentity.WalletAccount
	if err := r.db.WithContext(ctx).Where("id = ?", apiKey.AccountID).First(&account).Error; err != nil {
		return walletentity.APIKeyAccount{}, err
	}

	return walletentity.APIKeyAccount{
		AccountID: account.AccountID,
		KeyID:     apiKey.ID,
		Key:       apiKey.Key,
		Name:      account.Name,
		Balance:   account.Balance,
		Enabled:   account.Enabled && apiKey.Enabled,
	}, nil
}

// ChargeAccount 扣减账号余额并写入用量记录。
func (r *GormWalletRepository) ChargeAccount(ctx context.Context, account walletentity.APIKeyAccount, usage walletentity.Usage, cost float64, traceID string) (walletentity.UsageRecord, error) {
	var record walletentity.UsageRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dbAccount walletentity.WalletAccount
		if err := tx.Where("account_id = ?", account.AccountID).First(&dbAccount).Error; err != nil {
			return err
		}
		dbAccount.Balance -= cost
		if err := tx.Save(&dbAccount).Error; err != nil {
			return err
		}

		dbRecord := walletentity.WalletUsageRecord{
			AccountID:    dbAccount.ID,
			APIKeyID:     account.KeyID,
			Model:        usage.Model,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			Cost:         cost,
			BalanceAfter: dbAccount.Balance,
			TraceID:      traceID,
		}
		if err := tx.Create(&dbRecord).Error; err != nil {
			return err
		}

		record = walletentity.UsageRecord{
			AccountID:    dbAccount.AccountID,
			APIKeyID:     account.KeyID,
			AccountName:  dbAccount.Name,
			Model:        usage.Model,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			Cost:         cost,
			BalanceAfter: dbAccount.Balance,
			CreatedAt:    dbRecord.CreatedAt,
		}
		return nil
	})
	return record, err
}
