package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Bughay/egolifter/internal/shared/lib"
)

// AuthHandler handles auth-related HTTP requests.
type AuthHandler struct {
	authSvc AuthService
	logger  *slog.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authSvc AuthService, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, logger: logger}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, err := h.authSvc.Register(r.Context(), &req)
	if err != nil {
		if isValidationErr(err) {
			log.WarnContext(r.Context(), "registration validation failed", "error", err)
			lib.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.ErrorContext(r.Context(), "registration failed", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	log.InfoContext(r.Context(), "user registered", "email", req.Email)
	lib.WriteJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	resp, refreshToken, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		log.WarnContext(r.Context(), "login failed", "email", req.Email, "error", err)
		lib.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	log.InfoContext(r.Context(), "user logged in", "email", req.Email)
	lib.WriteJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		_ = h.authSvc.Logout(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	log.InfoContext(r.Context(), "user logged out")
	lib.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "logged out successfully",
	})
}

// Refresh handles token rotation
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		log.WarnContext(r.Context(), "refresh token missing")
		lib.WriteError(w, http.StatusUnauthorized, "refresh token required")
		return
	}

	resp, newRefreshToken, err := h.authSvc.Refresh(r.Context(), cookie.Value)
	if err != nil {
		log.WarnContext(r.Context(), "token refresh failed", "error", err)
		lib.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    newRefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	log.InfoContext(r.Context(), "token refreshed")
	lib.WriteJSON(w, http.StatusOK, resp)
}

// isValidationErr checks if the error originated from a validation rule.
func isValidationErr(err error) bool {
	return strings.HasPrefix(err.Error(), "validation:")
}
