package recipe

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RecipeRepository defines the contract for Recipe data access.
type RecipeRepository interface {
	Create(ctx context.Context, userID string, req *CreateRecipeRequest) (*Recipe, error)
	FindByID(ctx context.Context, userID, id string) (*Recipe, error)
	List(ctx context.Context, userID string) ([]Recipe, error)
	Update(ctx context.Context, userID string, req *UpdateRecipeRequest) (*Recipe, error)
	Delete(ctx context.Context, userID, id string) (bool, error)
	GetFoods(ctx context.Context, userID, recipeID string) ([]RecipeFood, error)
}

type pgRecipeRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewRecipeRepository creates a new PostgreSQL-backed RecipeRepository wrapping the sqlc-generated queries.
func NewRecipeRepository(pool *pgxpool.Pool) RecipeRepository {
	return &pgRecipeRepository{pool: pool, queries: db.New(pool)}
}

// Create inserts the recipe and its ingredients in a single transaction.
func (r *pgRecipeRepository) Create(ctx context.Context, userID string, req *CreateRecipeRequest) (*Recipe, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recipeRepo.Create: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateRecipe(ctx, db.CreateRecipeParams{
		UserID: userID,
		Name:   req.Name,
		Notes:  toPgText(req.Notes),
	})
	if err != nil {
		return nil, fmt.Errorf("recipeRepo.Create: %w", err)
	}

	if err := insertIngredients(ctx, qtx, row.ID, req.Ingredients); err != nil {
		return nil, fmt.Errorf("recipeRepo.Create: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recipeRepo.Create: commit: %w", err)
	}

	return r.FindByID(ctx, userID, row.ID)
}

func (r *pgRecipeRepository) FindByID(ctx context.Context, userID, id string) (*Recipe, error) {
	row, err := r.queries.GetRecipeByID(ctx, db.GetRecipeByIDParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is not an error at this layer
		}
		return nil, fmt.Errorf("recipeRepo.FindByID: %w", err)
	}

	recipe := toRecipe(row)
	if err := r.attachIngredients(ctx, recipe); err != nil {
		return nil, fmt.Errorf("recipeRepo.FindByID: %w", err)
	}
	return recipe, nil
}

// attachIngredients loads the recipe's ingredients (with the food's per-100g
// macros joined in) and sets them on the domain model.
func (r *pgRecipeRepository) attachIngredients(ctx context.Context, recipe *Recipe) error {
	ingredientRows, err := r.queries.ListRecipeIngredients(ctx, recipe.ID)
	if err != nil {
		return fmt.Errorf("ingredients: %w", err)
	}

	recipe.Ingredients = make([]Ingredient, 0, len(ingredientRows))
	for _, ir := range ingredientRows {
		recipe.Ingredients = append(recipe.Ingredients, Ingredient{
			ID:               ir.ID,
			FoodID:           ir.FoodID,
			FoodName:         ir.FoodName,
			WeightG:          ir.WeightG,
			Notes:            fromPgText(ir.Notes),
			Calories100:      ir.Calories100,
			Protein100:       ir.Protein100,
			Carbohydrates100: ir.Carbohydrates100,
			Fat100:           ir.Fat100,
		})
	}
	return nil
}

func (r *pgRecipeRepository) List(ctx context.Context, userID string) ([]Recipe, error) {
	rows, err := r.queries.ListRecipes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("recipeRepo.List: %w", err)
	}
	recipes := make([]Recipe, 0, len(rows))
	for _, row := range rows {
		recipe := toRecipe(row)
		if err := r.attachIngredients(ctx, recipe); err != nil {
			return nil, fmt.Errorf("recipeRepo.List: %w", err)
		}
		recipes = append(recipes, *recipe)
	}
	return recipes, nil
}

// GetFoods returns the foods used by the recipe's ingredients with their
// macros adjusted for each ingredient's weight.
func (r *pgRecipeRepository) GetFoods(ctx context.Context, userID, recipeID string) ([]RecipeFood, error) {
	rows, err := r.queries.GetRecipeFoods(ctx, db.GetRecipeFoodsParams{
		RecipeID: recipeID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("recipeRepo.GetFoods: %w", err)
	}
	foods := make([]RecipeFood, 0, len(rows))
	for _, row := range rows {
		foods = append(foods, RecipeFood{
			FoodID:             row.FoodID,
			FoodName:           row.FoodName,
			WeightG:            row.WeightG,
			Notes:              fromPgText(row.Notes),
			TotalCalories:      scaleByWeight(row.Calories100, row.WeightG),
			TotalProtein:       scaleByWeight(row.Protein100, row.WeightG),
			TotalCarbohydrates: scaleByWeight(row.Carbohydrates100, row.WeightG),
			TotalFat:           scaleByWeight(row.Fat100, row.WeightG),
		})
	}
	return foods, nil
}

// Update modifies the recipe and replaces its ingredient list in a single transaction.
func (r *pgRecipeRepository) Update(ctx context.Context, userID string, req *UpdateRecipeRequest) (*Recipe, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("recipeRepo.Update: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	row, err := qtx.UpdateRecipe(ctx, db.UpdateRecipeParams{
		ID:     req.ID,
		UserID: userID,
		Name:   req.Name,
		Notes:  toPgText(req.Notes),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("recipeRepo.Update: %w", err)
	}

	if err := qtx.DeleteRecipeIngredientsByRecipe(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("recipeRepo.Update: clear ingredients: %w", err)
	}
	if err := insertIngredients(ctx, qtx, row.ID, req.Ingredients); err != nil {
		return nil, fmt.Errorf("recipeRepo.Update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("recipeRepo.Update: commit: %w", err)
	}

	return r.FindByID(ctx, userID, row.ID)
}

// Delete removes the recipe and all of its recipe_ingredients rows; the
// returned bool reports whether a recipe was actually deleted.
func (r *pgRecipeRepository) Delete(ctx context.Context, userID, id string) (bool, error) {
	rows, err := r.queries.DeleteRecipe(ctx, db.DeleteRecipeParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("recipeRepo.Delete: %w", err)
	}
	return rows > 0, nil
}

func insertIngredients(ctx context.Context, q *db.Queries, recipeID string, ingredients []IngredientInput) error {
	for _, ing := range ingredients {
		if _, err := q.CreateRecipeIngredient(ctx, db.CreateRecipeIngredientParams{
			RecipeID: recipeID,
			FoodID:   ing.FoodID,
			WeightG:  ing.WeightG,
			Notes:    toPgText(ing.Notes),
		}); err != nil {
			return fmt.Errorf("insert ingredient (food %s): %w", ing.FoodID, err)
		}
	}
	return nil
}

// toRecipe maps a sqlc-generated row to the domain model (without ingredients).
func toRecipe(row db.Recipe) *Recipe {
	return &Recipe{
		ID:          row.ID,
		UserID:      row.UserID,
		Name:        row.Name,
		Notes:       fromPgText(row.Notes),
		Ingredients: []Ingredient{},
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toPgText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func fromPgText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
