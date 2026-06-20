package profile

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// ProfileHandler handles profile-related HTTP requests.
type ProfileHandler struct {
	profileSvc ProfileService
	logger     *slog.Logger
}

// NewProfileHandler creates a new ProfileHandler.
func NewProfileHandler(profileSvc ProfileService, logger *slog.Logger) *ProfileHandler {
	return &ProfileHandler{profileSvc: profileSvc, logger: logger}
}

// RegisterRoutes attaches the profile endpoints to the given mux,
// wrapping every route with the provided middleware (JWT auth).
func (h *ProfileHandler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("GET /profile", mw(http.HandlerFunc(h.GetProfile)))
	mux.Handle("PUT /profile", mw(http.HandlerFunc(h.UpdateProfile)))
}

// userID extracts the authenticated user's ID from the JWT claims in the context.
func userID(r *http.Request) (string, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

// GetProfile returns the authenticated user's profile (empty fields if never saved).
func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated profile view attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	p, err := h.profileSvc.GetProfile(r.Context(), uid)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to get profile", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}

	lib.WriteJSON(w, http.StatusOK, p)
}

// UpdateProfile creates or updates the authenticated user's profile.
func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated profile update attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	p, err := h.profileSvc.UpdateProfile(r.Context(), uid, &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "profile validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "failed to update profile", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	log.InfoContext(r.Context(), "profile updated")
	lib.WriteJSON(w, http.StatusOK, p)
}

// isValidationErr checks if the error originated from a validation rule.
func isValidationErr(err error) bool {
	return strings.HasPrefix(err.Error(), "validation:")
}
