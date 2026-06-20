package nutrition

import (
	"context"
	"fmt"
)

// NutritionService defines the contract for nutrition business logic.
type NutritionService interface {
	CreateFood(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error)
	CreateFoods(ctx context.Context, userID string, reqs []*CreateFoodRequest) ([]Food, error)
	GetFood(ctx context.Context, userID, id string) (*Food, error)
	ListFoods(ctx context.Context, userID string) ([]Food, error)
	UpdateFood(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error)
	DeleteFood(ctx context.Context, userID, id string) error
}

type nutritionService struct {
	foodRepo FoodRepository
}

// NewNutritionService creates a new NutritionService.
func NewNutritionService(foodRepo FoodRepository) NutritionService {
	return &nutritionService{foodRepo: foodRepo}
}

func (s *nutritionService) CreateFood(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error) {
	if err := validateFoodInput(req.Name, req.Calories100, req.Protein100, req.Carbohydrates100, req.Fat100); err != nil {
		return nil, err
	}
	return s.foodRepo.Create(ctx, userID, req)
}

// CreateFoods validates every food up front (so a single bad food fails before
// any transaction opens — no partial inserts) then creates them all in one
// transaction, returning the created foods (with their new ids) in input order.
func (s *nutritionService) CreateFoods(ctx context.Context, userID string, reqs []*CreateFoodRequest) ([]Food, error) {
	if len(reqs) == 0 {
		return nil, fmt.Errorf("validation: at least one food is required")
	}
	for i, req := range reqs {
		if err := validateFoodInput(req.Name, req.Calories100, req.Protein100, req.Carbohydrates100, req.Fat100); err != nil {
			return nil, fmt.Errorf("food %d: %w", i, err)
		}
	}
	return s.foodRepo.CreateMany(ctx, userID, reqs)
}

func (s *nutritionService) GetFood(ctx context.Context, userID, id string) (*Food, error) {
	if err := validateFoodID(id); err != nil {
		return nil, err
	}
	return s.foodRepo.FindByID(ctx, userID, id)
}

func (s *nutritionService) ListFoods(ctx context.Context, userID string) ([]Food, error) {
	return s.foodRepo.List(ctx, userID)
}

func (s *nutritionService) UpdateFood(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error) {
	if err := validateFoodID(req.ID); err != nil {
		return nil, err
	}
	if err := validateFoodInput(req.Name, req.Calories100, req.Protein100, req.Carbohydrates100, req.Fat100); err != nil {
		return nil, err
	}
	return s.foodRepo.Update(ctx, userID, req)
}

func (s *nutritionService) DeleteFood(ctx context.Context, userID, id string) error {
	if err := validateFoodID(id); err != nil {
		return err
	}
	return s.foodRepo.Delete(ctx, userID, id)
}
