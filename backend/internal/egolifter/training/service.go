package training

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TrainingService defines the contract for training business logic.
type TrainingService interface {
	SaveRoutine(ctx context.Context, userID string, req *CreateRoutineRequest) (*Routine, error)
	ListRoutines(ctx context.Context, userID string) ([]Routine, error)
	LogRoutine(ctx context.Context, userID string, req *LogWorkoutRequest) (*Workout, error)
	ListWorkoutsByDate(ctx context.Context, userID, dateStr string) ([]Workout, error)
	ListWorkoutsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]Workout, error)
	DeleteWorkout(ctx context.Context, userID, id string) (bool, error)
}

type trainingService struct {
	trainingRepo TrainingRepository
}

// NewTrainingService creates a new TrainingService.
func NewTrainingService(trainingRepo TrainingRepository) TrainingService {
	return &trainingService{trainingRepo: trainingRepo}
}

func (s *trainingService) SaveRoutine(ctx context.Context, userID string, req *CreateRoutineRequest) (*Routine, error) {
	if err := validateRoutineInput(req.Name, req.Entries); err != nil {
		return nil, err
	}
	return s.trainingRepo.CreateRoutine(ctx, userID, req)
}

func (s *trainingService) ListRoutines(ctx context.Context, userID string) ([]Routine, error) {
	return s.trainingRepo.ListRoutines(ctx, userID)
}

// LogRoutine records a performed workout. When req.Exercises is non-empty
// those (possibly edited) exercises are logged; otherwise the routine's
// entries are snapshotted as-is. The routine only serves as a template, so
// RoutineID may be omitted when Name and Exercises are provided.
// Returns (nil, nil) when the routine does not exist or belongs to another user.
func (s *trainingService) LogRoutine(ctx context.Context, userID string, req *LogWorkoutRequest) (*Workout, error) {
	name := strings.TrimSpace(req.Name)
	routineID := strings.TrimSpace(req.RoutineID)

	if routineID == "" && len(req.Exercises) == 0 {
		return nil, fmt.Errorf("validation: routine_id is required")
	}

	if routineID != "" {
		routine, err := s.trainingRepo.FindRoutineByID(ctx, userID, routineID)
		if err != nil {
			return nil, err
		}
		if routine == nil {
			return nil, nil
		}
		if name == "" {
			name = routine.Name
		}
		if len(req.Exercises) == 0 {
			return s.trainingRepo.LogWorkout(ctx, userID, name, routine.Entries)
		}
	}

	if err := validateRoutineInput(name, req.Exercises); err != nil {
		return nil, err
	}
	entries := make([]RoutineEntry, 0, len(req.Exercises))
	for _, ex := range req.Exercises {
		entries = append(entries, RoutineEntry{Name: ex.Name, WeightKg: ex.WeightKg, Reps: ex.Reps})
	}
	return s.trainingRepo.LogWorkout(ctx, userID, name, entries)
}

// ListWorkoutsByDate lists workouts performed on the given YYYY-MM-DD date,
// defaulting to today when dateStr is empty.
func (s *trainingService) ListWorkoutsByDate(ctx context.Context, userID, dateStr string) ([]Workout, error) {
	date := time.Now()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, err
		}
		date = parsed
	}
	return s.trainingRepo.ListWorkoutsByDate(ctx, userID, date)
}

// ListWorkoutsByDateRange lists workouts performed between the given
// YYYY-MM-DD dates (inclusive); either bound defaults to today when empty.
func (s *trainingService) ListWorkoutsByDateRange(ctx context.Context, userID, fromStr, toStr string) ([]Workout, error) {
	parseOrToday := func(s string) (time.Time, error) {
		if s == "" {
			return time.Now(), nil
		}
		return time.Parse("2006-01-02", s)
	}

	from, err := parseOrToday(fromStr)
	if err != nil {
		return nil, err
	}
	to, err := parseOrToday(toStr)
	if err != nil {
		return nil, err
	}

	fy, fm, fd := from.Date()
	ty, tm, td := to.Date()
	if time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC).After(time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC)) {
		return nil, fmt.Errorf("validation: date_from must not be after date_to")
	}

	return s.trainingRepo.ListWorkoutsByDateRange(ctx, userID, from, to)
}

// DeleteWorkout removes the workout and all of its exercises; the returned
// bool reports whether the workout existed.
func (s *trainingService) DeleteWorkout(ctx context.Context, userID, id string) (bool, error) {
	if strings.TrimSpace(id) == "" {
		return false, fmt.Errorf("validation: workout id is required")
	}
	return s.trainingRepo.DeleteWorkout(ctx, userID, id)
}
