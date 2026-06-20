package training

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
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// stubTrainingRepository records calls and returns configurable fixtures.
type stubTrainingRepository struct {
	err     error    // when set, every method fails with it
	routine *Routine // returned by FindRoutineByID (nil = not found)

	createRoutineCalled bool
	logWorkoutCalled    bool
	loggedName          string
	loggedEntries       []RoutineEntry
	listDateCalled      bool
	listDate            time.Time

	rangeFrom time.Time // recorded by ListWorkoutsByDateRange
	rangeTo   time.Time

	deleteCalled  bool
	deletedID     string // recorded by DeleteWorkout
	deleteMissing bool   // when set, DeleteWorkout reports the workout as not found
}

func (s *stubTrainingRepository) CreateRoutine(ctx context.Context, userID string, req *CreateRoutineRequest) (*Routine, error) {
	s.createRoutineCalled = true
	if s.err != nil {
		return nil, s.err
	}
	entries := make([]RoutineEntry, 0, len(req.Entries))
	for i, e := range req.Entries {
		entries = append(entries, RoutineEntry{ID: fmt.Sprintf("entry-%d", i), Name: e.Name, WeightKg: e.WeightKg, Reps: e.Reps})
	}
	return &Routine{ID: "routine-1", Name: req.Name, Entries: entries}, nil
}

func (s *stubTrainingRepository) FindRoutineByID(ctx context.Context, userID, id string) (*Routine, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.routine, nil
}

func (s *stubTrainingRepository) ListRoutines(ctx context.Context, userID string) ([]Routine, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Routine{{ID: "routine-1", Name: "push day", Entries: []RoutineEntry{}}}, nil
}

func (s *stubTrainingRepository) LogWorkout(ctx context.Context, userID, name string, entries []RoutineEntry) (*Workout, error) {
	s.logWorkoutCalled = true
	s.loggedName = name
	s.loggedEntries = entries
	if s.err != nil {
		return nil, s.err
	}
	exercises := make([]Exercise, 0, len(entries))
	for i, e := range entries {
		exercises = append(exercises, Exercise{ID: fmt.Sprintf("exercise-%d", i), Name: e.Name, WeightKg: e.WeightKg, Reps: e.Reps})
	}
	return &Workout{ID: "workout-1", Name: name, Exercises: exercises, PerformedAt: time.Now()}, nil
}

func (s *stubTrainingRepository) ListWorkoutsByDate(ctx context.Context, userID string, date time.Time) ([]Workout, error) {
	s.listDateCalled = true
	s.listDate = date
	if s.err != nil {
		return nil, s.err
	}
	return []Workout{{ID: "workout-1", Name: "push day", Exercises: []Exercise{}, PerformedAt: date}}, nil
}

func (s *stubTrainingRepository) ListWorkoutsByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Workout, error) {
	s.rangeFrom, s.rangeTo = from, to
	if s.err != nil {
		return nil, s.err
	}
	return []Workout{{ID: "workout-1", Name: "push day", Exercises: []Exercise{}, PerformedAt: from}}, nil
}

func (s *stubTrainingRepository) DeleteWorkout(ctx context.Context, userID, id string) (bool, error) {
	s.deleteCalled = true
	s.deletedID = id
	if s.err != nil {
		return false, s.err
	}
	return !s.deleteMissing, nil
}

// --- Service tests ---

