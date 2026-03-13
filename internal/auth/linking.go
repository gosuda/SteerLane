package auth

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gosuda/steerlane/internal/domain"
)

const defaultLinkPath = "/auth/link"

// LinkingClaims carries the information needed to link a messenger identity to
// a SteerLane account after the user returns from the browser flow.
type LinkingClaims struct {
	jwt.RegisteredClaims
	Platform       string          `json:"platform"`
	ExternalUserID string          `json:"external_user_id"`
	TenantID       domain.TenantID `json:"tenant_id"`
}

// LinkingService generates and validates signed account-linking URLs.
type LinkingService struct {
	baseURL string
	secret  []byte
	expiry  time.Duration
}

// NewLinkingService creates a new LinkingService.
func NewLinkingService(secret, baseURL string, expiry time.Duration) *LinkingService {
	if strings.TrimSpace(secret) == "" {
		panic("auth: linking secret must not be empty")
	}
	if strings.TrimSpace(baseURL) == "" {
		panic("auth: linking base URL must not be empty")
	}
	if expiry <= 0 {
		panic("auth: linking expiry must be positive")
	}

	return &LinkingService{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  []byte(secret),
		expiry:  expiry,
	}
}

// IssueToken creates a signed linking token for a messenger identity.
func (s *LinkingService) IssueToken(tenantID domain.TenantID, platform, externalUserID string) (string, error) {
	if strings.TrimSpace(platform) == "" {
		return "", fmt.Errorf("issue linking token: platform: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(externalUserID) == "" {
		return "", fmt.Errorf("issue linking token: external user id: %w", domain.ErrInvalidInput)
	}

	now := time.Now().UTC()
	claims := &LinkingClaims{
		Platform:       platform,
		ExternalUserID: externalUserID,
		TenantID:       tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "steerlane-linking",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
			ID:        domain.NewID().String(),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("issue linking token: %w", err)
	}

	return token, nil
}

// GenerateLink builds a browser URL containing a signed linking token.
func (s *LinkingService) GenerateLink(tenantID domain.TenantID, platform, externalUserID string) (string, error) {
	token, err := s.IssueToken(tenantID, platform, externalUserID)
	if err != nil {
		return "", fmt.Errorf("generate linking URL: %w", err)
	}

	values := url.Values{}
	values.Set("token", token)

	return s.baseURL + defaultLinkPath + "?" + values.Encode(), nil
}

// ParseToken validates and parses a linking token.
func (s *LinkingService) ParseToken(raw string) (*LinkingClaims, error) {
	token, err := jwt.ParseWithClaims(raw, &LinkingClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithIssuer("steerlane-linking"),
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*LinkingClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
