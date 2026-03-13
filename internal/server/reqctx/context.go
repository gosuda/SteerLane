package reqctx

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	tenantIDKey
	userIDKey
	userRoleKey
)

func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func TenantIDFrom(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(tenantIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func UserIDFrom(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func UserRoleFrom(ctx context.Context) string {
	if role, ok := ctx.Value(userRoleKey).(string); ok {
		return role
	}
	return ""
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func WithTenant(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

func WithUser(ctx context.Context, userID uuid.UUID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, userRoleKey, role)
}
