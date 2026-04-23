package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	userentity "GPTBridge/internal/domain/user/entity"
	userrepository "GPTBridge/internal/domain/user/repository"
	"GPTBridge/internal/infra/config"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredential = errors.New("用户名或密码错误")
	ErrUserDisabled      = errors.New("用户已禁用")
	ErrInvalidSession    = errors.New("登录已失效")
)

// AuthService 负责用户登录和 session 校验。
type AuthService struct {
	repository   userrepository.UserRepository
	cookieName   string
	ttl          time.Duration
	cookieSecure bool
}

// NewAuthService 创建用户认证服务。
func NewAuthService(cfg config.AuthConfig, repository userrepository.UserRepository) *AuthService {
	ttlHours := cfg.SessionTTLHours
	if ttlHours <= 0 {
		ttlHours = 168
	}
	cookieName := cfg.SessionCookieName
	if cookieName == "" {
		cookieName = "gptbridge_session"
	}
	return &AuthService{
		repository:   repository,
		cookieName:   cookieName,
		ttl:          time.Duration(ttlHours) * time.Hour,
		cookieSecure: cfg.CookieSecure,
	}
}

// CookieName 返回 session cookie 名称。
func (s *AuthService) CookieName() string {
	return s.cookieName
}

// CookieMaxAge 返回 session cookie 最大存活秒数。
func (s *AuthService) CookieMaxAge() int {
	return int(s.ttl.Seconds())
}

// CookieSecure 返回 cookie 是否只允许 HTTPS。
func (s *AuthService) CookieSecure() bool {
	return s.cookieSecure
}

// Login 校验用户名密码并创建 session。
func (s *AuthService) Login(ctx context.Context, username string, password string) (userentity.CurrentUser, string, error) {
	user, err := s.repository.FindByUsername(ctx, username)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return userentity.CurrentUser{}, "", ErrInvalidCredential
	}
	if err != nil {
		return userentity.CurrentUser{}, "", err
	}
	if !user.Enabled {
		return userentity.CurrentUser{}, "", ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return userentity.CurrentUser{}, "", ErrInvalidCredential
	}

	token, err := newSessionToken()
	if err != nil {
		return userentity.CurrentUser{}, "", err
	}
	session := userentity.UserSession{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(s.ttl),
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return userentity.CurrentUser{}, "", err
	}
	return currentUser(user), token, nil
}

// AuthenticateSession 根据 session token 查询当前用户。
func (s *AuthService) AuthenticateSession(ctx context.Context, token string) (userentity.CurrentUser, error) {
	if token == "" {
		return userentity.CurrentUser{}, ErrInvalidSession
	}
	user, _, err := s.repository.FindBySessionTokenHash(ctx, hashToken(token), time.Now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return userentity.CurrentUser{}, ErrInvalidSession
	}
	if err != nil {
		return userentity.CurrentUser{}, err
	}
	if !user.Enabled {
		return userentity.CurrentUser{}, ErrUserDisabled
	}
	return currentUser(user), nil
}

// Logout 注销当前 session。
func (s *AuthService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repository.RevokeSession(ctx, hashToken(token), time.Now())
}

func currentUser(user userentity.User) userentity.CurrentUser {
	return userentity.CurrentUser{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	}
}

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