func TestSaveRoutineValidation(t *testing.T) {
	validEntries := []EntryInput{
		{Name: "bench press", WeightKg: 75.5, Reps: 8},
		{Name: "push up", WeightKg: 0, Reps: 20},
	}
	longName := strings.Repeat("a", 101)

	tests := []struct {
		name    string
		req     *CreateRoutineRequest
		wantErr string // substring of the expected error; empty means success
	}{
		{
			name: "valid request",
			req:  &CreateRoutineRequest{Name: "push day", Entries: validEntries},
		},
		{
			name: "boundary values are valid",
			req: &CreateRoutineRequest{Name: "limits", Entries: []EntryInput{
				{Name: "squat", WeightKg: 1000, Reps: 100},
				{Name: "plank", WeightKg: 0, Reps: 1},
			}},
		},
		{
			name:    "empty name",
			req:     &CreateRoutineRequest{Name: "", Entries: validEntries},
			wantErr: "routine name is required",
		},
		{
			name:    "whitespace name",
			req:     &CreateRoutineRequest{Name: "   ", Entries: validEntries},
			wantErr: "routine name is required",
		},
		{
			name:    "name too long",
			req:     &CreateRoutineRequest{Name: longName, Entries: validEntries},
			wantErr: "at most 100 characters",
		},
		{
			name:    "no entries",
			req:     &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{}},
			wantErr: "at least one entry",
		},
		{
			name: "blank entry name",
			req: &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{
				{Name: "  ", WeightKg: 50, Reps: 10},
			}},
			wantErr: "entry 0: name is required",
		},
		{
			name: "negative weight",
			req: &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{
				{Name: "bench press", WeightKg: -1, Reps: 10},
			}},
			wantErr: "weight_kg must be between 0 and 1000",
		},
		{
			name: "absurd weight",
			req: &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{
				{Name: "bench press", WeightKg: 1001, Reps: 10},
			}},
			wantErr: "weight_kg must be between 0 and 1000",
		},
		{
			name: "zero reps",
			req: &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{
				{Name: "bench press", WeightKg: 50, Reps: 0},
			}},
			wantErr: "reps must be between 1 and 100",
		},
		{
			name: "absurd reps",
			req: &CreateRoutineRequest{Name: "push day", Entries: []EntryInput{
				{Name: "bench press", WeightKg: 50, Reps: 101},
			}},
			wantErr: "reps must be between 1 and 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubTrainingRepository{}
			svc := NewTrainingService(repo)

			routine, err := svc.SaveRoutine(context.Background(), "user-1", tt.req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if !repo.createRoutineCalled {
					t.Fatal("expected repository CreateRoutine to be called")
				}
				if routine == nil || routine.Name != tt.req.Name {
					t.Fatalf("unexpected routine returned: %+v", routine)
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
			if repo.createRoutineCalled {
				t.Error("repository CreateRoutine should not be called on validation failure")
			}
		})
	}
}

