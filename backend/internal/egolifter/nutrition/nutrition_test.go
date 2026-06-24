package nutrition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/cache"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// --- Stubs ---

// stubMealRepository records whether Create was called and returns a fixed meal.
type stubMealRepository struct {
	err          error // when set, every method fails with it
	meal         *Meal // returned by FindByID (nil = not found)
	createCalled bool

	rangeFrom time.Time // recorded by ListByDateRange
	rangeTo   time.Time

	deleteCalled  bool
	deletedID     string // recorded by Delete
	deleteMissing bool   // when set, Delete reports the meal as not found
}

func (s *stubMealRepository) Create(ctx context.Context, userID string, req *CreateMealRequest) (*Meal, error) {
	s.createCalled = true
	if s.err != nil {
		return nil, s.err
	}
	return &Meal{ID: "meal-1", UserID: userID, Name: req.Name, Foods: []ConsumedFood{}}, nil
}

func (s *stubMealRepository) FindByID(ctx context.Context, userID, id string) (*Meal, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.meal, nil
}

func (s *stubMealRepository) List(ctx context.Context, userID string) ([]Meal, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Meal{{ID: "meal-1", UserID: userID, Name: "breakfast", Foods: []ConsumedFood{}}}, nil
}

func (s *stubMealRepository) Delete(ctx context.Context, userID, id string) (bool, error) {
	s.deleteCalled = true
	s.deletedID = id
	if s.err != nil {
		return false, s.err
	}
	return !s.deleteMissing, nil
}

func (s *stubMealRepository) ListByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Meal, error) {
	s.rangeFrom, s.rangeTo = from, to
	if s.err != nil {
		return nil, s.err
	}
	return []Meal{{ID: "meal-1", UserID: userID, Name: "breakfast", Foods: []ConsumedFood{}}}, nil
}

// stubFoodRepository records calls and returns configurable fixtures.
type stubFoodRepository struct {
	err  error // when set, every method fails with it
	food *Food // returned by FindByID/Update (nil = not found)

	createCalled    bool
	createManyCount int // number of foods passed to the last CreateMany call
	updateCalled    bool
	deleteCalled    bool
}

func (s *stubFoodRepository) Create(ctx context.Context, userID string, req *CreateFoodRequest) (*Food, error) {
	s.createCalled = true
	if s.err != nil {
		return nil, s.err
	}
	return &Food{
		ID:               "food-1",
		UserID:           userID,
		Name:             req.Name,
		Calories100:      req.Calories100,
		Protein100:       req.Protein100,
		Carbohydrates100: req.Carbohydrates100,
		Fat100:           req.Fat100,
	}, nil
}

func (s *stubFoodRepository) CreateMany(ctx context.Context, userID string, reqs []*CreateFoodRequest) ([]Food, error) {
	s.createManyCount = len(reqs)
	if s.err != nil {
		return nil, s.err
	}
	foods := make([]Food, len(reqs))
	for i, req := range reqs {
		foods[i] = Food{
			ID:               fmt.Sprintf("food-%d", i+1),
			UserID:           userID,
			Name:             req.Name,
			Calories100:      req.Calories100,
			Protein100:       req.Protein100,
			Carbohydrates100: req.Carbohydrates100,
			Fat100:           req.Fat100,
		}
	}
	return foods, nil
}

func (s *stubFoodRepository) FindByID(ctx context.Context, userID, id string) (*Food, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.food, nil
}

func (s *stubFoodRepository) List(ctx context.Context, userID string) ([]Food, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Food{{ID: "food-1", Name: "oats", Calories100: 389, Protein100: 16.9}}, nil
}

func (s *stubFoodRepository) Update(ctx context.Context, userID string, req *UpdateFoodRequest) (*Food, error) {
	s.updateCalled = true
	if s.err != nil {
		return nil, s.err
	}
	return s.food, nil
}

func (s *stubFoodRepository) Delete(ctx context.Context, userID, id string) error {
	s.deleteCalled = true
	return s.err
}

