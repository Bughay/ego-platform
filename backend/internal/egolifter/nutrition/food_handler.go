package nutrition

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Bughay/egolifter/internal/shared/lib"
)

// NutritionHandler handles nutrition-related HTTP requests.
type NutritionHandler struct {
	nutritionSvc NutritionService
	logger       *slog.Logger
}

// NewNutritionHandler creates a new NutritionHandler.
func NewNutritionHandler(nutritionSvc NutritionService, logger *slog.Logger) *NutritionHandler {
	return &NutritionHandler{nutritionSvc: nutritionSvc, logger: logger}
}

// RegisterRoutes attaches the food CRUD endpoints to the given mux,
// wrapping every route with the provided middleware (JWT auth).
func (h *NutritionHandler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("POST /food/create", mw(http.HandlerFunc(h.CreateFood)))
	mux.Handle("GET /food/view", mw(http.HandlerFunc(h.ViewFood)))
	mux.Handle("PUT /food/update", mw(http.HandlerFunc(h.UpdateFood)))
	mux.Handle("DELETE /food/delete", mw(http.HandlerFunc(h.DeleteFood)))
}

func (h *NutritionHandler) CreateFood(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated food create attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req CreateFoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	food, err := h.nutritionSvc.CreateFood(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "food validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to create food", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to create food")
		return
	}

	log.InfoContext(r.Context(), "food created", "food_id", food.ID, "name", food.Name)
	lib.WriteJSON(w, http.StatusCreated, food)
}

// ViewFood returns a single food when ?id= is given, otherwise lists all foods.
func (h *NutritionHandler) ViewFood(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated food view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")
	if id == "" {
		foods, err := h.nutritionSvc.ListFoods(r.Context(), uid)
		if err != nil {
			log.ErrorContext(r.Context(), "failed to list foods", "error", err)
			lib.WriteError(w, http.StatusInternalServerError, "failed to list foods")
			return
		}
		log.InfoContext(r.Context(), "foods listed", "count", len(foods))
		lib.WriteJSON(w, http.StatusOK, foods)
		return
	}

	food, err := h.nutritionSvc.GetFood(r.Context(), uid, id)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "food validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to get food", "food_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to get food")
		return
	}
	if food == nil {
		log.WarnContext(r.Context(), "food not found", "food_id", id)
		lib.WriteError(w, http.StatusNotFound, "food not found")
		return
	}

	log.InfoContext(r.Context(), "food retrieved", "food_id", id)
	lib.WriteJSON(w, http.StatusOK, food)
}

func (h *NutritionHandler) UpdateFood(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated food update attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req UpdateFoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	food, err := h.nutritionSvc.UpdateFood(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "food validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to update food", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to update food")
		return
	}
	if food == nil {
		log.WarnContext(r.Context(), "food not found for update", "food_id", req.ID)
		lib.WriteError(w, http.StatusNotFound, "food not found")
		return
	}

	log.InfoContext(r.Context(), "food updated", "food_id", food.ID)
	lib.WriteJSON(w, http.StatusOK, food)
}

func (h *NutritionHandler) DeleteFood(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated food delete attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")

	if err := h.nutritionSvc.DeleteFood(r.Context(), uid, id); err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "food validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to delete food", "food_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to delete food")
		return
	}

	log.InfoContext(r.Context(), "food deleted", "food_id", id)
	lib.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "food deleted successfully",
	})
}
