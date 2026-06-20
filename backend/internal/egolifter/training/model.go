package training

import "time"

// Routine represents a saved training routine along with its entries.
type Routine struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Entries   []RoutineEntry `json:"entries"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// RoutineEntry is a planned exercise inside a routine.
type RoutineEntry struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int32   `json:"reps"`
}

// Workout represents a performed training along with its exercises.
type Workout struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Exercises   []Exercise `json:"exercises"`
	PerformedAt time.Time  `json:"performed_at"`
}

// Exercise is a single exercise recorded in a performed workout.
type Exercise struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int32   `json:"reps"`
}

// --- Request Payloads (Incoming) ---

type EntryInput struct {
	Name     string  `json:"name"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int32   `json:"reps"`
}

type CreateRoutineRequest struct {
	Name    string       `json:"name"`
	Entries []EntryInput `json:"entries"`
}

// LogWorkoutRequest logs a performed workout. When Exercises is non-empty the
// given (possibly edited) exercises are recorded; otherwise the routine's
// entries are snapshotted as-is. RoutineID may be omitted when Name and
// Exercises are provided.
type LogWorkoutRequest struct {
	RoutineID string       `json:"routine_id"`
	Name      string       `json:"name"`
	Exercises []EntryInput `json:"exercises"`
}