func TestLogRoutine(t *testing.T) {
	routine := &Routine{
		ID:   "routine-1",
		Name: "push day",
		Entries: []RoutineEntry{
			{ID: "entry-0", Name: "bench press", WeightKg: 75.5, Reps: 8},
			{ID: "entry-1", Name: "push up", WeightKg: 0, Reps: 20},
		},
	}

	t.Run("blank routine_id", func(t *testing.T) {
		repo := &stubTrainingRepository{routine: routine}
		svc := NewTrainingService(repo)

		_, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{RoutineID: "  "})
		if err == nil || !strings.Contains(err.Error(), "routine_id is required") {
			t.Fatalf("expected routine_id validation error, got: %v", err)
		}
		if !strings.HasPrefix(err.Error(), "validation:") {
			t.Errorf("expected validation error, got: %v", err)
		}
		if repo.logWorkoutCalled {
			t.Error("repository LogWorkout should not be called on validation failure")
		}
	})

	t.Run("unknown routine", func(t *testing.T) {
		repo := &stubTrainingRepository{routine: nil}
		svc := NewTrainingService(repo)

		workout, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{RoutineID: "missing"})
		if err != nil {
			t.Fatalf("expected no error for unknown routine, got: %v", err)
		}
		if workout != nil {
			t.Fatalf("expected nil workout for unknown routine, got: %+v", workout)
		}
		if repo.logWorkoutCalled {
			t.Error("repository LogWorkout should not be called for unknown routine")
		}
	})

	t.Run("success snapshots routine entries", func(t *testing.T) {
		repo := &stubTrainingRepository{routine: routine}
		svc := NewTrainingService(repo)

		workout, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{RoutineID: "routine-1"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !repo.logWorkoutCalled {
			t.Fatal("expected repository LogWorkout to be called")
		}
		if repo.loggedName != routine.Name {
			t.Errorf("expected workout named %q, got %q", routine.Name, repo.loggedName)
		}
		if len(repo.loggedEntries) != len(routine.Entries) {
			t.Fatalf("expected %d entries logged, got %d", len(routine.Entries), len(repo.loggedEntries))
		}
		for i, e := range routine.Entries {
			got := repo.loggedEntries[i]
			if got.Name != e.Name || got.WeightKg != e.WeightKg || got.Reps != e.Reps {
				t.Errorf("entry %d: expected %+v, got %+v", i, e, got)
			}
		}
		if workout == nil || workout.Name != routine.Name || len(workout.Exercises) != len(routine.Entries) {
			t.Fatalf("unexpected workout returned: %+v", workout)
		}
	})

	t.Run("repository error propagates", func(t *testing.T) {
		repo := &stubTrainingRepository{err: errors.New("db down")}
		svc := NewTrainingService(repo)

		_, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{RoutineID: "routine-1"})
		if err == nil || !strings.Contains(err.Error(), "db down") {
			t.Fatalf("expected repository error to propagate, got: %v", err)
		}
	})

	t.Run("custom exercises override routine entries", func(t *testing.T) {
		repo := &stubTrainingRepository{routine: routine}
		svc := NewTrainingService(repo)

		edited := []EntryInput{
			{Name: "bench press", WeightKg: 80, Reps: 6}, // heavier than the routine's 75.5
			{Name: "dips", WeightKg: 10, Reps: 12},       // exercise not in the routine
		}
		workout, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{RoutineID: "routine-1", Exercises: edited})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if repo.loggedName != routine.Name {
			t.Errorf("expected workout named %q, got %q", routine.Name, repo.loggedName)
		}
		if len(repo.loggedEntries) != 2 {
			t.Fatalf("expected the 2 edited exercises logged, got %d", len(repo.loggedEntries))
		}
		for i, e := range edited {
			got := repo.loggedEntries[i]
			if got.Name != e.Name || got.WeightKg != e.WeightKg || got.Reps != e.Reps {
				t.Errorf("exercise %d: expected %+v, got %+v", i, e, got)
			}
		}
		if workout == nil || len(workout.Exercises) != 2 {
			t.Fatalf("unexpected workout returned: %+v", workout)
		}
	})

	t.Run("exercises without routine but with name", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)

		workout, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{
			Name:      "improvised session",
			Exercises: []EntryInput{{Name: "squat", WeightKg: 100, Reps: 5}},
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if workout == nil || repo.loggedName != "improvised session" {
			t.Fatalf("expected custom-named workout, got name %q workout %+v", repo.loggedName, workout)
		}
	})

	t.Run("invalid custom exercise rejected", func(t *testing.T) {
		repo := &stubTrainingRepository{routine: routine}
		svc := NewTrainingService(repo)

		_, err := svc.LogRoutine(context.Background(), "user-1", &LogWorkoutRequest{
			RoutineID: "routine-1",
			Exercises: []EntryInput{{Name: "bench press", WeightKg: 80, Reps: 0}},
		})
		if err == nil || !strings.HasPrefix(err.Error(), "validation:") {
			t.Fatalf("expected validation error, got: %v", err)
		}
		if repo.logWorkoutCalled {
			t.Error("repository LogWorkout should not be called on validation failure")
		}
	})
}