// --- Meal service tests ---

func TestCreateMealValidation(t *testing.T) {
	validFoods := []MealFoodInput{
		{Name: "oats", WeightG: 150, Calories: 200, Protein: 10, Carbohydrates: 30, Fat: 5},
		{Name: "milk", WeightG: 60, Calories: 30, Protein: 2, Carbohydrates: 3, Fat: 1},
	}

	tests := []struct {
		name    string
		req     *CreateMealRequest
		wantErr string // substring of the expected error; empty means success
	}{
		{
			name: "valid request",
			req:  &CreateMealRequest{Name: "breakfast", Foods: validFoods},
		},
		{
			name:    "empty name",
			req:     &CreateMealRequest{Name: "", Foods: validFoods},
			wantErr: "meal name is required",
		},
		{
			name:    "whitespace name",
			req:     &CreateMealRequest{Name: "   ", Foods: validFoods},
			wantErr: "meal name is required",
		},
		{
			name:    "name too long",
			req:     &CreateMealRequest{Name: strings.Repeat("a", 101), Foods: validFoods},
			wantErr: "at most 100 characters",
		},
		{
			name:    "no foods",
			req:     &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{}},
			wantErr: "at least one food",
		},
		{
			name: "blank food name",
			req: &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{
				{Name: "  ", WeightG: 100},
			}},
			wantErr: "name is required",
		},
		{
			name: "zero weight",
			req: &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{
				{Name: "oats", WeightG: 0},
			}},
			wantErr: "weight_g must be greater than zero",
		},
		{
			name: "negative weight",
			req: &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{
				{Name: "oats", WeightG: -50},
			}},
			wantErr: "weight_g must be greater than zero",
		},
		{
			name: "absurd weight",
			req: &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{
				{Name: "oats", WeightG: 5001},
			}},
			wantErr: "weight_g must be at most 5000",
		},
		{
			name: "negative macro",
			req: &CreateMealRequest{Name: "breakfast", Foods: []MealFoodInput{
				{Name: "oats", WeightG: 100, Protein: -1},
			}},
			wantErr: "macros must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubMealRepository{}
			svc := NewMealService(repo)

			meal, err := svc.CreateMeal(context.Background(), "user-1", tt.req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if !repo.createCalled {
					t.Fatal("expected repository Create to be called")
				}
				if meal == nil || meal.Name != tt.req.Name {
					t.Fatalf("unexpected meal returned: %+v", meal)
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

// --- Meal food resolution (catalog match / auto-save) ---

func TestResolveMealFood(t *testing.T) {
	apple := Food{ID: "f-apple", Name: "apple", Calories100: 52, Protein100: 0.3, Carbohydrates100: 14, Fat100: 0.2}
	apple2 := Food{ID: "f-apple2", Name: "apple_2", Calories100: 60, Protein100: 0.3, Carbohydrates100: 14, Fat100: 0.2}

	tests := []struct {
		name        string
		saved       []Food
		in          MealFoodInput
		wantMatchID string  // non-empty means an existing food should be reused
		wantCreate  string  // expected toCreate.Name; empty means no create expected
		wantErr     string  // substring; empty means no error
		wantCal100  float64 // checked only when checkCal is true
		checkCal    bool
	}{
		{
			name:        "exact match reuses existing food",
			saved:       []Food{apple},
			in:          MealFoodInput{Name: "apple", WeightG: 100, Calories: 52, Protein: 0.3, Carbohydrates: 14, Fat: 0.2},
			wantMatchID: "f-apple",
		},
		{
			name:       "unknown food is created",
			saved:      []Food{apple},
			in:         MealFoodInput{Name: "banana", WeightG: 100, Calories: 89, Protein: 1.1, Carbohydrates: 23, Fat: 0.3},
			wantCreate: "banana",
		},
		{
			name:       "same name different macros creates _2 variant",
			saved:      []Food{apple},
			in:         MealFoodInput{Name: "apple", WeightG: 100, Calories: 60, Protein: 0.3, Carbohydrates: 14, Fat: 0.2},
			wantCreate: "apple_2",
		},
		{
			name:        "variant exact match reuses _2",
			saved:       []Food{apple, apple2},
			in:          MealFoodInput{Name: "apple", WeightG: 100, Calories: 60, Protein: 0.3, Carbohydrates: 14, Fat: 0.2},
			wantMatchID: "f-apple2",
		},
		{
			name:    "third differing variant errors",
			saved:   []Food{apple, apple2},
			in:      MealFoodInput{Name: "apple", WeightG: 100, Calories: 70, Protein: 0.3, Carbohydrates: 14, Fat: 0.2},
			wantErr: "two foods named",
		},
		{
			name:       "totals are converted to per-100g",
			saved:      nil,
			in:         MealFoodInput{Name: "rice", WeightG: 200, Calories: 260, Protein: 5, Carbohydrates: 56, Fat: 0.6},
			wantCreate: "rice",
			wantCal100: 130,
			checkCal:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchID, toCreate, err := resolveMealFood(tt.saved, tt.in)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantMatchID != "" {
				if matchID != tt.wantMatchID {
					t.Errorf("matchID = %q, want %q", matchID, tt.wantMatchID)
				}
				if toCreate != nil {
					t.Errorf("expected no food to create, got %+v", toCreate)
				}
				return
			}

			if toCreate == nil {
				t.Fatalf("expected a food to create (%q), got matchID=%q", tt.wantCreate, matchID)
			}
			if toCreate.Name != tt.wantCreate {
				t.Errorf("toCreate.Name = %q, want %q", toCreate.Name, tt.wantCreate)
			}
			if tt.checkCal && toCreate.Calories100 != tt.wantCal100 {
				t.Errorf("toCreate.Calories100 = %v, want %v", toCreate.Calories100, tt.wantCal100)
			}
		})
	}
}

// --- Food service tests ---

func TestCreateFoodValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *CreateFoodRequest
		wantErr string // substring of the expected error; empty means success
	}{
		{
			name: "valid request",
			req:  &CreateFoodRequest{Name: "oats", Calories100: 389, Protein100: 16.9, Carbohydrates100: 66.3, Fat100: 6.9},
		},
		{
			name: "boundary values are valid",
			req:  &CreateFoodRequest{Name: "pure fat", Calories100: 900, Protein100: 100, Carbohydrates100: 100, Fat100: 100},
		},
		{
			name:    "empty name",
			req:     &CreateFoodRequest{Name: "", Calories100: 100},
			wantErr: "food name is required",
		},
		{
			name:    "whitespace name",
			req:     &CreateFoodRequest{Name: "   ", Calories100: 100},
			wantErr: "food name is required",
		},
		{
			name:    "name too long",
			req:     &CreateFoodRequest{Name: strings.Repeat("a", 101), Calories100: 100},
			wantErr: "at most 100 characters",
		},
		{
			name:    "negative calories",
			req:     &CreateFoodRequest{Name: "oats", Calories100: -1},
			wantErr: "calories_100 must be between 0 and 900",
		},
		{
			name:    "absurd calories",
			req:     &CreateFoodRequest{Name: "oats", Calories100: 901},
			wantErr: "calories_100 must be between 0 and 900",
		},
		{
			name:    "negative protein",
			req:     &CreateFoodRequest{Name: "oats", Calories100: 100, Protein100: -1},
			wantErr: "protein_100 must be between 0 and 100",
		},
		{
			name:    "absurd protein",
			req:     &CreateFoodRequest{Name: "oats", Calories100: 100, Protein100: 101},
			wantErr: "protein_100 must be between 0 and 100",
		},
		{
			name:    "absurd carbohydrates",
			req:     &CreateFoodRequest{Name: "oats", Calories100: 100, Carbohydrates100: 101},
			wantErr: "carbohydrates_100 must be between 0 and 100",
		},
		{
			name:    "absurd fat",
			req:     &CreateFoodRequest{Name: "oats", Calories100: 100, Fat100: 101},
			wantErr: "fat_100 must be between 0 and 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubFoodRepository{}
			svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

			food, err := svc.CreateFood(context.Background(), "user-1", tt.req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if !repo.createCalled {
					t.Fatal("expected repository Create to be called")
				}
				if food == nil || food.Name != tt.req.Name {
					t.Fatalf("unexpected food returned: %+v", food)
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

func TestCreateFoods(t *testing.T) {
	valid := func(name string) *CreateFoodRequest {
		return &CreateFoodRequest{Name: name, Calories100: 100, Protein100: 10, Carbohydrates100: 10, Fat100: 5}
	}

	t.Run("creates all foods and returns them in order", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		foods, err := svc.CreateFoods(context.Background(), "user-1", []*CreateFoodRequest{
			valid("oats"), valid("rice"), valid("chicken"),
		})
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if repo.createManyCount != 3 {
			t.Errorf("CreateMany got %d foods, want 3", repo.createManyCount)
		}
		if len(foods) != 3 || foods[0].Name != "oats" || foods[2].Name != "chicken" {
			t.Fatalf("unexpected foods (order not preserved): %+v", foods)
		}
	})

	t.Run("empty slice is rejected before any insert", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		_, err := svc.CreateFoods(context.Background(), "user-1", nil)
		if err == nil || !strings.Contains(err.Error(), "at least one food") {
			t.Fatalf("expected empty-slice validation error, got: %v", err)
		}
		if repo.createManyCount != 0 {
			t.Error("CreateMany should not be called on an empty slice")
		}
	})

	t.Run("one invalid food fails the whole batch before any insert", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		_, err := svc.CreateFoods(context.Background(), "user-1", []*CreateFoodRequest{
			valid("oats"),
			{Name: "", Calories100: 100}, // invalid: missing name
		})
		if err == nil || !strings.Contains(err.Error(), "food name is required") {
			t.Fatalf("expected validation error, got: %v", err)
		}
		if repo.createManyCount != 0 {
			t.Error("CreateMany should not be called when a food fails validation")
		}
	})
}

func TestFoodIDValidation(t *testing.T) {
	t.Run("update requires id", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		_, err := svc.UpdateFood(context.Background(), "user-1", &UpdateFoodRequest{ID: "  ", Name: "oats", Calories100: 100})
		if err == nil || !strings.Contains(err.Error(), "food id is required") {
			t.Fatalf("expected food id validation error, got: %v", err)
		}
		if repo.updateCalled {
			t.Error("repository Update should not be called on validation failure")
		}
	})

	t.Run("update validates payload", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		_, err := svc.UpdateFood(context.Background(), "user-1", &UpdateFoodRequest{ID: "food-1", Name: "", Calories100: 100})
		if err == nil || !strings.Contains(err.Error(), "food name is required") {
			t.Fatalf("expected name validation error, got: %v", err)
		}
	})

	t.Run("delete requires id", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		err := svc.DeleteFood(context.Background(), "user-1", "")
		if err == nil || !strings.Contains(err.Error(), "food id is required") {
			t.Fatalf("expected food id validation error, got: %v", err)
		}
		if repo.deleteCalled {
			t.Error("repository Delete should not be called on validation failure")
		}
	})

	t.Run("get requires id", func(t *testing.T) {
		repo := &stubFoodRepository{}
		svc := NewNutritionService(repo, &cache.NopCache{}, testLogger())

		_, err := svc.GetFood(context.Background(), "user-1", "  ")
		if err == nil || !strings.Contains(err.Error(), "food id is required") {
			t.Fatalf("expected food id validation error, got: %v", err)
		}
	})
}

