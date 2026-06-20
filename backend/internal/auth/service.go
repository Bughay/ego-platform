package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthService defines the contract for authentication business logic.
type AuthService interface {
	Register(ctx context.Context, req *RegisterRequest) (*RegistrationResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*LoginResponse, string, error)
	Logout(ctx context.Context, refreshToken string) error
	Refresh(ctx context.Context, refreshToken string) (*RefreshResponse, string, error)
}

type authService struct {
	userRepo   UserRepository
	tokenRepo  RefreshTokenRepository
	jwtManager *Manager
}

// NewAuthService creates a new AuthService.
func NewAuthService(userRepo UserRepository, tokenRepo RefreshTokenRepository, jwtManager *Manager) AuthService {
	return &authService{userRepo: userRepo, tokenRepo: tokenRepo, jwtManager: jwtManager}
}

// hashToken returns the hex-encoded SHA-256 of a refresh token. We persist only
// this hash (never the raw token), so a leaked sessions table cannot be used to
// impersonate users. SHA-256 is sufficient here — the token is already long and
// random, so there is nothing to brute-force.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*RegistrationResponse, error) {
	// --- Validation ---
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		return nil, fmt.Errorf("validation: a valid email address is required")
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("validation: password must be at least 8 characters long")
	}

	// --- Check for existing user ---
	existing, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("register: failed to check existing user: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("validation: a user with this email already exists")
	}

	// --- Hash the password using bcrypt at cost 12 ---
	// Cost 12 is the modern recommendation: secure enough, not so slow it impacts UX.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("register: failed to hash password: %w", err)
	}

	user, err := s.userRepo.Create(ctx, req.Email, string(hash))
	if err != nil {
		return nil, fmt.Errorf("register: failed to create user: %w", err)
	}

	return &RegistrationResponse{
		Message: "You have succesfully registered",
		Success: true,
		Person:  user,
	}, nil
}

func (s *authService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, string, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, "", fmt.Errorf("login: database error: %w", err)
	}

	// IMPORTANT: Return the same generic error for both "user not found" and
	// "wrong password" to prevent user enumeration attacks.
	if user == nil {
		return nil, "", fmt.Errorf("auth: invalid email or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, "", fmt.Errorf("auth: invalid email or password")
	}

	accessToken, err := s.jwtManager.Generate(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, "", err
	}
	refreshToken, err := s.jwtManager.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, "", err
	}

	// Record the refresh token in the session store so it can be honored (and
	// later revoked). Without a row here the token would be rejected on refresh.
	if err := s.tokenRepo.Create(ctx, user.ID, hashToken(refreshToken), time.Now().Add(s.jwtManager.RefreshExpiry())); err != nil {
		return nil, "", fmt.Errorf("login: failed to persist refresh token: %w", err)
	}

	return &LoginResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		User:        user,
	}, refreshToken, nil
}

// Refresh validates the refresh token and issues NEW tokens (rotation)
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*RefreshResponse, string, error) {
	claims, err := s.jwtManager.ValidateRefresh(refreshToken)
	if err != nil {
		return nil, "", fmt.Errorf("auth: invalid refresh token: %w", err)
	}

	// Verify user still exists
	user, err := s.userRepo.FindByEmail(ctx, claims.Email)
	if err != nil {
		return nil, "", fmt.Errorf("refresh: database error: %w", err)
	}
	if user == nil {
		return nil, "", fmt.Errorf("auth: user no longer exists")
	}

	// Consume the presented refresh token. Deleting it is the single-use gate:
	// if no row was removed, the token was already rotated or revoked (logout or
	// a replay), so we refuse to mint new tokens. The DELETE is atomic, so two
	// concurrent refreshes with the same token can't both succeed.
	deleted, err := s.tokenRepo.Delete(ctx, hashToken(refreshToken))
	if err != nil {
		return nil, "", fmt.Errorf("refresh: database error: %w", err)
	}
	if !deleted {
		return nil, "", fmt.Errorf("auth: refresh token is no longer valid")
	}

	// Issue a NEW token pair (rotation).
	newAccessToken, err := s.jwtManager.Generate(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, "", err
	}
	newRefreshToken, err := s.jwtManager.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, "", err
	}

	// Record the new refresh token so it is honored on the next refresh.
	if err := s.tokenRepo.Create(ctx, user.ID, hashToken(newRefreshToken), time.Now().Add(s.jwtManager.RefreshExpiry())); err != nil {
		return nil, "", fmt.Errorf("refresh: failed to persist refresh token: %w", err)
	}

	return &RefreshResponse{
		AccessToken: newAccessToken,
		TokenType:   "Bearer",
	}, newRefreshToken, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	// Validate the token is even real before accepting logout
	_, err := s.jwtManager.ValidateRefresh(refreshToken)
	if err != nil {
		return fmt.Errorf("auth: invalid token")
	}
	// Revoke it for real by removing it from the session store, so it can never
	// be used again. A token that isn't present (already logged out) is fine —
	// logout is idempotent, so we ignore the deleted flag.
	if _, err := s.tokenRepo.Delete(ctx, hashToken(refreshToken)); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}
