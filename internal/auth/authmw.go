package auth

import (
	"context"
	"fmt"

	"github.com/gosuda/steerlane/internal/server/middleware"
)

// AuthAdapter wraps Service to implement middleware.Authenticator.
// It bridges the gap between the concrete auth claims and the general
// identity structure required by the HTTP middleware.
type AuthAdapter struct {
	svc *Service
}

// NewAuthAdapter creates a new AuthAdapter wrapping the provided auth service.
func NewAuthAdapter(svc *Service) *AuthAdapter {
	if svc == nil {
		panic("auth: service must not be nil")
	}
	return &AuthAdapter{svc: svc}
}

// AuthenticateJWT validates a JWT and returns the identity.
func (a *AuthAdapter) AuthenticateJWT(token string) (*middleware.Identity, error) {
	claims, err := a.svc.AuthenticateJWT(token)
	if err != nil {
		return nil, err
	}
	return claimsToIdentity(claims)
}

// AuthenticateAPIKey validates an API key and returns the identity.
func (a *AuthAdapter) AuthenticateAPIKey(ctx context.Context, rawKey string) (*middleware.Identity, error) {
	claims, err := a.svc.AuthenticateAPIKey(ctx, rawKey)
	if err != nil {
		return nil, err
	}
	return claimsToIdentity(claims)
}

func claimsToIdentity(claims *Claims) (*middleware.Identity, error) {
	userID, err := claims.SubjectUUID()
	if err != nil {
		return nil, fmt.Errorf("invalid subject in claims: %w", err)
	}

	return &middleware.Identity{
		UserID:   userID,
		TenantID: claims.TenantID,
		Role:     claims.Role,
	}, nil
}
