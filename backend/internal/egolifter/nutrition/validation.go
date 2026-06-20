package nutrition

import (
	"fmt"
	"strings"
)

const (
	maxNameLength  = 100
	maxWeightG     = 5000
	maxCalories100 = 900
	maxMacro100    = 100
)

func validateMealInput(name string, foods []MealFoodInput) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("validation: meal name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("validation: meal name must be at most %d characters", maxNameLength)
	}
	if len(foods) == 0 {
		return fmt.Errorf("validation: a meal must contain at least one food")
	}
	for i, f := range foods {
		if strings.TrimSpace(f.Name) == "" {
			return fmt.Errorf("validation: food %d: name is required", i)
		}
		if len(f.Name) > maxNameLength {
			return fmt.Errorf("validation: food %d: name must be at most %d characters", i, maxNameLength)
		}
		if f.WeightG <= 0 {
			return fmt.Errorf("validation: food %d: weight_g must be greater than zero", i)
		}
		if f.WeightG > maxWeightG {
			return fmt.Errorf("validation: food %d: weight_g must be at most %d", i, maxWeightG)
		}
		if f.Calories < 0 || f.Protein < 0 || f.Carbohydrates < 0 || f.Fat < 0 {
			return fmt.Errorf("validation: food %d: macros must not be negative", i)
		}
	}
	return nil
}

func validateFoodInput(name string, calories, protein, carbs, fat float64) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("validation: food name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("validation: food name must be at most %d characters", maxNameLength)
	}
	if calories < 0 || calories > maxCalories100 {
		return fmt.Errorf("validation: calories_100 must be between 0 and %d", maxCalories100)
	}
	if protein < 0 || protein > maxMacro100 {
		return fmt.Errorf("validation: protein_100 must be between 0 and %d", maxMacro100)
	}
	if carbs < 0 || carbs > maxMacro100 {
		return fmt.Errorf("validation: carbohydrates_100 must be between 0 and %d", maxMacro100)
	}
	if fat < 0 || fat > maxMacro100 {
		return fmt.Errorf("validation: fat_100 must be between 0 and %d", maxMacro100)
	}
	return nil
}

func validateFoodID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("validation: food id is required")
	}
	return nil
}
