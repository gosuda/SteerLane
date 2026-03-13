package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims extends jwt.RegisteredClaims with tenant and role info.
type Claims struct {
	jwt.RegisteredClaims
	Role     string    `json:"role"`
	TenantID uuid.UUID `json:"tid"`
}

// SubjectUUID parses the standard "sub" claim as a UUID.
func (c *Claims) SubjectUUID() (uuid.UUID, error) {
	sub, err := c.GetSubject()
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting subject: %w", err)
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing subject UUID: %w", err)
	}
	return id, nil
}

// JWTService manages JWT token issuance and validation.
type JWTService struct {
	issuer        string
	secret        []byte
	expiry        time.Duration
	refreshExpiry time.Duration
}

// NewJWTService creates a JWTService with the given signing parameters.
// Panics if secret is empty -- this is a configuration error that must be caught at startup.
func NewJWTService(secret, issuer string, expiry, refreshExpiry time.Duration) *JWTService {
	if secret == "" {
		panic("auth: JWT secret must not be empty")
	}
	if issuer == "" {
		panic("auth: JWT issuer must not be empty")
	}
	return &JWTService{
		secret:        []byte(secret),
		issuer:        issuer,
		expiry:        expiry,
		refreshExpiry: refreshExpiry,
	}
}

// IssueTokens generates an access token and refresh token for the given identity.
// The access token has a short expiry; the refresh token has a longer expiry.
func (s *JWTService) IssueTokens(userID, tenantID uuid.UUID, role string) (access, refresh string, err error) {
	now := time.Now()

	accessClaims := &Claims{
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
			ID:        uuid.New().String(),
		},
	}

	access, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("signing access token: %w", err)
	}

	refreshClaims := &Claims{
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpiry)),
			ID:        uuid.New().String(),
		},
	}

	refresh, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(s.secret)
	if err != nil {
		return "", "", fmt.Errorf("signing refresh token: %w", err)
	}

	return access, refresh, nil
}

// ParseToken validates and parses a JWT string, returning the claims.
// Returns ErrInvalidToken if the token is malformed, expired, or has an invalid signature.
func (s *JWTService) ParseToken(raw string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithIssuer(s.issuer),
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
