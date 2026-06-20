package analytics

// Summary is the combined nutrition + training report for a date range.
type Summary struct {
	DateFrom  string           `json:"date_from"`
	DateTo    string           `json:"date_to"`
	Days      int              `json:"days"` // inclusive calendar days in the range
	Nutrition NutritionSummary `json:"nutrition"`
	Training  TrainingSummary  `json:"training"`
}

// NutritionSummary aggregates a user's logged meals over the range.
type NutritionSummary struct {
	TotalCalories      float64    `json:"total_calories"`
	TotalProtein       float64    `json:"total_protein"`
	TotalCarbohydrates float64    `json:"total_carbohydrates"`
	TotalFat           float64    `json:"total_fat"`
	MealsLogged        int        `json:"meals_logged"`
	DaysLogged         int        `json:"days_logged"` // distinct dates with >=1 meal
	DailyAvg           MacroDaily `json:"daily_avg"`   // totals divided by Days
}

// MacroDaily holds the per-day average macros across the range.
type MacroDaily struct {
	Calories      float64 `json:"calories"`
	Protein       float64 `json:"protein"`
	Carbohydrates float64 `json:"carbohydrates"`
	Fat           float64 `json:"fat"`
}

// TrainingSummary aggregates a user's performed workouts over the range.
type TrainingSummary struct {
	Workouts      int     `json:"workouts"`
	TotalSets     int     `json:"total_sets"`      // count of exercise rows
	TotalReps     int     `json:"total_reps"`      // sum of Exercise.Reps
	TotalVolumeKg float64 `json:"total_volume_kg"` // sum(WeightKg * Reps)
	DaysTrained   int     `json:"days_trained"`    // distinct dates with >=1 workout
}
