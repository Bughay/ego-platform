package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshTokenRepository defines the contract for refresh-token session storage.
// A refresh token is only honored while a matching row exists here (an allowlist),
// which is what makes logout and single-use rotation possible despite JWTs being
// stateless. Only a SHA-256 hash of the token is stored, never the token itself.
type RefreshTokenRepository interface {
	// Create records a freshly issued refresh token (by its hash) so it becomes valid.
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// Delete removes the token (logout, or the old token during rotation) and
	// reports whether a row was actually removed. A false return means the token
	// was not in the store, which callers use as the revocation / single-use gate.
	Delete(ctx context.Context, tokenHash string) (bool, error)
}

type pgRefreshTokenRepository struct {
	queries *db.Queries
}

// NewRefreshTokenRepository creates a PostgreSQL-backed RefreshTokenRepository
// wrapping the sqlc-generated queries.
func NewRefreshTokenRepository(pool *pgxpool.Pool) RefreshTokenRepository {
	return &pgRefreshTokenRepository{queries: db.New(pool)}
}

func (r *pgRefreshTokenRepository) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if err := r.queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("refreshTokenRepo.Create: %w", err)
	}
	return nil
}

func (r *pgRefreshTokenRepository) Delete(ctx context.Context, tokenHash string) (bool, error) {
	rows, err := r.queries.DeleteRefreshToken(ctx, tokenHash)
	if err != nil {
		return false, fmt.Errorf("refreshTokenRepo.Delete: %w", err)
	}
	return rows > 0, nil
}
