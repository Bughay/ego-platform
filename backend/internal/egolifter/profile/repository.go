package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProfileRepository defines the contract for user profile data access.
type ProfileRepository interface {
	Get(ctx context.Context, userID string) (*Profile, error)
	Upsert(ctx context.Context, userID string, req *UpdateProfileRequest) (*Profile, error)
}

type pgProfileRepository struct {
	queries *db.Queries
}

// NewProfileRepository creates a new PostgreSQL-backed ProfileRepository wrapping the sqlc-generated queries.
func NewProfileRepository(pool *pgxpool.Pool) ProfileRepository {
	return &pgProfileRepository{queries: db.New(pool)}
}

// Get returns the user's profile. A user who has never saved one has no row yet,
// so a zero-value profile is returned (not an error) so the form starts empty.
func (r *pgProfileRepository) Get(ctx context.Context, userID string) (*Profile, error) {
	row, err := r.queries.GetUserProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Profile{}, nil
		}
		return nil, fmt.Errorf("profileRepo.Get: %w", err)
	}
	return toProfile(row), nil
}

// Upsert creates or updates the user's profile row and returns the saved values.
func (r *pgProfileRepository) Upsert(ctx context.Context, userID string, req *UpdateProfileRequest) (*Profile, error) {
	row, err := r.queries.UpsertUserProfile(ctx, db.UpsertUserProfileParams{
		UserID:    userID,
		FirstName: pgText(req.FirstName),
		LastName:  pgText(req.LastName),
		HeightCm:  pgFloat(req.HeightCm),
		WeightKg:  pgFloat(req.WeightKg),
	})
	if err != nil {
		return nil, fmt.Errorf("profileRepo.Upsert: %w", err)
	}
	return toProfile(row), nil
}

// toProfile maps the sqlc row (nullable columns) to the clean domain model,
// using empty string / 0 for NULLs.
func toProfile(row db.UserProfile) *Profile {
	p := &Profile{}
	if row.FirstName.Valid {
		p.FirstName = row.FirstName.String
	}
	if row.LastName.Valid {
		p.LastName = row.LastName.String
	}
	if row.HeightCm.Valid {
		p.HeightCm = row.HeightCm.Float64
	}
	if row.WeightKg.Valid {
		p.WeightKg = row.WeightKg.Float64
	}
	return p
}

// pgText wraps a string for a nullable text column, storing NULL when empty.
func pgText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// pgFloat wraps a value for a nullable float column, storing NULL when zero.
func pgFloat(v float64) pgtype.Float8 {
	if v == 0 {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: v, Valid: true}
}
