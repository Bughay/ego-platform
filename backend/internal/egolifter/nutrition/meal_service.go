package nutrition

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// MealService defines the contract for meal business logic.
type MealService interface {
	CreateMeal(ctx context.Context, userID string, req *CreateMealRequest) (*Meal, error)
	GetMeal(ctx context.Context, userID, id string) (*Meal, error)
	ListMeals(ctx context.Context, userID string) ([]Meal, error)
	ListMealsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]Meal, error)
	DeleteMeal(ctx context.Context, userID, id string) (bool, error)
}

type mealService struct {
	mealRepo MealRepository
}

// NewMealService creates a new MealService.
func NewMealService(mealRepo MealRepository) MealService {
	return &mealService{mealRepo: mealRepo}
}

func (s *mealService) CreateMeal(ctx context.Context, userID string, req *CreateMealRequest) (*Meal, error) {
	if err := validateMealInput(req.Name, req.Foods); err != nil {
		return nil, err
	}
	return s.mealRepo.Create(ctx, userID, req)
}

func (s *mealService) GetMeal(ctx context.Context, userID, id string) (*Meal, error) {
	return s.mealRepo.FindByID(ctx, userID, id)
}

func (s *mealService) ListMeals(ctx context.Context, userID string) ([]Meal, error) {
	return s.mealRepo.List(ctx, userID)
}

// ListMealsByDateRange lists meals created between the given YYYY-MM-DD dates
// (inclusive); either bound defaults to today when empty.
func (s *mealService) ListMealsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]Meal, error) {
	parseOrToday := func(s string) (time.Time, error) {
		if s == "" {
			return time.Now(), nil
		}
		return time.Parse("2006-01-02", s)
	}

	from, err := parseOrToday(fromStr)
	if err != nil {
		return nil, err
	}
	to, err := parseOrToday(toStr)
	if err != nil {
		return nil, err
	}

	fy, fm, fd := from.Date()
	ty, tm, td := to.Date()
	if time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC).After(time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)) {
		return nil, fmt.Errorf("validation: date_from must not be after date_to")
	}

	return s.mealRepo.ListByDateRange(ctx, userID, from, to)
}

// DeleteMeal removes the meal and all of its consumed foods; the returned
// bool reports whether the meal existed.
func (s *mealService) DeleteMeal(ctx context.Context, userID, id string) (bool, error) {
	if strings.TrimSpace(id) == "" {
		return false, fmt.Errorf("validation: meal id is required")
	}
	return s.mealRepo.Delete(ctx, userID, id)
}

// --- Food resolution logic (used by the repository when logging a meal) ---

// toPer100 converts a consumed total for weightG grams into its per-100g value,
// rounded to 2 decimals so matching and storage are deterministic.
func toPer100(total, weightG float64) float64 {
	return round2(total / weightG * 100)
}

// round2 rounds a value to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// macrosEqual reports whether a saved food's per-100g macros equal the given values.
func macrosEqual(f Food, cal, prot, carb, fat float64) bool {
	return f.Calories100 == cal && f.Protein100 == prot &&
		f.Carbohydrates100 == carb && f.Fat100 == fat
}

// resolveMealFood matches one logged food against the user's saved catalog.
//
// It returns exactly one of:
//   - matchID: the id of an existing catalog food to reuse, or
//   - toCreate: a new *Food (named N, or the "N_2" variant) to insert for reuse, or
//   - err: when both N and N_2 already exist with different nutrition values.
//
// The input macros are totals for in.WeightG; they are converted to per-100g first.
func resolveMealFood(saved []Food, in MealFoodInput) (matchID string, toCreate *Food, err error) {
	cal := toPer100(in.Calories, in.WeightG)
	prot := toPer100(in.Protein, in.WeightG)
	carb := toPer100(in.Carbohydrates, in.WeightG)
	fat := toPer100(in.Fat, in.WeightG)

	// Guard the computed per-100g values against the food bounds so we never store
	// an out-of-range food in the catalog.
	if err := validateFoodInput(in.Name, cal, prot, carb, fat); err != nil {
		return "", nil, err
	}

	newFood := func(name string) *Food {
		return &Food{
			Name:             name,
			Calories100:      cal,
			Protein100:       prot,
			Carbohydrates100: carb,
			Fat100:           fat,
		}
	}

	// Exact match on the given name → reuse it.
	nameExists := false
	for _, f := range saved {
		if f.Name == in.Name {
			nameExists = true
			if macrosEqual(f, cal, prot, carb, fat) {
				return f.ID, nil, nil
			}
		}
	}
	if !nameExists {
		return "", newFood(in.Name), nil
	}

	// Name is taken with different macros → fall back to the "_2" variant.
	variant := in.Name + "_2"
	variantExists := false
	for _, f := range saved {
		if f.Name == variant {
			variantExists = true
			if macrosEqual(f, cal, prot, carb, fat) {
				return f.ID, nil, nil
			}
		}
	}
	if variantExists {
		return "", nil, fmt.Errorf(
			"validation: you already have two foods named %q (%q and %q) with different "+
				"nutrition values — please rename or update one before logging this food",
			in.Name, in.Name, variant)
	}
	return "", newFood(variant), nil
}
