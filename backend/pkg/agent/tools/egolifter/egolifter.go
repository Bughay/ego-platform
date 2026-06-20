package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Bughay/egolifter/internal/egolifter/analytics"
	"github.com/Bughay/egolifter/internal/egolifter/nutrition"
	"github.com/Bughay/egolifter/internal/egolifter/recipe"
	"github.com/Bughay/egolifter/internal/egolifter/training"
)

//go:embed egolifter.json
var SchemaJSON []byte

// Services bundles the egolifter domain services the agent tools act on. The
// caller builds these from the same repositories the HTTP handlers use and
// passes them to EgolifterFunctions. Only the services needed by the currently
// exposed tools are included; add more here as tools are added.
type Services struct {
	Meal      nutrition.MealService
	Food      nutrition.NutritionService
	Training  training.TrainingService
	Analytics analytics.AnalyticsService
	Recipe    recipe.RecipeService
}

// dateRange is the argument shape for date-range tools (YYYY-MM-DD); either
// bound may be empty, which the underlying services default to today.
type dateRange struct {
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
}

// recipeRef is the argument shape for tools that address a single recipe by id.
type recipeRef struct {
	ID string `json:"id"`
}

// EgolifterFunctions returns tool handlers that let the agent act in the
// egolifter app on behalf of a single user.
//
// userID is captured here (never taken from the model's arguments) so the agent
// can only ever touch the authenticated user's own data, mirroring how the HTTP
// handlers read identity from the JWT claims. ctx is captured for the database
// calls the services make. Each handler decodes its single string argument as
// JSON, calls the matching service method, and returns the result as JSON.
func EgolifterFunctions(ctx context.Context, svc Services, userID string) map[string]func(string) (string, error) {
	return map[string]func(string) (string, error){
		// --- nutrition: foods (catalog) ---------------------------------

		// list_foods returns the user's food catalog. Each food carries an id
		// and per-100g macros; the agent uses those ids to build recipes.
		"list_foods": func(string) (string, error) {
			foods, err := svc.Food.ListFoods(ctx, userID)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "list_foods")
			return jsonResult(foods)
		},

		// create_foods adds one or more foods to the catalog in a single
		// transaction and returns them (each including its new id, in order), so
		// the agent can immediately reference those ids in a recipe.
		"create_foods": func(args string) (string, error) {
			var in struct {
				Foods []nutrition.CreateFoodRequest `json:"foods"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("create_foods: invalid JSON args: %w", err)
			}
			reqs := make([]*nutrition.CreateFoodRequest, len(in.Foods))
			for i := range in.Foods {
				reqs[i] = &in.Foods[i]
			}
			foods, err := svc.Food.CreateFoods(ctx, userID, reqs)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "create_foods", "count", len(foods))
			return jsonResult(foods)
		},

		// --- nutrition: meals -------------------------------------------

		// create_meal logs a meal the user ate. The foods carry their own
		// weight and macro totals, so no separate catalog step is required.
		"create_meal": func(args string) (string, error) {
			var req nutrition.CreateMealRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("create_meal: invalid JSON args: %w", err)
			}
			meal, err := svc.Meal.CreateMeal(ctx, userID, &req)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "create_meal")
			return jsonResult(meal)
		},

		// list_meals returns the user's logged meals. With no date range it
		// returns all of them; with a range it filters to that inclusive span.
		"list_meals": func(args string) (string, error) {
			r, err := parseDateRange("list_meals", args)
			if err != nil {
				return "", err
			}
			var meals []nutrition.Meal
			if r.DateFrom == "" && r.DateTo == "" {
				meals, err = svc.Meal.ListMeals(ctx, userID)
			} else {
				meals, err = svc.Meal.ListMealsByDateRange(ctx, userID, r.DateFrom, r.DateTo)
			}
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "list_meals")
			return jsonResult(meals)
		},

		// --- training ---------------------------------------------------

		// log_workout logs a workout the user performed.
		"log_workout": func(args string) (string, error) {
			var req training.LogWorkoutRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("log_workout: invalid JSON args: %w", err)
			}
			workout, err := svc.Training.LogRoutine(ctx, userID, &req)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "log_workout")
			return jsonResult(workout)
		},

		// list_workouts returns the workouts the user actually performed. With
		// no date range it defaults to today; with a range it covers that span.
		"list_workouts": func(args string) (string, error) {
			r, err := parseDateRange("list_workouts", args)
			if err != nil {
				return "", err
			}
			workouts, err := svc.Training.ListWorkoutsByDateRange(ctx, userID, r.DateFrom, r.DateTo)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "list_workouts")
			return jsonResult(workouts)
		},

		// list_routines returns the user's saved routine templates. The agent
		// can pass a routine's id to log_workout to log that routine.
		"list_routines": func(string) (string, error) {
			routines, err := svc.Training.ListRoutines(ctx, userID)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "list_routines")
			return jsonResult(routines)
		},

		// create_routine saves a reusable workout routine (template) for the
		// user. The saved routine's id can later be passed to log_workout to
		// log it.
		"create_routine": func(args string) (string, error) {
			var req training.CreateRoutineRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("create_routine: invalid JSON args: %w", err)
			}
			routine, err := svc.Training.SaveRoutine(ctx, userID, &req)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "create_routine")
			return jsonResult(routine)
		},

		// --- recipes ----------------------------------------------------

		// create_recipe builds a recipe from foods already in the catalog. Each
		// ingredient references a food by food_id (from list_foods / create_foods).
		"create_recipe": func(args string) (string, error) {
			var req recipe.CreateRecipeRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				return "", fmt.Errorf("create_recipe: invalid JSON args: %w", err)
			}
			rec, err := svc.Recipe.CreateRecipe(ctx, userID, &req)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "create_recipe")
			return jsonResult(rec)
		},

		// list_recipes returns the user's recipes, each with its ingredients
		// (foods) and notes.
		"list_recipes": func(string) (string, error) {
			recipes, err := svc.Recipe.ListRecipes(ctx, userID)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "list_recipes")
			return jsonResult(recipes)
		},

		// get_recipe returns one recipe by id, with its ingredients and notes.
		"get_recipe": func(args string) (string, error) {
			var ref recipeRef
			if err := json.Unmarshal([]byte(args), &ref); err != nil {
				return "", fmt.Errorf("get_recipe: invalid JSON args: %w", err)
			}
			rec, err := svc.Recipe.GetRecipe(ctx, userID, ref.ID)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "get_recipe")
			return jsonResult(rec)
		},

		// --- analytics --------------------------------------------------

		// get_summary returns the combined nutrition + training summary over an
		// inclusive date range.
		"get_summary": func(args string) (string, error) {
			r, err := parseDateRange("get_summary", args)
			if err != nil {
				return "", err
			}
			summary, err := svc.Analytics.GetSummary(ctx, userID, r.DateFrom, r.DateTo)
			if err != nil {
				return "", err
			}
			slog.Info("tool called", "tool", "get_summary")
			return jsonResult(summary)
		},
	}
}

// parseDateRange decodes an optional {date_from, date_to} argument. An empty
// argument is valid and yields a zero range (the services default it to today or
// to "all", depending on the tool).
func parseDateRange(tool, args string) (dateRange, error) {
	var r dateRange
	if strings.TrimSpace(args) == "" {
		return r, nil
	}
	if err := json.Unmarshal([]byte(args), &r); err != nil {
		return r, fmt.Errorf("%s: invalid JSON args: %w", tool, err)
	}
	return r, nil
}

// jsonResult marshals a tool's return value to a JSON string for the agent's
// observation.
func jsonResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b), nil
}
