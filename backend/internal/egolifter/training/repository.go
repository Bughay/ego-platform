package training

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TrainingRepository defines the contract for training data access.
type TrainingRepository interface {
	CreateRoutine(ctx context.Context, userID string, req *CreateRoutineRequest) (*Routine, error)
	FindRoutineByID(ctx context.Context, userID, id string) (*Routine, error)
	ListRoutines(ctx context.Context, userID string) ([]Routine, error)
	LogWorkout(ctx context.Context, userID, name string, entries []RoutineEntry) (*Workout, error)
	ListWorkoutsByDate(ctx context.Context, userID string, date time.Time) ([]Workout, error)
	ListWorkoutsByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Workout, error)
	DeleteWorkout(ctx context.Context, userID, id string) (bool, error)
}

type pgTrainingRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewTrainingRepository creates a new PostgreSQL-backed TrainingRepository wrapping the sqlc-generated queries.
func NewTrainingRepository(pool *pgxpool.Pool) TrainingRepository {
	return &pgTrainingRepository{pool: pool, queries: db.New(pool)}
}

// CreateRoutine inserts the routine and its entries in a single transaction.
func (r *pgTrainingRepository) CreateRoutine(ctx context.Context, userID string, req *CreateRoutineRequest) (*Routine, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.CreateRoutine: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateWorkoutRoutine(ctx, db.CreateWorkoutRoutineParams{
		UserID: userID,
		Name:   req.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.CreateRoutine: %w", err)
	}

	for _, entry := range req.Entries {
		if _, err := qtx.CreateWorkoutRoutineEntry(ctx, db.CreateWorkoutRoutineEntryParams{
			WorkoutRoutineID: row.ID,
			Name:             entry.Name,
			WeightKg:         entry.WeightKg,
			Reps:             entry.Reps,
		}); err != nil {
			return nil, fmt.Errorf("trainingRepo.CreateRoutine: insert entry %q: %w", entry.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("trainingRepo.CreateRoutine: commit: %w", err)
	}

	return r.FindRoutineByID(ctx, userID, row.ID)
}

func (r *pgTrainingRepository) FindRoutineByID(ctx context.Context, userID, id string) (*Routine, error) {
	row, err := r.queries.GetWorkoutRoutineByID(ctx, db.GetWorkoutRoutineByIDParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found is not an error at this layer
		}
		return nil, fmt.Errorf("trainingRepo.FindRoutineByID: %w", err)
	}

	routine := toRoutine(row)
	if err := r.attachEntries(ctx, routine); err != nil {
		return nil, fmt.Errorf("trainingRepo.FindRoutineByID: %w", err)
	}
	return routine, nil
}

func (r *pgTrainingRepository) ListRoutines(ctx context.Context, userID string) ([]Routine, error) {
	rows, err := r.queries.ListWorkoutRoutines(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.ListRoutines: %w", err)
	}

	routines := make([]Routine, 0, len(rows))
	for _, row := range rows {
		routine := toRoutine(row)
		if err := r.attachEntries(ctx, routine); err != nil {
			return nil, fmt.Errorf("trainingRepo.ListRoutines: %w", err)
		}
		routines = append(routines, *routine)
	}
	return routines, nil
}

// LogWorkout records a performed workout and its exercises in a single transaction.
func (r *pgTrainingRepository) LogWorkout(ctx context.Context, userID, name string, entries []RoutineEntry) (*Workout, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.LogWorkout: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateWorkout(ctx, db.CreateWorkoutParams{
		UserID: userID,
		Name:   name,
	})
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.LogWorkout: %w", err)
	}

	workout := toWorkout(row)
	for _, entry := range entries {
		exRow, err := qtx.CreateExercise(ctx, db.CreateExerciseParams{
			WorkoutID: row.ID,
			Name:      entry.Name,
			WeightKg:  entry.WeightKg,
			Reps:      entry.Reps,
		})
		if err != nil {
			return nil, fmt.Errorf("trainingRepo.LogWorkout: insert exercise %q: %w", entry.Name, err)
		}
		workout.Exercises = append(workout.Exercises, toExercise(exRow))
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("trainingRepo.LogWorkout: commit: %w", err)
	}

	return workout, nil
}

func (r *pgTrainingRepository) ListWorkoutsByDate(ctx context.Context, userID string, date time.Time) ([]Workout, error) {
	rows, err := r.queries.ListWorkoutsByDate(ctx, db.ListWorkoutsByDateParams{
		UserID:  userID,
		Column2: pgtype.Date{Time: date, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.ListWorkoutsByDate: %w", err)
	}

	workouts := make([]Workout, 0, len(rows))
	for _, row := range rows {
		workout := toWorkout(row)
		exRows, err := r.queries.ListExercisesByWorkout(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("trainingRepo.ListWorkoutsByDate: exercises: %w", err)
		}
		for _, exRow := range exRows {
			workout.Exercises = append(workout.Exercises, toExercise(exRow))
		}
		workouts = append(workouts, *workout)
	}
	return workouts, nil
}

func (r *pgTrainingRepository) ListWorkoutsByDateRange(ctx context.Context, userID string, from, to time.Time) ([]Workout, error) {
	rows, err := r.queries.ListWorkoutsByDateRange(ctx, db.ListWorkoutsByDateRangeParams{
		UserID:   userID,
		DateFrom: pgtype.Date{Time: from, Valid: true},
		DateTo:   pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("trainingRepo.ListWorkoutsByDateRange: %w", err)
	}

	workouts := make([]Workout, 0, len(rows))
	for _, row := range rows {
		workout := toWorkout(row)
		exRows, err := r.queries.ListExercisesByWorkout(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("trainingRepo.ListWorkoutsByDateRange: exercises: %w", err)
		}
		for _, exRow := range exRows {
			workout.Exercises = append(workout.Exercises, toExercise(exRow))
		}
		workouts = append(workouts, *workout)
	}
	return workouts, nil
}

// DeleteWorkout removes the workout and all of its exercise rows; the returned
// bool reports whether a workout was actually deleted.
func (r *pgTrainingRepository) DeleteWorkout(ctx context.Context, userID, id string) (bool, error) {
	rows, err := r.queries.DeleteWorkout(ctx, db.DeleteWorkoutParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("trainingRepo.DeleteWorkout: %w", err)
	}
	return rows > 0, nil
}

// attachEntries loads the routine's entries and sets them on the domain model.
func (r *pgTrainingRepository) attachEntries(ctx context.Context, routine *Routine) error {
	entryRows, err := r.queries.ListWorkoutRoutineEntries(ctx, routine.ID)
	if err != nil {
		return fmt.Errorf("entries: %w", err)
	}
	for _, er := range entryRows {
		routine.Entries = append(routine.Entries, RoutineEntry{
			ID:       er.ID,
			Name:     er.Name,
			WeightKg: er.WeightKg,
			Reps:     er.Reps,
		})
	}
	return nil
}

// toRoutine maps a sqlc-generated row to the domain model (without entries).
func toRoutine(row db.WorkoutRoutine) *Routine {
	return &Routine{
		ID:        row.ID,
		Name:      row.Name,
		Entries:   []RoutineEntry{},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// toWorkout maps a sqlc-generated row to the domain model (without exercises).
func toWorkout(row db.Workout) *Workout {
	return &Workout{
		ID:          row.ID,
		Name:        row.Name,
		Exercises:   []Exercise{},
		PerformedAt: row.CreatedAt,
	}
}

func toExercise(row db.Exercise) Exercise {
	return Exercise{
		ID:       row.ID,
		Name:     row.Name,
		WeightKg: row.WeightKg,
		Reps:     row.Reps,
	}
}
