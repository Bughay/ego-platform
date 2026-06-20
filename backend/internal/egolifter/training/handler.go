package training

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// TrainingHandler handles training-related HTTP requests.
type TrainingHandler struct {
	trainingSvc TrainingService
	logger      *slog.Logger
}

// NewTrainingHandler creates a new TrainingHandler.
func NewTrainingHandler(trainingSvc TrainingService, logger *slog.Logger) *TrainingHandler {
	return &TrainingHandler{trainingSvc: trainingSvc, logger: logger}
}

// RegisterRoutes attaches the training endpoints to the given mux,
// wrapping every route with the provided middleware (JWT auth).
func (h *TrainingHandler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("POST /training/routine/create", mw(http.HandlerFunc(h.CreateRoutine)))
	mux.Handle("GET /training/routine/view", mw(http.HandlerFunc(h.ViewRoutines)))
	mux.Handle("POST /training/log", mw(http.HandlerFunc(h.LogWorkout)))
	mux.Handle("GET /training/view", mw(http.HandlerFunc(h.ViewWorkouts)))
	mux.Handle("GET /training/by-date", mw(http.HandlerFunc(h.ViewWorkoutsByDate)))
	mux.Handle("DELETE /training/del", mw(http.HandlerFunc(h.DeleteWorkout)))
}

// userID extracts the authenticated user's ID from the JWT claims in the context.
func userID(r *http.Request) (string, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

// CreateRoutine saves a training routine along with its entries.
func (h *TrainingHandler) CreateRoutine(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated routine create attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req CreateRoutineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	routine, err := h.trainingSvc.SaveRoutine(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "routine validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to save routine", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to save routine")
		return
	}

	log.InfoContext(r.Context(), "routine saved", "routine_id", routine.ID, "entries", len(routine.Entries))
	lib.WriteJSON(w, http.StatusCreated, routine)
}

// ViewRoutines lists all of the user's training routines with their entries.
func (h *TrainingHandler) ViewRoutines(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated routine view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	routines, err := h.trainingSvc.ListRoutines(r.Context(), uid)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to list routines", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to list routines")
		return
	}

	log.InfoContext(r.Context(), "routines listed", "count", len(routines))
	lib.WriteJSON(w, http.StatusOK, routines)
}

// LogWorkout records that a routine was performed, snapshotting its entries
// into a new workout with exercises.
func (h *TrainingHandler) LogWorkout(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated workout log attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req LogWorkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	workout, err := h.trainingSvc.LogRoutine(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "workout log validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to log workout", "routine_id", req.RoutineID, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to log workout")
		return
	}
	if workout == nil {
		log.WarnContext(r.Context(), "routine not found for log", "routine_id", req.RoutineID)
		lib.WriteError(w, http.StatusNotFound, "routine not found")
		return
	}

	log.InfoContext(r.Context(), "workout logged", "workout_id", workout.ID, "routine_id", req.RoutineID, "exercises", len(workout.Exercises))
	lib.WriteJSON(w, http.StatusCreated, workout)
}

// ViewWorkouts lists the workouts performed on ?date=YYYY-MM-DD (default: today).
func (h *TrainingHandler) ViewWorkouts(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated workout view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	date := r.URL.Query().Get("date")
	workouts, err := h.trainingSvc.ListWorkoutsByDate(r.Context(), uid, date)
	if err != nil {
		var parseErr *time.ParseError
		if errors.As(err, &parseErr) {
			log.WarnContext(r.Context(), "invalid date parameter", "date", date, "error", err)
			lib.WriteError(w, http.StatusBadRequest, "invalid date, expected YYYY-MM-DD")
			return
		}
		log.ErrorContext(r.Context(), "failed to list workouts", "date", date, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to list workouts")
		return
	}

	log.InfoContext(r.Context(), "workouts listed", "date", date, "count", len(workouts))
	lib.WriteJSON(w, http.StatusOK, workouts)
}

// ViewWorkoutsByDate lists the workouts performed between ?date_from= and
// ?date_to= (YYYY-MM-DD, inclusive); either bound defaults to today.
func (h *TrainingHandler) ViewWorkoutsByDate(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated workout view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")

	workouts, err := h.trainingSvc.ListWorkoutsByDateRange(r.Context(), uid, dateFrom, dateTo)
	if err != nil {
		var parseErr *time.ParseError
		if errors.As(err, &parseErr) {
			log.WarnContext(r.Context(), "invalid date parameter", "date_from", dateFrom, "date_to", dateTo, "error", err)
			lib.WriteError(w, http.StatusBadRequest, "invalid date, expected YYYY-MM-DD")
			return
		}
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "invalid date range", "date_from", dateFrom, "date_to", dateTo, "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to list workouts by date", "date_from", dateFrom, "date_to", dateTo, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to list workouts")
		return
	}

	log.InfoContext(r.Context(), "workouts listed by date", "date_from", dateFrom, "date_to", dateTo, "count", len(workouts))
	lib.WriteJSON(w, http.StatusOK, workouts)
}

// DeleteWorkout removes the workout given by ?id= together with all of its
// exercise rows.
func (h *TrainingHandler) DeleteWorkout(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated workout delete attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")

	found, err := h.trainingSvc.DeleteWorkout(r.Context(), uid, id)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "workout validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to delete workout", "workout_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to delete workout")
		return
	}
	if !found {
		log.WarnContext(r.Context(), "workout not found", "workout_id", id)
		lib.WriteError(w, http.StatusNotFound, "workout not found")
		return
	}

	log.InfoContext(r.Context(), "workout deleted", "workout_id", id)
	lib.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "workout deleted successfully",
	})
}

// isValidationErr checks if the error originated from a validation rule.
func isValidationErr(err error) bool {
	return strings.HasPrefix(err.Error(), "validation:")
}