func TestListWorkoutsByDateRangeService(t *testing.T) {
	sameDate := func(a, b time.Time) bool {
		ay, am, ad := a.Date()
		by, bm, bd := b.Date()
		return ay == by && am == bm && ad == bd
	}

	t.Run("empty params default to today", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)
		if _, err := svc.ListWorkoutsByDateRange(context.Background(), "user-1", "", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		now := time.Now()
		if !sameDate(repo.rangeFrom, now) || !sameDate(repo.rangeTo, now) {
			t.Errorf("expected both bounds to default to today, got from=%v to=%v", repo.rangeFrom, repo.rangeTo)
		}
	})

	t.Run("explicit range passed through", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)
		if _, err := svc.ListWorkoutsByDateRange(context.Background(), "user-1", "2026-06-01", "2026-06-12"); err != nil {
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
		svc := NewTrainingService(&stubTrainingRepository{})
		_, err := svc.ListWorkoutsByDateRange(context.Background(), "user-1", "12-06-2026", "")
		var parseErr *time.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("expected *time.ParseError, got %v", err)
		}
	})

	t.Run("inverted range", func(t *testing.T) {
		svc := NewTrainingService(&stubTrainingRepository{})
		_, err := svc.ListWorkoutsByDateRange(context.Background(), "user-1", "2026-06-12", "2026-06-01")
		if err == nil || !strings.HasPrefix(err.Error(), "validation:") {
			t.Fatalf("expected validation error, got %v", err)
		}
	})
}

func TestListWorkoutsByDate(t *testing.T) {
	t.Run("invalid date format", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)

		_, err := svc.ListWorkoutsByDate(context.Background(), "user-1", "not-a-date")
		var parseErr *time.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("expected *time.ParseError, got: %v", err)
		}
		if repo.listDateCalled {
			t.Error("repository should not be called for an invalid date")
		}
	})

	t.Run("explicit date is parsed", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)

		workouts, err := svc.ListWorkoutsByDate(context.Background(), "user-1", "2026-06-11")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		want := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
		if !repo.listDate.Equal(want) {
			t.Errorf("expected repository to receive %v, got %v", want, repo.listDate)
		}
		if len(workouts) != 1 {
			t.Errorf("expected 1 workout, got %d", len(workouts))
		}
	})

	t.Run("empty date defaults to today", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		svc := NewTrainingService(repo)

		if _, err := svc.ListWorkoutsByDate(context.Background(), "user-1", ""); err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !repo.listDateCalled {
			t.Fatal("expected repository to be called")
		}
		if time.Since(repo.listDate) > time.Minute {
			t.Errorf("expected repository to receive ~now, got %v", repo.listDate)
		}
	})
}

// --- Handler tests ---

