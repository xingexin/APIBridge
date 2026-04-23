package service

import (
	"context"

	"GPTBridge/internal/domain/wallet/entity"
)

type accountContextKey struct{}

// WithAccount 将 API Key 账号写入上下文。
func WithAccount(ctx context.Context, account entity.APIKeyAccount) context.Context {
	return context.WithValue(ctx, accountContextKey{}, account)
}

// AccountFromContext 从上下文中读取 API Key 账号。
func AccountFromContext(ctx context.Context) (entity.APIKeyAccount, bool) {
	account, ok := ctx.Value(accountContextKey{}).(entity.APIKeyAccount)
	return account, ok
}
