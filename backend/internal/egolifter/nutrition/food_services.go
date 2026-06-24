package nutrition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Bughay/egolifter/internal/shared/cache"
)

// foodListTTL bounds how long a cached food list may live. The catalog is a
// per-user, rarely-changing reference list, and every write invalidates it, so a
// generous TTL is safe — it only backstops a missed invalidation.
const foodListTTL = 30 * time.Minute

// foodListKey is the only place that knows the food-list cache key format.
// Foods are per-user (never global), so the userID is part of the key.
func foodListKey(userID string) string {
	return fmt.Sprintf("foods:user:%s", userID)
}

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
	cache    cache.Cache  // shared cache layer (cache-aside for ListFoods)
	logger   *slog.Logger // for cache observability
}

// NewNutritionService creates a new NutritionService.
func NewNutritionService(foodRepo FoodRepository, c cache.Cache, logger *slog.Logger) NutritionService {
	return &nutritionService{foodRepo: foodRepo, cache: c, logger: logger}
}

func (s *nutritionService) CreateFood(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error) {
	if err := validateFoodInput(req.Name, req.Calories100, req.Protein100, req.Carbohydrates100, req.Fat100); err != nil {
		return nil, err
	}
	food, err := s.foodRepo.Create(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	s.invalidateFoodList(ctx, userID)
	return food, nil
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
	foods, err := s.foodRepo.CreateMany(ctx, userID, reqs)
	if err != nil {
		return nil, err
	}
	s.invalidateFoodList(ctx, userID)
	return foods, nil
}

func (s *nutritionService) GetFood(ctx context.Context, userID, id string) (*Food, error) {
	if err := validateFoodID(id); err != nil {
		return nil, err
	}
	return s.foodRepo.FindByID(ctx, userID, id)
}

// ListFoods returns the user's food catalog, cache-aside: serve from Redis on a
// hit, otherwise read the DB (the source of truth) and populate the cache. A
// cache miss is normal; a real Redis error is logged and falls through to the DB
// so the request never fails because of the cache.
func (s *nutritionService) ListFoods(ctx context.Context, userID string) ([]Food, error) {
	key := foodListKey(userID)

	if cached, err := s.cache.Get(ctx, key); err == nil {
		var foods []Food
		if jsonErr := json.Unmarshal([]byte(cached), &foods); jsonErr == nil {
			s.logger.DebugContext(ctx, "cache hit", "key", key)
			return foods, nil
		}
		// Unmarshal failed (e.g. stale schema) — fall through and re-populate.
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		s.logger.WarnContext(ctx, "cache get error", "key", key, "error", err)
	} else {
		s.logger.DebugContext(ctx, "cache miss", "key", key)
	}

	foods, err := s.foodRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	if setErr := s.cache.Set(ctx, key, foods, foodListTTL); setErr != nil {
		s.logger.WarnContext(ctx, "cache set failed", "key", key, "error", setErr)
	}
	return foods, nil
}

func (s *nutritionService) UpdateFood(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error) {
	if err := validateFoodID(req.ID); err != nil {
		return nil, err
	}
	if err := validateFoodInput(req.Name, req.Calories100, req.Protein100, req.Carbohydrates100, req.Fat100); err != nil {
		return nil, err
	}
	food, err := s.foodRepo.Update(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	s.invalidateFoodList(ctx, userID)
	return food, nil
}

func (s *nutritionService) DeleteFood(ctx context.Context, userID, id string) error {
	if err := validateFoodID(id); err != nil {
		return err
	}
	if err := s.foodRepo.Delete(ctx, userID, id); err != nil {
		return err
	}
	s.invalidateFoodList(ctx, userID)
	return nil
}

// invalidateFoodList deletes the user's cached food list after a write. The DB is
// already updated (source of truth) so the next ListFoods misses and repopulates
// from fresh data. Non-fatal: a Redis failure is logged, not returned — the
// 30-minute TTL is the backstop.
func (s *nutritionService) invalidateFoodList(ctx context.Context, userID string) {
	key := foodListKey(userID)
	if err := s.cache.Delete(ctx, key); err != nil {
		s.logger.WarnContext(ctx, "food list cache invalidation failed", "key", key, "error", err)
	}
}