// --- Handler test helpers ---

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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

// newFoodServer mounts the food routes behind real JWT middleware and
// returns the mux together with a valid bearer token for user-1.
func newFoodServer(t *testing.T, repo FoodRepository) (*http.ServeMux, string) {
	t.Helper()
	mgr := auth.NewManager("test-secret", 1, 1)
	handler := NewNutritionHandler(NewNutritionService(repo, &cache.NopCache{}, testLogger()), testLogger())
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, mgr.Middleware)

	token, err := mgr.Generate("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return mux, token
}

// newMealServer mounts the meal routes behind real JWT middleware and
// returns the mux together with a valid bearer token for user-1.
func newMealServer(t *testing.T, repo MealRepository) (*http.ServeMux, string) {
	t.Helper()
	mgr := auth.NewManager("test-secret", 1, 1)
	handler := NewMealHandler(NewMealService(repo), testLogger())
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, mgr.Middleware)

	token, err := mgr.Generate("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}
	return mux, token
}

// --- Food handler tests ---

func TestFoodEndpointsRequireAuth(t *testing.T) {
	mux, _ := newFoodServer(t, &stubFoodRepository{})

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/food/create"},
		{http.MethodGet, "/food/view"},
		{http.MethodPut, "/food/update"},
		{http.MethodDelete, "/food/delete"},
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

func TestCreateFoodEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/food/create", token, CreateFoodRequest{
			Name: "oats", Calories100: 389, Protein100: 16.9, Carbohydrates100: 66.3, Fat100: 6.9,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var food Food
		if err := json.NewDecoder(rec.Body).Decode(&food); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if food.Name != "oats" || food.Calories100 != 389 {
			t.Errorf("unexpected food in response: %+v", food)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/food/create", token, CreateFoodRequest{Name: "oats", Calories100: 901})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		var apiErr lib.APIError
		if err := json.NewDecoder(rec.Body).Decode(&apiErr); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if apiErr.Code != http.StatusUnprocessableEntity || !strings.HasPrefix(apiErr.Message, "validation:") {
			t.Errorf("unexpected error body: %+v", apiErr)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		req := httptest.NewRequest(http.MethodPost, "/food/create", strings.NewReader("{not json"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodPost, "/food/create", token, CreateFoodRequest{Name: "oats", Calories100: 389})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestViewFoodEndpoint(t *testing.T) {
	t.Run("list all", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/food/view", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var foods []Food
		if err := json.NewDecoder(rec.Body).Decode(&foods); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(foods) != 1 {
			t.Errorf("expected 1 food, got %d", len(foods))
		}
	})

	t.Run("by id found", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{food: &Food{ID: "food-1", Name: "oats"}})
		rec := doJSON(t, mux, http.MethodGet, "/food/view?id=food-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("by id not found", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{food: nil})
		rec := doJSON(t, mux, http.MethodGet, "/food/view?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateFoodEndpoint(t *testing.T) {
	validReq := UpdateFoodRequest{ID: "food-1", Name: "oats v2", Calories100: 380}

	t.Run("found", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{food: &Food{ID: "food-1", Name: "oats v2"}})
		rec := doJSON(t, mux, http.MethodPut, "/food/update", token, validReq)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{food: nil})
		rec := doJSON(t, mux, http.MethodPut, "/food/update", token, validReq)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodPut, "/food/update", token, UpdateFoodRequest{Name: "oats"})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteFoodEndpoint(t *testing.T) {
	t.Run("valid id", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/food/delete?id=food-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newFoodServer(t, &stubFoodRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/food/delete", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// --- Meal handler tests ---

func TestMealEndpointsRequireAuth(t *testing.T) {
	mux, _ := newMealServer(t, &stubMealRepository{})

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/meal/create"},
		{http.MethodGet, "/meal/view"},
		{http.MethodGet, "/meal/by-date"},
		{http.MethodDelete, "/meal/del"},
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

func TestCreateMealEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/meal/create", token, CreateMealRequest{
			Name:  "breakfast",
			Foods: []MealFoodInput{{Name: "oats", WeightG: 150, Calories: 200, Protein: 10, Carbohydrates: 30, Fat: 5}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var meal Meal
		if err := json.NewDecoder(rec.Body).Decode(&meal); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if meal.Name != "breakfast" {
			t.Errorf("unexpected meal in response: %+v", meal)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/meal/create", token, CreateMealRequest{Name: ""})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestViewMealEndpoint(t *testing.T) {
	t.Run("list all", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/meal/view", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var meals []Meal
		if err := json.NewDecoder(rec.Body).Decode(&meals); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(meals) != 1 {
			t.Errorf("expected 1 meal, got %d", len(meals))
		}
	})

	t.Run("by id not found", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{meal: nil})
		rec := doJSON(t, mux, http.MethodGet, "/meal/view?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteMealEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		repo := &stubMealRepository{}
		mux, token := newMealServer(t, repo)
		rec := doJSON(t, mux, http.MethodDelete, "/meal/del?id=meal-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !repo.deleteCalled || repo.deletedID != "meal-1" {
			t.Errorf("expected repo.Delete called with meal-1, got called=%v id=%q", repo.deleteCalled, repo.deletedID)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/meal/del", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{deleteMissing: true})
		rec := doJSON(t, mux, http.MethodDelete, "/meal/del?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodDelete, "/meal/del?id=meal-1", token, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestListMealsByDateRangeService(t *testing.T) {
	sameDate := func(a, b time.Time) bool {
		ay, am, ad := a.Date()
		by, bm, bd := b.Date()
		return ay == by && am == bm && ad == bd
	}

	t.Run("empty params default to today", func(t *testing.T) {
		repo := &stubMealRepository{}
		svc := NewMealService(repo)
		if _, err := svc.ListMealsByDateRange(context.Background(), "user-1", "", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		now := time.Now()
		if !sameDate(repo.rangeFrom, now) || !sameDate(repo.rangeTo, now) {
			t.Errorf("expected both bounds to default to today, got from=%v to=%v", repo.rangeFrom, repo.rangeTo)
		}
	})

	t.Run("explicit range passed through", func(t *testing.T) {
		repo := &stubMealRepository{}
		svc := NewMealService(repo)
		if _, err := svc.ListMealsByDateRange(context.Background(), "user-1", "2026-06-01", "2026-06-12"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := func(s string) time.Time {
			d, _ := time.Parse("2006-01-02", s)
			return d
		}
		if !sameDate(repo.rangeFrom, want("2026-06-01")) || !sameDate(repo.rangeTo, want("2026-06-12")) {
			t.Errorf("unexpected range: from=%v to=%v", repo.rangeFrom, repo.rangeTo)
		}
	})

	t.Run("bad date format", func(t *testing.T) {
		svc := NewMealService(&stubMealRepository{})
		_, err := svc.ListMealsByDateRange(context.Background(), "user-1", "12-06-2026", "")
		var parseErr *time.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("expected *time.ParseError, got %v", err)
		}
	})

	t.Run("inverted range", func(t *testing.T) {
		svc := NewMealService(&stubMealRepository{})
		_, err := svc.ListMealsByDateRange(context.Background(), "user-1", "2026-06-12", "2026-06-01")
		if err == nil || !strings.HasPrefix(err.Error(), "validation:") {
			t.Fatalf("expected validation error, got %v", err)
		}
	})
}

func TestViewMealsByDateEndpoint(t *testing.T) {
	t.Run("valid range", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/meal/by-date?date_from=2026-06-01&date_to=2026-06-12", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var meals []Meal
		if err := json.NewDecoder(rec.Body).Decode(&meals); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(meals) != 1 {
			t.Errorf("expected 1 meal, got %d", len(meals))
		}
	})

	t.Run("no params defaults to today", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/meal/by-date", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bad date", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/meal/by-date?date_from=garbage", token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("inverted range", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/meal/by-date?date_from=2026-06-12&date_to=2026-06-01", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newMealServer(t, &stubMealRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodGet, "/meal/by-date", token, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
