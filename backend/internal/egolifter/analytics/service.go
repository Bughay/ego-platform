package analytics

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Bughay/egolifter/internal/egolifter/nutrition"
	"github.com/Bughay/egolifter/internal/egolifter/training"
)

// dateLayout is the YYYY-MM-DD format shared with the meal/training endpoints.
const dateLayout = "2006-01-02"

// AnalyticsService defines the contract for analytics business logic.
type AnalyticsService interface {
	GetSummary(ctx context.Context, userID, fromStr, toStr string) (*Summary, error)
}

// analyticsService composes the existing nutrition and training services and
// aggregates their date-range results in memory. It owns no tables of its own.
type analyticsService struct {
	mealSvc     nutrition.MealService
	trainingSvc training.TrainingService
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(mealSvc nutrition.MealService, trainingSvc training.TrainingService) AnalyticsService {
	return &analyticsService{mealSvc: mealSvc, trainingSvc: trainingSvc}
}

// GetSummary builds a combined nutrition + training summary for the given
// YYYY-MM-DD range (inclusive); either bound defaults to today when empty.
func (s *analyticsService) GetSummary(ctx context.Context, userID, fromStr, toStr string) (*Summary, error) {
	from, err := parseOrToday(fromStr)
	if err != nil {
		return nil, err
	}
	to, err := parseOrToday(toStr)
	if err != nil {
		return nil, err
	}

	fromDay := truncateDay(from)
	toDay := truncateDay(to)
	if fromDay.After(toDay) {
		return nil, fmt.Errorf("validation: date_from must not be after date_to")
	}
	days := int(toDay.Sub(fromDay).Hours()/24) + 1

	meals, err := s.mealSvc.ListMealsByDateRange(ctx, userID, fromStr, toStr)
	if err != nil {
		return nil, err
	}
	workouts, err := s.trainingSvc.ListWorkoutsByDateRange(ctx, userID, fromStr, toStr)
	if err != nil {
		return nil, err
	}

	return &Summary{
		DateFrom:  fromDay.Format(dateLayout),
		DateTo:    toDay.Format(dateLayout),
		Days:      days,
		Nutrition: summarizeMeals(meals),
		Training:  summarizeWorkouts(workouts),
	}, nil
}

// summarizeMeals sums macros across the meals and derives per-day averages.
// Averages divide by the number of days that actually had logged meals so that
// unlogged days don't drag the average down; with no meals the averages are 0.
func summarizeMeals(meals []nutrition.Meal) NutritionSummary {
	loggedDays := map[string]struct{}{}
	var sum NutritionSummary
	for _, m := range meals {
		sum.TotalCalories += m.TotalCalories
		sum.TotalProtein += m.TotalProtein
		sum.TotalCarbohydrates += m.TotalCarbohydrates
		sum.TotalFat += m.TotalFat
		loggedDays[m.CreatedAt.Format(dateLayout)] = struct{}{}
	}

	sum.TotalCalories = round2(sum.TotalCalories)
	sum.TotalProtein = round2(sum.TotalProtein)
	sum.TotalCarbohydrates = round2(sum.TotalCarbohydrates)
	sum.TotalFat = round2(sum.TotalFat)
	sum.MealsLogged = len(meals)
	sum.DaysLogged = len(loggedDays)
	if sum.DaysLogged > 0 {
		d := float64(sum.DaysLogged)
		sum.DailyAvg = MacroDaily{
			Calories:      round2(sum.TotalCalories / d),
			Protein:       round2(sum.TotalProtein / d),
			Carbohydrates: round2(sum.TotalCarbohydrates / d),
			Fat:           round2(sum.TotalFat / d),
		}
	}
	return sum
}

// summarizeWorkouts counts workouts and rolls up sets, reps, and volume
// (weight_kg * reps) across all of their exercises.
func summarizeWorkouts(workouts []training.Workout) TrainingSummary {
	trainedDays := map[string]struct{}{}
	var sum TrainingSummary
	for _, w := range workouts {
		trainedDays[w.PerformedAt.Format(dateLayout)] = struct{}{}
		for _, ex := range w.Exercises {
			sum.TotalSets++
			sum.TotalReps += int(ex.Reps)
			sum.TotalVolumeKg += ex.WeightKg * float64(ex.Reps)
		}
	}
	sum.Workouts = len(workouts)
	sum.TotalVolumeKg = round2(sum.TotalVolumeKg)
	sum.DaysTrained = len(trainedDays)
	return sum
}

// parseOrToday parses a YYYY-MM-DD date, defaulting to now when empty,
// matching the behavior of the meal and training date-range endpoints.
func parseOrToday(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	return time.Parse(dateLayout, s)
}

// truncateDay zeroes the time-of-day so date comparisons ignore the clock.
func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// round2 rounds a value to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
