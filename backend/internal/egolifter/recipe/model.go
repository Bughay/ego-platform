package recipe

import "time"

// Recipe represents a user's recipe along with its ingredients.
type Recipe struct {
	ID          string       `json:"id"`
	UserID      string       `json:"user_id"`
	Name        string       `json:"name"`
	Notes       *string      `json:"notes,omitempty"`
	Ingredients []Ingredient `json:"ingredients"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Ingredient is a food used in a recipe, with the food's per-100g macros
// joined in so clients can compute totals: (weight_g / 100) * per_100_value.
type Ingredient struct {
	ID               string  `json:"id"`
	FoodID           string  `json:"food_id"`
	FoodName         string  `json:"food_name"`
	WeightG          float64 `json:"weight_g"`
	Notes            *string `json:"notes,omitempty"`
	Calories100      float64 `json:"calories_100"`
	Protein100       float64 `json:"protein_100"`
	Carbohydrates100 float64 `json:"carbohydrates_100"`
	Fat100           float64 `json:"fat_100"`
}

// RecipeFood is an ingredient's food with its macros already adjusted for the
// ingredient's weight: total = (weight_g / 100) * per_100_value.
type RecipeFood struct {
	FoodID             string  `json:"food_id"`
	FoodName           string  `json:"food_name"`
	WeightG            float64 `json:"weight_g"`
	Notes              *string `json:"notes,omitempty"`
	TotalCalories      float64 `json:"total_calories"`
	TotalProtein       float64 `json:"total_protein"`
	TotalCarbohydrates float64 `json:"total_carbohydrates"`
	TotalFat           float64 `json:"total_fat"`
}

// --- Request Payloads (Incoming) ---

type IngredientInput struct {
	FoodID  string  `json:"food_id"`
	WeightG float64 `json:"weight_g"`
	Notes   *string `json:"notes,omitempty"`
}

type CreateRecipeRequest struct {
	Name        string            `json:"name"`
	Notes       *string           `json:"notes,omitempty"`
	Ingredients []IngredientInput `json:"ingredients"`
}

type UpdateRecipeRequest struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Notes       *string           `json:"notes,omitempty"`
	Ingredients []IngredientInput `json:"ingredients"`
}
