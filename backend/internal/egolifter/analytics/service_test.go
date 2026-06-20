package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Bughay/egolifter/internal/egolifter/nutrition"
	"github.com/Bughay/egolifter/internal/egolifter/training"
)

// fakeMealSvc is a stub nutrition.MealService that returns canned meals and
// records the date strings it was called with.
type fakeMealSvc struct {
	meals          []nutrition.Meal
	err            error
	gotFrom, gotTo string
}

func (f *fakeMealSvc) CreateMeal(ctx context.Context, userID string, req *nutrition.CreateMealRequest) (*nutrition.Meal, error) {
	return nil, nil
}
func (f *fakeMealSvc) GetMeal(ctx context.Context, userID, id string) (*nutrition.Meal, error) {
	return nil, nil
}
func (f *fakeMealSvc) ListMeals(ctx context.Context, userID string) ([]nutrition.Meal, error) {
	return nil, nil
}
func (f *fakeMealSvc) ListMealsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]nutrition.Meal, error) {
	f.gotFrom, f.gotTo = fromStr, toStr
	return f.meals, f.err
}
func (f *fakeMealSvc) DeleteMeal(ctx context.Context, userID, id string) (bool, error) {
	return false, nil
}

// fakeTrainingSvc is a stub training.TrainingService that returns canned workouts.
type fakeTrainingSvc struct {
	workouts []training.Workout
	err      error
}

func (f *fakeTrainingSvc) SaveRoutine(ctx context.Context, userID string, req *training.CreateRoutineRequest) (*training.Routine, error) {
	return nil, nil
}
func (f *fakeTrainingSvc) ListRoutines(ctx context.Context, userID string) ([]training.Routine, error) {
	return nil, nil
}
func (f *fakeTrainingSvc) LogRoutine(ctx context.Context, userID string, req *training.LogWorkoutRequest) (*training.Workout, error) {
	return nil, nil
}
func (f *fakeTrainingSvc) ListWorkoutsByDate(ctx context.Context, userID, dateStr string) ([]training.Workout, error) {
	return nil, nil
}
func (f *fakeTrainingSvc) ListWorkoutsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]training.Workout, error) {
	return f.workouts, f.err
}
func (f *fakeTrainingSvc) DeleteWorkout(ctx context.Context, userID, id string) (bool, error) {
	return false, nil
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func TestGetSummary_Aggregates(t *testing.T) {
	meals := []nutrition.Meal{
		{TotalCalories: 500, TotalProtein: 30, TotalCarbohydrates: 50, TotalFat: 20, CreatedAt: day(2026, 6, 1)},
		{TotalCalories: 700, TotalProtein: 40, TotalCarbohydrates: 60, TotalFat: 25, CreatedAt: day(2026, 6, 1)},
		{TotalCalories: 300, TotalProtein: 20, TotalCarbohydrates: 30, TotalFat: 10, CreatedAt: day(2026, 6, 3)},
	}
	workouts := []training.Workout{
		{PerformedAt: day(2026, 6, 1), Exercises: []training.Exercise{
			{WeightKg: 100, Reps: 5},
			{WeightKg: 80, Reps: 8},
		}},
		{PerformedAt: day(2026, 6, 2), Exercises: []training.Exercise{
			{WeightKg: 60, Reps: 10},
		}},
	}

	meal := &fakeMealSvc{meals: meals}
	svc := NewAnalyticsService(meal, &fakeTrainingSvc{workouts: workouts})

	got, err := svc.GetSummary(context.Background(), "user-1", "2026-06-01", "2026-06-03")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}

	// Raw strings must be forwarded untouched to the underlying service.
	if meal.gotFrom != "2026-06-01" || meal.gotTo != "2026-06-03" {
		t.Errorf("date strings not forwarded: from=%q to=%q", meal.gotFrom, meal.gotTo)
	}

	if got.Days != 3 {
		t.Errorf("Days = %d, want 3", got.Days)
	}

	n := got.Nutrition
	if n.TotalCalories != 1500 || n.TotalProtein != 90 || n.TotalCarbohydrates != 140 || n.TotalFat != 55 {
		t.Errorf("nutrition totals wrong: %+v", n)
	}
	if n.MealsLogged != 3 {
		t.Errorf("MealsLogged = %d, want 3", n.MealsLogged)
	}
	if n.DaysLogged != 2 {
		t.Errorf("DaysLogged = %d, want 2", n.DaysLogged)
	}
	// Daily averages divide totals by the 2 days that had logged meals.
	if n.DailyAvg.Calories != 750 || n.DailyAvg.Protein != 45 || n.DailyAvg.Carbohydrates != 70 || n.DailyAvg.Fat != 27.5 {
		t.Errorf("daily_avg wrong: %+v", n.DailyAvg)
	}

	tr := got.Training
	if tr.Workouts != 2 {
		t.Errorf("Workouts = %d, want 2", tr.Workouts)
	}
	if tr.TotalSets != 3 {
		t.Errorf("TotalSets = %d, want 3", tr.TotalSets)
	}
	if tr.TotalReps != 23 {
		t.Errorf("TotalReps = %d, want 23", tr.TotalReps)
	}
	if tr.TotalVolumeKg != 1740 { // 100*5 + 80*8 + 60*10
		t.Errorf("TotalVolumeKg = %v, want 1740", tr.TotalVolumeKg)
	}
	if tr.DaysTrained != 2 {
		t.Errorf("DaysTrained = %d, want 2", tr.DaysTrained)
	}
}

func TestGetSummary_EmptyRangeDefaultsToToday(t *testing.T) {
	svc := NewAnalyticsService(&fakeMealSvc{}, &fakeTrainingSvc{})

	got, err := svc.GetSummary(context.Background(), "user-1", "", "")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if got.Days != 1 {
		t.Errorf("Days = %d, want 1 for an empty (today-only) range", got.Days)
	}
	// With no meals, averages stay at zero (no divide-by-zero).
	if got.Nutrition.DaysLogged != 0 || got.Nutrition.DailyAvg != (MacroDaily{}) {
		t.Errorf("expected zero-value nutrition averages, got DaysLogged=%d avg=%+v", got.Nutrition.DaysLogged, got.Nutrition.DailyAvg)
	}
	today := time.Now().UTC().Format(dateLayout)
	if got.DateFrom != today || got.DateTo != today {
		t.Errorf("range = %s..%s, want %s..%s", got.DateFrom, got.DateTo, today, today)
	}
}

func TestGetSummary_FromAfterTo(t *testing.T) {
	svc := NewAnalyticsService(&fakeMealSvc{}, &fakeTrainingSvc{})

	_, err := svc.GetSummary(context.Background(), "user-1", "2026-06-10", "2026-06-01")
	if err == nil {
		t.Fatal("expected a validation error when date_from is after date_to")
	}
	if !isValidationErr(err) {
		t.Errorf("error %q is not a validation error", err)
	}
}

func TestGetSummary_BadDate(t *testing.T) {
	svc := NewAnalyticsService(&fakeMealSvc{}, &fakeTrainingSvc{})

	_, err := svc.GetSummary(context.Background(), "user-1", "06-01-2026", "")
	var parseErr *time.ParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("expected *time.ParseError for a malformed date, got %v", err)
	}
}