// newTrainingServer mounts the training routes behind real JWT middleware and
// returns the mux together with a valid bearer token for user-1.
func newTrainingServer(t *testing.T, repo TrainingRepository) (*http.ServeMux, string) {
	t.Helper()
	mgr := auth.NewManager("test-secret", 1, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewTrainingHandler(NewTrainingService(repo), logger)
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

func TestTrainingEndpointsRequireAuth(t *testing.T) {
	mux, _ := newTrainingServer(t, &stubTrainingRepository{})

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/training/routine/create"},
		{http.MethodGet, "/training/routine/view"},
		{http.MethodPost, "/training/log"},
		{http.MethodGet, "/training/view"},
		{http.MethodGet, "/training/by-date"},
		{http.MethodDelete, "/training/del"},
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

func TestCreateRoutineEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/training/routine/create", token, CreateRoutineRequest{
			Name:    "push day",
			Entries: []EntryInput{{Name: "bench press", WeightKg: 75.5, Reps: 8}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var routine Routine
		if err := json.NewDecoder(rec.Body).Decode(&routine); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if routine.Name != "push day" || len(routine.Entries) != 1 {
			t.Errorf("unexpected routine in response: %+v", routine)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodPost, "/training/routine/create", token, CreateRoutineRequest{Name: ""})
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
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		req := httptest.NewRequest(http.MethodPost, "/training/routine/create", strings.NewReader("{not json"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodPost, "/training/routine/create", token, CreateRoutineRequest{
			Name:    "push day",
			Entries: []EntryInput{{Name: "bench press", WeightKg: 75.5, Reps: 8}},
		})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestViewRoutinesEndpoint(t *testing.T) {
	mux, token := newTrainingServer(t, &stubTrainingRepository{})
	rec := doJSON(t, mux, http.MethodGet, "/training/routine/view", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var routines []Routine
	if err := json.NewDecoder(rec.Body).Decode(&routines); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(routines) != 1 || routines[0].Name != "push day" {
		t.Errorf("unexpected routines in response: %+v", routines)
	}
}

func TestLogWorkoutEndpoint(t *testing.T) {
	routine := &Routine{
		ID:      "routine-1",
		Name:    "push day",
		Entries: []RoutineEntry{{ID: "entry-0", Name: "bench press", WeightKg: 75.5, Reps: 8}},
	}

	t.Run("valid request", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{routine: routine})
		rec := doJSON(t, mux, http.MethodPost, "/training/log", token, LogWorkoutRequest{RoutineID: "routine-1"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var workout Workout
		if err := json.NewDecoder(rec.Body).Decode(&workout); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if workout.Name != "push day" || len(workout.Exercises) != 1 {
			t.Errorf("unexpected workout in response: %+v", workout)
		}
	})

	t.Run("unknown routine", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{routine: nil})
		rec := doJSON(t, mux, http.MethodPost, "/training/log", token, LogWorkoutRequest{RoutineID: "missing"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("blank routine_id", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{routine: routine})
		rec := doJSON(t, mux, http.MethodPost, "/training/log", token, LogWorkoutRequest{RoutineID: ""})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestViewWorkoutsEndpoint(t *testing.T) {
	t.Run("valid date", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/view?date=2026-06-11", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var workouts []Workout
		if err := json.NewDecoder(rec.Body).Decode(&workouts); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(workouts) != 1 {
			t.Errorf("expected 1 workout, got %d", len(workouts))
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/view?date=not-a-date", token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		var apiErr lib.APIError
		if err := json.NewDecoder(rec.Body).Decode(&apiErr); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if !strings.Contains(apiErr.Message, "YYYY-MM-DD") {
			t.Errorf("expected date format hint in message, got: %q", apiErr.Message)
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodGet, "/training/view", token, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", rec.Code)
		}
	})
}

func TestViewWorkoutsByDateEndpoint(t *testing.T) {
	t.Run("valid range", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/by-date?date_from=2026-06-01&date_to=2026-06-12", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var workouts []Workout
		if err := json.NewDecoder(rec.Body).Decode(&workouts); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(workouts) != 1 {
			t.Errorf("expected 1 workout, got %d", len(workouts))
		}
	})

	t.Run("no params defaults to today", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/by-date", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bad date", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/by-date?date_from=garbage", token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("inverted range", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodGet, "/training/by-date?date_from=2026-06-12&date_to=2026-06-01", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteWorkoutEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		repo := &stubTrainingRepository{}
		mux, token := newTrainingServer(t, repo)
		rec := doJSON(t, mux, http.MethodDelete, "/training/del?id=workout-1", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !repo.deleteCalled || repo.deletedID != "workout-1" {
			t.Errorf("expected repo.DeleteWorkout called with workout-1, got called=%v id=%q", repo.deleteCalled, repo.deletedID)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{})
		rec := doJSON(t, mux, http.MethodDelete, "/training/del", token, nil)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{deleteMissing: true})
		rec := doJSON(t, mux, http.MethodDelete, "/training/del?id=missing", token, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		mux, token := newTrainingServer(t, &stubTrainingRepository{err: errors.New("db down")})
		rec := doJSON(t, mux, http.MethodDelete, "/training/del?id=workout-1", token, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
