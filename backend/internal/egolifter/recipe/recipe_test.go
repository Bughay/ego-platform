package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// stubRecipeRepository records calls and returns configurable fixtures.
type stubRecipeRepository struct {
	err    error   // when set, every method fails with it
	recipe *Recipe // returned by FindByID/Update (nil = not found)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	deletedID    string

	deleteMissing bool         // when set, Delete reports the recipe as not found
	foods         []RecipeFood // returned by GetFoods
}

func (s *stubRecipeRepository) Create(ctx context.Context, userID string, req *CreateRecipeRequest) (*Recipe, error) {
	s.createCalled = true
	if s.err != nil {
		return nil, s.err
	}
	return &Recipe{ID: "recipe-1", UserID: userID, Name: req.Name, Ingredients: []Ingredient{}}, nil
}

func (s *stubRecipeRepository) FindByID(ctx context.Context, userID, id string) (*Recipe, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.recipe, nil
}

func (s *stubRecipeRepository) List(ctx context.Context, userID string) ([]Recipe, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Recipe{{ID: "recipe-1", UserID: userID, Name: "protein oats", Ingredients: []Ingredient{}}}, nil
}

func (s *stubRecipeRepository) Update(ctx context.Context, userID string, req *UpdateRecipeRequest) (*Recipe, error) {
	s.updateCalled = true
	if s.err != nil {
		return nil, s.err
	}
	return s.recipe, nil
}

func (s *stubRecipeRepository) GetFoods(ctx context.Context, userID, recipeID string) ([]RecipeFood, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.foods, nil
}

func (s *stubRecipeRepository) Delete(ctx context.Context, userID, id string) (bool, error) {
	s.deleteCalled = true
	s.deletedID = id
	if s.err != nil {
		return false, s.err
	}
	return !s.deleteMissing, nil
}

// --- Service tests ---

