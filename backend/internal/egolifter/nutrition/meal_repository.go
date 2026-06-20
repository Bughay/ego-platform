package nutrition

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MealRepository defines the contract for meal data access.
type MealRepository interface {
	Create(ctx context.Context, userID string, req *CreateMealRequest) (*Meal, error)
	FindByID(ctx context.Context, userID, id string) (*Meal, error)
	List(ctx context.Context, userID string) ([]Meal, error)
	ListByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Meal, error)
	Delete(ctx context.Context, userID, id string) (bool, error)
}

type pgMealRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewMealRepository creates a new PostgreSQL-backed MealRepository wrapping the sqlc-generated queries.
func NewMealRepository(pool *pgxpool.Pool) MealRepository {
	return &pgMealRepository{pool: pool, queries: db.New(pool)}
}

// Create inserts the meal and its consumed foods in a single transaction.
func (r *pgMealRepository) Create(ctx context.Context, userID string, req *CreateMealRequest) (*Meal, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("mealRepo.Create: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateMeal(ctx, db.CreateMealParams{
		UserID: userID,
		Name:   req.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("mealRepo.Create: %w", err)
	}

	// Load the user's food catalog once; logged foods are matched against it and any
	// new food is saved for reuse (see resolveMealFood).
	savedRows, err := qtx.ListFoods(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("mealRepo.Create: list foods: %w", err)
	}
	saved := make([]Food, 0, len(savedRows))
	for _, fr := range savedRows {
		saved = append(saved, *toFood(fr))
	}

	for _, f := range req.Foods {
		foodID, toCreate, err := resolveMealFood(saved, f)
		if err != nil {
			return nil, err
		}
		if toCreate != nil {
			created, err := qtx.CreateFood(ctx, db.CreateFoodParams{
				UserID:           userID,
				Name:             toCreate.Name,
				Calories100:      toCreate.Calories100,
				Protein100:       toCreate.Protein100,
				Carbohydrates100: toCreate.Carbohydrates100,
				Fat100:           toCreate.Fat100,
			})
			if err != nil {
				return nil, fmt.Errorf("mealRepo.Create: create food %q: %w", toCreate.Name, err)
			}
			foodID = created.ID
			// Make the new food visible to later entries in this same meal.
			saved = append(saved, *toFood(created))
		}

		if _, err := qtx.CreateFoodConsumed(ctx, db.CreateFoodConsumedParams{
			UserID:  userID,
			MealID:  row.ID,
			FoodID:  foodID,
			WeightG: f.WeightG,
		}); err != nil {
			return nil, fmt.Errorf("mealRepo.Create: insert food consumed (food %q): %w", f.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("mealRepo.Create: commit: %w", err)
	}

	return r.FindByID(ctx, userID, row.ID)
}

func (r *pgMealRepository) FindByID(ctx context.Context, userID, id string) (*Meal, error) {
	row, err := r.queries.GetMealByID(ctx, db.GetMealByIDParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is not an error at this layer
		}
		return nil, fmt.Errorf("mealRepo.FindByID: %w", err)
	}

	foodRows, err := r.queries.ListFoodConsumedByMeal(ctx, db.ListFoodConsumedByMealParams{
		MealID: row.ID,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("mealRepo.FindByID: foods: %w", err)
	}

	meal := &Meal{
		ID:                 row.ID,
		UserID:             row.UserID,
		Name:               row.Name,
		Foods:              make([]ConsumedFood, 0, len(foodRows)),
		TotalCalories:      row.TotalCalories,
		TotalProtein:       row.TotalProtein,
		TotalCarbohydrates: row.TotalCarbohydrates,
		TotalFat:           row.TotalFat,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	for _, fr := range foodRows {
		meal.Foods = append(meal.Foods, ConsumedFood{
			ID:                 fr.ID,
			FoodID:             fr.FoodID,
			FoodName:           fr.FoodName,
			WeightG:            fr.WeightG,
			TotalCalories:      fr.TotalCalories,
			TotalProtein:       fr.TotalProtein,
			TotalCarbohydrates: fr.TotalCarbohydrates,
			TotalFat:           fr.TotalFat,
		})
	}
	return meal, nil
}

func (r *pgMealRepository) List(ctx context.Context, userID string) ([]Meal, error) {
	rows, err := r.queries.ListMeals(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("mealRepo.List: %w", err)
	}
	meals := make([]Meal, 0, len(rows))
	for _, row := range rows {
		meals = append(meals, Meal{
			ID:                 row.ID,
			UserID:             row.UserID,
			Name:               row.Name,
			Foods:              []ConsumedFood{},
			TotalCalories:      row.TotalCalories,
			TotalProtein:       row.TotalProtein,
			TotalCarbohydrates: row.TotalCarbohydrates,
			TotalFat:           row.TotalFat,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return meals, nil
}

// Delete removes the meal and all of its food_consumed rows; the returned
// bool reports whether a meal was actually deleted.
func (r *pgMealRepository) Delete(ctx context.Context, userID, id string) (bool, error) {
	rows, err := r.queries.DeleteMeal(ctx, db.DeleteMealParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("mealRepo.Delete: %w", err)
	}
	return rows > 0, nil
}

func (r *pgMealRepository) ListByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Meal, error) {
	rows, err := r.queries.ListMealsByDateRange(ctx, db.ListMealsByDateRangeParams{
		UserID:   userID,
		DateFrom: pgtype.Date{Time: from, Valid: true},
		DateTo:   pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("mealRepo.ListByDateRange: %w", err)
	}
	meals := make([]Meal, 0, len(rows))
	for _, row := range rows {
		meals = append(meals, Meal{
			ID:                 row.ID,
			UserID:             row.UserID,
			Name:               row.Name,
			Foods:              []ConsumedFood{},
			TotalCalories:      row.TotalCalories,
			TotalProtein:       row.TotalProtein,
			TotalCarbohydrates: row.TotalCarbohydrates,
			TotalFat:           row.TotalFat,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return meals, nil
}
