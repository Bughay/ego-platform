package recipe

import (
	"context"
	"fmt"
	"strings"
)

// RecipeService defines the contract for recipe business logic.
type RecipeService interface {
	CreateRecipe(ctx context.Context, userID string, req *CreateRecipeRequest) (*Recipe, error)
	GetRecipe(ctx context.Context, userID, id string) (*Recipe, error)
	ListRecipes(ctx context.Context, userID string) ([]Recipe, error)
	UpdateRecipe(ctx context.Context, userID string, req *UpdateRecipeRequest) (*Recipe, error)
	DeleteRecipe(ctx context.Context, userID, id string) (bool, error)
	GetRecipeFoods(ctx context.Context, userID, id string) ([]RecipeFood, error)
}

type recipeService struct {
	recipeRepo RecipeRepository
}

// NewRecipeService creates a new RecipeService.
func NewRecipeService(recipeRepo RecipeRepository) RecipeService {
	return &recipeService{recipeRepo: recipeRepo}
}

func (s *recipeService) CreateRecipe(ctx context.Context, userID string, req *CreateRecipeRequest) (*Recipe, error) {
	if err := validateRecipeInput(req.Name, req.Ingredients); err != nil {
		return nil, err
	}
	return s.recipeRepo.Create(ctx, userID, req)
}

func (s *recipeService) GetRecipe(ctx context.Context, userID, id string) (*Recipe, error) {
	return s.recipeRepo.FindByID(ctx, userID, id)
}

func (s *recipeService) ListRecipes(ctx context.Context, userID string) ([]Recipe, error) {
	return s.recipeRepo.List(ctx, userID)
}

func (s *recipeService) UpdateRecipe(ctx context.Context, userID string, req *UpdateRecipeRequest) (*Recipe, error) {
	if strings.TrimSpace(req.ID) == "" {
		return nil, fmt.Errorf("validation: recipe id is required")
	}
	if err := validateRecipeInput(req.Name, req.Ingredients); err != nil {
		return nil, err
	}
	return s.recipeRepo.Update(ctx, userID, req)
}

// scaleByWeight converts a per-100g value to the total for weightG grams.
func scaleByWeight(per100, weightG float64) float64 {
	return per100 * weightG / 100
}

// GetRecipeFoods returns the recipe's foods with macros adjusted for each
// ingredient's weight. A nil slice with nil error means the recipe was not found.
func (s *recipeService) GetRecipeFoods(ctx context.Context, userID, id string) ([]RecipeFood, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("validation: recipe id is required")
	}

	foods, err := s.recipeRepo.GetFoods(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if len(foods) == 0 {
		// Distinguish "recipe has no ingredients" from "recipe doesn't exist".
		recipe, err := s.recipeRepo.FindByID(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if recipe == nil {
			return nil, nil
		}
		return []RecipeFood{}, nil
	}
	return foods, nil
}

func (s *recipeService) DeleteRecipe(ctx context.Context, userID, id string) (bool, error) {
	if strings.TrimSpace(id) == "" {
		return false, fmt.Errorf("validation: recipe id is required")
	}
	return s.recipeRepo.Delete(ctx, userID, id)
}