func TestCreateRecipeValidation(t *testing.T) {
	validIngredients := []IngredientInput{
		{FoodID: "food-1", WeightG: 100},
		{FoodID: "food-2", WeightG: 40},
	}
	longName := strings.Repeat("a", 101)

	tests := []struct {
		name    string
		req     *CreateRecipeRequest
		wantErr string // substring of the expected error; empty means success
	}{
		{
			name: "valid request",
			req:  &CreateRecipeRequest{Name: "protein oats", Ingredients: validIngredients},
		},
		{
			name: "boundary weight is valid",
			req: &CreateRecipeRequest{Name: "bulk rice", Ingredients: []IngredientInput{
				{FoodID: "food-1", WeightG: 5000},
			}},
		},
		{
			name:    "empty name",
			req:     &CreateRecipeRequest{Name: "", Ingredients: validIngredients},
			wantErr: "recipe name is required",
		},
		{
			name:    "whitespace name",
			req:     &CreateRecipeRequest{Name: "   ", Ingredients: validIngredients},
			wantErr: "recipe name is required",
		},
		{
			name:    "name too long",
			req:     &CreateRecipeRequest{Name: longName, Ingredients: validIngredients},
			wantErr: "at most 100 characters",
		},
		{
			name:    "no ingredients",
			req:     &CreateRecipeRequest{Name: "protein oats", Ingredients: []IngredientInput{}},
			wantErr: "at least one ingredient",
		},
		{
			name: "blank food_id",
			req: &CreateRecipeRequest{Name: "protein oats", Ingredients: []IngredientInput{
				{FoodID: "  ", WeightG: 100},
			}},
			wantErr: "ingredient 0: food_id is required",
		},
		{
			name: "zero weight",
			req: &CreateRecipeRequest{Name: "protein oats", Ingredients: []IngredientInput{
				{FoodID: "food-1", WeightG: 0},
			}},
			wantErr: "weight_g must be greater than zero",
		},
		{
			name: "negative weight",
			req: &CreateRecipeRequest{Name: "protein oats", Ingredients: []IngredientInput{
				{FoodID: "food-1", WeightG: -10},
			}},
			wantErr: "weight_g must be greater than zero",
		},
		{
			name: "absurd weight",
			req: &CreateRecipeRequest{Name: "protein oats", Ingredients: []IngredientInput{
				{FoodID: "food-1", WeightG: 5001},
			}},
			wantErr: "weight_g must be at most 5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRecipeRepository{}
			svc := NewRecipeService(repo)

			recipe, err := svc.CreateRecipe(context.Background(), "user-1", tt.req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if !repo.createCalled {
					t.Fatal("expected repository Create to be called")
				}
				if recipe == nil || recipe.Name != tt.req.Name {
					t.Fatalf("unexpected recipe returned: %+v", recipe)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "validation:") {
				t.Errorf("expected validation error, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
			if repo.createCalled {
				t.Error("repository Create should not be called on validation failure")
			}
		})
	}
}

func TestUpdateRecipeValidation(t *testing.T) {
	validIngredients := []IngredientInput{{FoodID: "food-1", WeightG: 100}}

	t.Run("blank id", func(t *testing.T) {
		repo := &stubRecipeRepository{}
		svc := NewRecipeService(repo)

		_, err := svc.UpdateRecipe(context.Background(), "user-1", &UpdateRecipeRequest{
			ID: "  ", Name: "protein oats", Ingredients: validIngredients,
		})
		if err == nil || !strings.Contains(err.Error(), "recipe id is required") {
			t.Fatalf("expected recipe id validation error, got: %v", err)
		}
		if repo.updateCalled {
			t.Error("repository Update should not be called on validation failure")
		}
	})

	t.Run("invalid payload", func(t *testing.T) {
		repo := &stubRecipeRepository{}
		svc := NewRecipeService(repo)

		_, err := svc.UpdateRecipe(context.Background(), "user-1", &UpdateRecipeRequest{
			ID: "recipe-1", Name: "", Ingredients: validIngredients,
		})
		if err == nil || !strings.Contains(err.Error(), "recipe name is required") {
			t.Fatalf("expected name validation error, got: %v", err)
		}
	})

	t.Run("valid request", func(t *testing.T) {
		want := &Recipe{ID: "recipe-1", Name: "protein oats v2", Ingredients: []Ingredient{}}
		repo := &stubRecipeRepository{recipe: want}
		svc := NewRecipeService(repo)

		recipe, err := svc.UpdateRecipe(context.Background(), "user-1", &UpdateRecipeRequest{
			ID: "recipe-1", Name: "protein oats v2", Ingredients: validIngredients,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !repo.updateCalled {
			t.Fatal("expected repository Update to be called")
		}
		if recipe != want {
			t.Fatalf("unexpected recipe returned: %+v", recipe)
		}
	})
}

func TestDeleteRecipeValidation(t *testing.T) {
	t.Run("blank id", func(t *testing.T) {
		repo := &stubRecipeRepository{}
		svc := NewRecipeService(repo)

		_, err := svc.DeleteRecipe(context.Background(), "user-1", "")
		if err == nil || !strings.Contains(err.Error(), "recipe id is required") {
			t.Fatalf("expected recipe id validation error, got: %v", err)
		}
		if repo.deleteCalled {
			t.Error("repository Delete should not be called on validation failure")
		}
	})

	t.Run("valid id", func(t *testing.T) {
		repo := &stubRecipeRepository{}
		svc := NewRecipeService(repo)

		found, err := svc.DeleteRecipe(context.Background(), "user-1", "recipe-1")
		if err != nil || !found {
			t.Fatalf("expected success, got found=%v error: %v", found, err)
		}
		if !repo.deleteCalled || repo.deletedID != "recipe-1" {
			t.Fatalf("expected repository Delete called with recipe-1, called=%v id=%q", repo.deleteCalled, repo.deletedID)
		}
	})
}

func TestGetAndListRecipesPassThrough(t *testing.T) {
	want := &Recipe{ID: "recipe-1", Name: "protein oats", Ingredients: []Ingredient{}}
	repo := &stubRecipeRepository{recipe: want}
	svc := NewRecipeService(repo)

	recipe, err := svc.GetRecipe(context.Background(), "user-1", "recipe-1")
	if err != nil || recipe != want {
		t.Fatalf("GetRecipe: expected %+v, got %+v (err=%v)", want, recipe, err)
	}

	recipes, err := svc.ListRecipes(context.Background(), "user-1")
	if err != nil || len(recipes) != 1 {
		t.Fatalf("ListRecipes: expected 1 recipe, got %d (err=%v)", len(recipes), err)
	}
}

// --- Handler tests ---

// newRecipeServer mounts the recipe routes behind real JWT middleware and
// returns the mux together with a valid bearer token for user-1.
func newRecipeServer(t *testing.T, repo RecipeRepository) (*http.ServeMux, string) {
	t.Helper()
	mgr := auth.NewManager("test-secret", 1, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRecipeHandler(NewRecipeService(repo), logger)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, mgr.Middleware)

	token, err := mgr.Generate("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return mux, token
}

func doJSON(t *testing.T, mux *http.ServeMux, method, target, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestRecipeEndpointsRequireAuth(t *testing.T) {
	mux, _ := newRecipeServer(t, &stubRecipeRepository{})

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/recipe/create"},
		{http.MethodGet, "/recipe/view"},
		{http.MethodPut, "/recipe/update"},
		{http.MethodDelete, "/recipe/del"},
		{http.MethodGet, "/recipe/getfoods"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.target, func(t *testing.T) {
			if rec := doJSON(t, mux, rt.method, rt.target, "", nil); rec.Code != http.StatusUnauthorized {
				t.Errorf("no token: expected 401, got %d", rec.Code)
			}
			if rec := doJSON(t, mux, rt.method, rt.target, "garbage-token", nil); rec.Code != http.StatusUnauthorized {
				t.Errorf("garbage token: expected 401, got %d", rec.Code)
			}
		})
	}
}

func TestCreateRecipeEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/recipe/create", token, CreateRecipeRequest{
			Name:        "protein oats",
			Ingredients: []IngredientInput{{FoodID: "food-1", WeightG: 100}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var recipe Recipe
		if err := json.NewDecoder(rec.Body).Decode(&recipe); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if recipe.Name != "protein oats" {
			t.Errorf("unexpected recipe in response: %+v", recipe)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/recipe/create", token, CreateRecipeRequest{Name: ""})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		var apiErr lib.APIError
		if err := json.NewDecoder(rec.Body).Decode(&apiErr); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if !strings.HasPrefix(apiErr.Message, "validation:") {
			t.Errorf("expected validation message, got: %q", apiErr.Message)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		req := httptest.NewRequest(http.MethodPost, "/recipe/create", strings.NewReader("{not json"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodPost, "/recipe/create", token, CreateRecipeRequest{
			Name:        "protein oats",
			Ingredients: []IngredientInput{{FoodID: "food-1", WeightG: 100}},
		})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestViewRecipeEndpoint(t *testing.T) {
	t.Run("list all", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/recipe/view", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var recipes []Recipe
		if err := json.NewDecoder(rec.Body).Decode(&recipes); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(recipes) != 1 {
			t.Errorf("expected 1 recipe, got %d", len(recipes))
		}
	})

	t.Run("by id found", func(t *testing.T) {
		want := &Recipe{ID: "recipe-1", Name: "protein oats", Ingredients: []Ingredient{}}
		mux, token := newRecipeServer(t, &stubRecipeRepository{recipe: want})
		rec := doJSON(t, mux, http.MethodGet, "/recipe/view?id=recipe-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("by id not found", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{recipe: nil})
		rec := doJSON(t, mux, http.MethodGet, "/recipe/view?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateRecipeEndpoint(t *testing.T) {
	validReq := UpdateRecipeRequest{
		ID:          "recipe-1",
		Name:        "protein oats v2",
		Ingredients: []IngredientInput{{FoodID: "food-1", WeightG: 100}},
	}

	t.Run("found", func(t *testing.T) {
		want := &Recipe{ID: "recipe-1", Name: "protein oats v2", Ingredients: []Ingredient{}}
		mux, token := newRecipeServer(t, &stubRecipeRepository{recipe: want})
		rec := doJSON(t, mux, http.MethodPut, "/recipe/update", token, validReq)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{recipe: nil})
		rec := doJSON(t, mux, http.MethodPut, "/recipe/update", token, validReq)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestScaleByWeight(t *testing.T) {
	cases := []struct {
		per100, weightG, want float64
	}{
		{389, 80, 311.2},
		{60, 250, 150},
		{100, 100, 100},
		{50, 0, 0},
	}
	for _, c := range cases {
		if got := scaleByWeight(c.per100, c.weightG); got != c.want {
			t.Errorf("scaleByWeight(%v, %v) = %v, want %v", c.per100, c.weightG, got, c.want)
		}
	}
}

func TestGetRecipeFoodsService(t *testing.T) {
	t.Run("blank id", func(t *testing.T) {
		svc := NewRecipeService(&stubRecipeRepository{})
		_, err := svc.GetRecipeFoods(context.Background(), "user-1", "")
		if err == nil || !strings.Contains(err.Error(), "recipe id is required") {
			t.Fatalf("expected recipe id validation error, got: %v", err)
		}
	})

	t.Run("returns foods", func(t *testing.T) {
		repo := &stubRecipeRepository{foods: []RecipeFood{{FoodID: "food-1", FoodName: "oats", WeightG: 80, TotalCalories: 311.2}}}
		svc := NewRecipeService(repo)
		foods, err := svc.GetRecipeFoods(context.Background(), "user-1", "recipe-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(foods) != 1 || foods[0].TotalCalories != 311.2 {
			t.Fatalf("unexpected foods: %+v", foods)
		}
	})

	t.Run("recipe not found", func(t *testing.T) {
		// No foods and FindByID returns nil → not found.
		svc := NewRecipeService(&stubRecipeRepository{recipe: nil})
		foods, err := svc.GetRecipeFoods(context.Background(), "user-1", "missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if foods != nil {
			t.Fatalf("expected nil foods for missing recipe, got: %+v", foods)
		}
	})

	t.Run("recipe with no ingredients", func(t *testing.T) {
		repo := &stubRecipeRepository{recipe: &Recipe{ID: "recipe-1", Ingredients: []Ingredient{}}}
		svc := NewRecipeService(repo)
		foods, err := svc.GetRecipeFoods(context.Background(), "user-1", "recipe-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if foods == nil || len(foods) != 0 {
			t.Fatalf("expected empty (non-nil) foods, got: %+v", foods)
		}
	})
}

func TestGetRecipeFoodsEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		repo := &stubRecipeRepository{foods: []RecipeFood{{FoodID: "food-1", FoodName: "oats", WeightG: 80, TotalCalories: 311.2}}}
		mux, token := newRecipeServer(t, repo)
		rec := doJSON(t, mux, http.MethodGet, "/recipe/getfoods?id=recipe-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var foods []RecipeFood
		if err := json.NewDecoder(rec.Body).Decode(&foods); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(foods) != 1 || foods[0].FoodName != "oats" || foods[0].TotalCalories != 311.2 {
			t.Errorf("unexpected foods in response: %+v", foods)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{recipe: nil})
		rec := doJSON(t, mux, http.MethodGet, "/recipe/getfoods?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/recipe/getfoods", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteRecipeEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/recipe/del?id=recipe-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["message"] == "" {
			t.Errorf("expected a confirmation message, got: %v", body)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{deleteMissing: true})
		rec := doJSON(t, mux, http.MethodDelete, "/recipe/del?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newRecipeServer(t, &stubRecipeRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/recipe/del", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
