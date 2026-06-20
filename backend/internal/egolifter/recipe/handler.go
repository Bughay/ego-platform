package recipe

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// RecipeHandler handles recipe-related HTTP requests.
type RecipeHandler struct {
	recipeSvc RecipeService
	logger    *slog.Logger
}

// NewRecipeHandler creates a new RecipeHandler.
func NewRecipeHandler(recipeSvc RecipeService, logger *slog.Logger) *RecipeHandler {
	return &RecipeHandler{recipeSvc: recipeSvc, logger: logger}
}

// RegisterRoutes attaches the recipe CRUD endpoints to the given mux,
// wrapping every route with the provided middleware (JWT auth).
func (h *RecipeHandler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("POST /recipe/create", mw(http.HandlerFunc(h.CreateRecipe)))
	mux.Handle("GET /recipe/view", mw(http.HandlerFunc(h.ViewRecipe)))
	mux.Handle("PUT /recipe/update", mw(http.HandlerFunc(h.UpdateRecipe)))
	mux.Handle("DELETE /recipe/del", mw(http.HandlerFunc(h.DeleteRecipe)))
	mux.Handle("GET /recipe/getfoods", mw(http.HandlerFunc(h.GetRecipeFoods)))
}

// userID extracts the authenticated user's ID from the JWT claims in the context.
func userID(r *http.Request) (string, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

func (h *RecipeHandler) CreateRecipe(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated recipe create attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req CreateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	recipe, err := h.recipeSvc.CreateRecipe(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "recipe validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to create recipe", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to create recipe")
		return
	}

	log.InfoContext(r.Context(), "recipe created", "recipe_id", recipe.ID)
	lib.WriteJSON(w, http.StatusCreated, recipe)
}

// ViewRecipe returns a single recipe (with ingredients) when ?id= is given,
// otherwise lists all of the user's recipes.
func (h *RecipeHandler) ViewRecipe(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated recipe view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")
	if id == "" {
		recipes, err := h.recipeSvc.ListRecipes(r.Context(), uid)
		if err != nil {
			log.ErrorContext(r.Context(), "failed to list recipes", "error", err)
			lib.WriteError(w, http.StatusInternalServerError, "failed to list recipes")
			return
		}
		log.InfoContext(r.Context(), "recipes listed", "count", len(recipes))
		lib.WriteJSON(w, http.StatusOK, recipes)
		return
	}

	recipe, err := h.recipeSvc.GetRecipe(r.Context(), uid, id)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to get recipe", "recipe_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to get recipe")
		return
	}
	if recipe == nil {
		log.WarnContext(r.Context(), "recipe not found", "recipe_id", id)
		lib.WriteError(w, http.StatusNotFound, "recipe not found")
		return
	}

	log.InfoContext(r.Context(), "recipe retrieved", "recipe_id", id)
	lib.WriteJSON(w, http.StatusOK, recipe)
}

func (h *RecipeHandler) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated recipe update attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req UpdateRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	recipe, err := h.recipeSvc.UpdateRecipe(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "recipe validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to update recipe", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to update recipe")
		return
	}
	if recipe == nil {
		log.WarnContext(r.Context(), "recipe not found for update", "recipe_id", req.ID)
		lib.WriteError(w, http.StatusNotFound, "recipe not found")
		return
	}

	log.InfoContext(r.Context(), "recipe updated", "recipe_id", recipe.ID)
	lib.WriteJSON(w, http.StatusOK, recipe)
}

func (h *RecipeHandler) DeleteRecipe(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated recipe delete attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")
	found, err := h.recipeSvc.DeleteRecipe(r.Context(), uid, id)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "recipe validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to delete recipe", "recipe_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to delete recipe")
		return
	}
	if !found {
		log.WarnContext(r.Context(), "recipe not found", "recipe_id", id)
		lib.WriteError(w, http.StatusNotFound, "recipe not found")
		return
	}

	log.InfoContext(r.Context(), "recipe deleted", "recipe_id", id)
	lib.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "recipe deleted successfully",
	})
}

// GetRecipeFoods returns the foods of the recipe given by ?id= with their
// macros adjusted for each ingredient's weight.
func (h *RecipeHandler) GetRecipeFoods(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated recipe foods view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	id := r.URL.Query().Get("id")
	foods, err := h.recipeSvc.GetRecipeFoods(r.Context(), uid, id)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "recipe validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to get recipe foods", "recipe_id", id, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to get recipe foods")
		return
	}
	if foods == nil {
		log.WarnContext(r.Context(), "recipe not found", "recipe_id", id)
		lib.WriteError(w, http.StatusNotFound, "recipe not found")
		return
	}

	log.InfoContext(r.Context(), "recipe foods retrieved", "recipe_id", id, "count", len(foods))
	lib.WriteJSON(w, http.StatusOK, foods)
}

// isValidationErr checks if the error originated from a validation rule.
func isValidationErr(err error) bool {
	return strings.HasPrefix(err.Error(), "validation:")
}
