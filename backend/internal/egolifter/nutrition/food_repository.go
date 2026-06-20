package nutrition

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FoodRepository defines the contract for Food data access.
type FoodRepository interface {
	Create(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error)
	CreateMany(ctx context.Context, userID string, reqs []*CreateFoodRequest) ([]Food, error)
	FindByID(ctx context.Context, userID, id string) (*Food, error)
	List(ctx context.Context, userID string) ([]Food, error)
	Update(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error)
	Delete(ctx context.Context, userID, id string) error
}

type pgFoodRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewFoodRepository creates a new PostgreSQL-backed FoodRepository wrapping the sqlc-generated queries.
func NewFoodRepository(pool *pgxpool.Pool) FoodRepository {
	return &pgFoodRepository{pool: pool, queries: db.New(pool)}
}

func (r *pgFoodRepository) Create(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error) {
	row, err := r.queries.CreateFood(ctx, db.CreateFoodParams{
		UserID:           userID,
		Name:             req.Name,
		Calories100:      req.Calories100,
		Protein100:       req.Protein100,
		Carbohydrates100: req.Carbohydrates100,
		Fat100:           req.Fat100,
	})
	if err != nil {
		return nil, fmt.Errorf("foodRepo.Create: %w", err)
	}
	return toFood(row), nil
}

// CreateMany inserts every food in a single transaction: either all rows are
// created or none are. The returned foods keep the input order so callers can
// correlate each created food (with its new id) back to its request.
func (r *pgFoodRepository) CreateMany(ctx context.Context, userID string, reqs []*CreateFoodRequest) ([]Food, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("foodRepo.CreateMany: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	foods := make([]Food, 0, len(reqs))
	for _, req := range reqs {
		row, err := qtx.CreateFood(ctx, db.CreateFoodParams{
			UserID:           userID,
			Name:             req.Name,
			Calories100:      req.Calories100,
			Protein100:       req.Protein100,
			Carbohydrates100: req.Carbohydrates100,
			Fat100:           req.Fat100,
		})
		if err != nil {
			return nil, fmt.Errorf("foodRepo.CreateMany: create food %q: %w", req.Name, err)
		}
		foods = append(foods, *toFood(row))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("foodRepo.CreateMany: commit: %w", err)
	}
	return foods, nil
}

func (r *pgFoodRepository) FindByID(ctx context.Context, userID, id string) (*Food, error) {
	row, err := r.queries.GetFoodByID(ctx, db.GetFoodByIDParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is not an error at this layer
		}
		return nil, fmt.Errorf("foodRepo.FindByID: %w", err)
	}
	return toFood(row), nil
}

func (r *pgFoodRepository) List(ctx context.Context, userID string) ([]Food, error) {
	rows, err := r.queries.ListFoods(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("foodRepo.List: %w", err)
	}
	foods := make([]Food, 0, len(rows))
	for _, row := range rows {
		foods = append(foods, *toFood(row))
	}
	return foods, nil
}

func (r *pgFoodRepository) Update(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error) {
	row, err := r.queries.UpdateFood(ctx, db.UpdateFoodParams{
		ID:               req.ID,
		UserID:           userID,
		Name:             req.Name,
		Calories100:      req.Calories100,
		Protein100:       req.Protein100,
		Carbohydrates100: req.Carbohydrates100,
		Fat100:           req.Fat100,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("foodRepo.Update: %w", err)
	}
	return toFood(row), nil
}

func (r *pgFoodRepository) Delete(ctx context.Context, userID, id string) error {
	if err := r.queries.DeleteFood(ctx, db.DeleteFoodParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("foodRepo.Delete: %w", err)
	}
	return nil
}

// toFood maps a sqlc-generated row to the domain model.
func toFood(row db.Food) *Food {
	return &Food{
		ID:               row.ID,
		UserID:           row.UserID,
		Name:             row.Name,
		Calories100:      row.Calories100,
		Protein100:       row.Protein100,
		Carbohydrates100: row.Carbohydrates100,
		Fat100:           row.Fat100,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
