package recipe

import (
	"fmt"
	"strings"
)

const (
	maxNameLength = 100
	maxWeightG    = 5000
)

func validateRecipeInput(name string, ingredients []IngredientInput) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("validation: recipe name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("validation: recipe name must be at most %d characters", maxNameLength)
	}
	if len(ingredients) == 0 {
		return fmt.Errorf("validation: a recipe must contain at least one ingredient")
	}
	for i, ing := range ingredients {
		if strings.TrimSpace(ing.FoodID) == "" {
			return fmt.Errorf("validation: ingredient %d: food_id is required", i)
		}
		if ing.WeightG <= 0 {
			return fmt.Errorf("validation: ingredient %d: weight_g must be greater than zero", i)
		}
		if ing.WeightG > maxWeightG {
			return fmt.Errorf("validation: ingredient %d: weight_g must be at most %d", i, maxWeightG)
		}
	}
	return nil
}
