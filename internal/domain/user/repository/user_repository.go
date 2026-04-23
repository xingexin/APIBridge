package repository

import (
	"context"
	"errors"
	"time"

	userentity "GPTBridge/internal/domain/user/entity"
	"GPTBridge/internal/infra/config"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserRepository 定义 user 域需要的仓库能力。
type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (userentity.User, error)
	FindBySessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (userentity.User, userentity.UserSession, error)
	CreateSession(ctx context.Context, session userentity.UserSession) error
	RevokeSession(ctx context.Context, tokenHash string, now time.Time) error
}

// GormUserRepository 使用 GORM 操作 user 域数据。
type GormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository 创建 GORM 用户仓库。
func NewGormUserRepository(db *gorm.DB) *GormUserRepository {
	return &GormUserRepository{db: db}
}

// AutoMigrate 自动迁移 user 域表结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&userentity.User{}, &userentity.UserSession{})
}

// SeedUsers 从配置初始化用户。
func (r *GormUserRepository) SeedUsers(ctx context.Context, users []config.SeedUserConfig) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range users {
			var user userentity.User
			err := tx.Where("username = ?", item.Username).First(&user).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				hash, err := bcrypt.GenerateFromPassword([]byte(item.Password), bcrypt.DefaultCost)
				if err != nil {
					return err
				}
				user = userentity.User{
					Username:     item.Username,
					PasswordHash: string(hash),
					DisplayName:  item.DisplayName,
					Role:         item.Role,
					Enabled:      item.Enabled,
				}
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		return nil
	})
}

// FindByUsername 根据用户名查询用户。
func (r *GormUserRepository) FindByUsername(ctx context.Context, username string) (userentity.User, error) {
	var user userentity.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	return user, err
}

// FindBySessionTokenHash 根据 session token hash 查询用户。
func (r *GormUserRepository) FindBySessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (userentity.User, userentity.UserSession, error) {
	var session userentity.UserSession
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ? AND revoked_at IS NULL", tokenHash, now).
		First(&session).Error
	if err != nil {
		return userentity.User{}, userentity.UserSession{}, err
	}

	var user userentity.User
	err = r.db.WithContext(ctx).Where("id = ?", session.UserID).First(&user).Error
	return user, session, err
}

// CreateSession 创建用户 session。
func (r *GormUserRepository) CreateSession(ctx context.Context, session userentity.UserSession) error {
	return r.db.WithContext(ctx).Create(&session).Error
}

// RevokeSession 注销用户 session。
func (r *GormUserRepository) RevokeSession(ctx context.Context, tokenHash string, now time.Time) error {
	return r.db.WithContext(ctx).
		Model(&userentity.UserSession{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Update("revoked_at", now).Error
}
