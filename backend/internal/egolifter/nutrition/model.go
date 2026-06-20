package nutrition

import "time"

// Food represents a food item with macros stored per 100g.
type Food struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Name             string    `json:"name"`
	Calories100      float64   `json:"calories_100"`
	Protein100       float64   `json:"protein_100"`
	Carbohydrates100 float64   `json:"carbohydrates_100"`
	Fat100           float64   `json:"fat_100"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Meal represents a logged meal with its consumed foods and aggregate macros.
type Meal struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	Name               string         `json:"name"`
	Foods              []ConsumedFood `json:"foods"`
	TotalCalories      float64        `json:"total_calories"`
	TotalProtein       float64        `json:"total_protein"`
	TotalCarbohydrates float64        `json:"total_carbohydrates"`
	TotalFat           float64        `json:"total_fat"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// ConsumedFood is one food row in a meal, with totals denormalized at insert time.
type ConsumedFood struct {
	ID                 string  `json:"id"`
	FoodID             string  `json:"food_id"`
	FoodName           string  `json:"food_name"`
	WeightG            float64 `json:"weight_g"`
	TotalCalories      float64 `json:"total_calories"`
	TotalProtein       float64 `json:"total_protein"`
	TotalCarbohydrates float64 `json:"total_carbohydrates"`
	TotalFat           float64 `json:"total_fat"`
}

// --- Request Payloads (Incoming) ---

type CreateFoodRequest struct {
	Name             string  `json:"name"`
	Calories100      float64 `json:"calories_100"`
	Protein100       float64 `json:"protein_100"`
	Carbohydrates100 float64 `json:"carbohydrates_100"`
	Fat100           float64 `json:"fat_100"`
}

type UpdateFoodRequest struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Calories100      float64 `json:"calories_100"`
	Protein100       float64 `json:"protein_100"`
	Carbohydrates100 float64 `json:"carbohydrates_100"`
	Fat100           float64 `json:"fat_100"`
}

// MealFoodInput is one food entry in a meal creation request. The macros are the
// totals actually consumed for WeightG (not per-100g); the server converts them to
// per-100g to match or create the corresponding food in the user's catalog.
type MealFoodInput struct {
	Name          string  `json:"name"`
	WeightG       float64 `json:"weight_g"`
	Calories      float64 `json:"calories"`
	Protein       float64 `json:"protein"`
	Carbohydrates float64 `json:"carbohydrates"`
	Fat           float64 `json:"fat"`
}

type CreateMealRequest struct {
	Name  string          `json:"name"`
	Foods []MealFoodInput `json:"foods"`
}
