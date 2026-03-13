package server

import (
	"context"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// Typed context keys and helpers are implemented in reqctx to avoid cyclic dependencies
// between the server and middleware packages, while exposing them here as requested.

func RequestIDFrom(ctx context.Context) string {
	return reqctx.RequestIDFrom(ctx)
}

func TenantIDFrom(ctx context.Context) uuid.UUID {
	return reqctx.TenantIDFrom(ctx)
}

func UserIDFrom(ctx context.Context) uuid.UUID {
	return reqctx.UserIDFrom(ctx)
}

func UserRoleFrom(ctx context.Context) string {
	return reqctx.UserRoleFrom(ctx)
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return reqctx.WithRequestID(ctx, id)
}

func WithTenant(ctx context.Context, id uuid.UUID) context.Context {
	return reqctx.WithTenant(ctx, id)
}

func WithUser(ctx context.Context, userID uuid.UUID, role string) context.Context {
	return reqctx.WithUser(ctx, userID, role)
}
