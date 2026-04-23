package service

import (
	"context"

	userentity "GPTBridge/internal/domain/user/entity"
)

type currentUserContextKey struct{}

// WithCurrentUser 将当前用户写入上下文。
func WithCurrentUser(ctx context.Context, user userentity.CurrentUser) context.Context {
	return context.WithValue(ctx, currentUserContextKey{}, user)
}

// CurrentUserFromContext 从上下文中读取当前用户。
func CurrentUserFromContext(ctx context.Context) (userentity.CurrentUser, bool) {
	user, ok := ctx.Value(currentUserContextKey{}).(userentity.CurrentUser)
	return user, ok
}
