package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtCustomClaims embeds the standard RegisteredClaims and adds our custom fields.
type jwtCustomClaims struct {
	Claims
	jwt.RegisteredClaims
}

// Manager handles all JWT operations using a typed struct, not a global variable.
type Manager struct {
	secretKey     []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewManager creates a new JWT Manager.
func NewManager(secret string, accessExpiryHours int, refreshExpiryHours int) *Manager {
	return &Manager{
		secretKey:     []byte(secret),
		accessExpiry:  time.Duration(accessExpiryHours) * time.Hour,
		refreshExpiry: time.Duration(refreshExpiryHours) * time.Hour,
	}
}

// RefreshExpiry returns the configured refresh-token lifetime, so callers can
// compute a matching expires_at when persisting the token in the session store.
func (m *Manager) RefreshExpiry() time.Duration {
	return m.refreshExpiry
}

// Generate creates a signed JWT string for a given user.
func (m *Manager) Generate(userID string, email, role string) (string, error) {
	claims := jwtCustomClaims{
		Claims: Claims{
			UserID: userID,
			Email:  email,
			Role:   role,
		},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "go-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", fmt.Errorf("auth: failed to sign token: %w", err)
	}
	return signedToken, nil
}

func (m *Manager) validate(tokenStr, expectedIssuer string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, fmt.Errorf("auth: token has expired")
		}
		return nil, fmt.Errorf("auth: invalid token: %w", err)
	}

	claims, ok := token.Claims.(*jwtCustomClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("auth: token claims are invalid")
	}
	if claims.RegisteredClaims.Issuer != expectedIssuer {
		return nil, fmt.Errorf("auth: invalid token type")
	}

	return &claims.Claims, nil
}

// ValidateAccess parses and validates an access token.
func (m *Manager) ValidateAccess(tokenStr string) (*Claims, error) {
	return m.validate(tokenStr, "go-api")
}

// ValidateRefresh parses and validates a refresh token.
func (m *Manager) ValidateRefresh(tokenStr string) (*Claims, error) {
	return m.validate(tokenStr, "go-api-refresh")
}

func (m *Manager) GenerateRefreshToken(userID string, email, role string) (string, error) {
	// A random JWT ID makes every refresh token unique. Without it, two tokens
	// minted in the same second (JWT timestamps are second-precision) would be
	// byte-identical, which would break single-use rotation in the session store.
	tokenID, err := newTokenID()
	if err != nil {
		return "", fmt.Errorf("auth: failed to generate token id: %w", err)
	}
	claims := jwtCustomClaims{
		Claims: Claims{
			UserID: userID,
			Email:  email,
			Role:   role,
		},
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.refreshExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "go-api-refresh",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// newTokenID returns a 128-bit random identifier as hex, used as the JWT ID
// (jti) of refresh tokens so each issued token is unique.
func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
