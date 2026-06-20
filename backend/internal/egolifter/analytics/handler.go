package analytics

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// AnalyticsHandler handles analytics-related HTTP requests.
type AnalyticsHandler struct {
	analyticsSvc AnalyticsService
	logger       *slog.Logger
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(analyticsSvc AnalyticsService, logger *slog.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{analyticsSvc: analyticsSvc, logger: logger}
}

// RegisterRoutes attaches the analytics endpoints to the given mux,
// wrapping every route with the provided middleware (JWT auth).
func (h *AnalyticsHandler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("GET /analytics/summary", mw(http.HandlerFunc(h.GetSummary)))
}

// userID extracts the authenticated user's ID from the JWT claims in the context.
func userID(r *http.Request) (string, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

// GetSummary returns a combined nutrition + training summary for the range
// given by ?date_from= and ?date_to= (YYYY-MM-DD, inclusive); either bound
// defaults to today.
func (h *AnalyticsHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		log.WarnContext(r.Context(), "unauthenticated analytics summary attempt")
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")

	summary, err := h.analyticsSvc.GetSummary(r.Context(), uid, dateFrom, dateTo)
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
		log.ErrorContext(r.Context(), "failed to build analytics summary", "date_from", dateFrom, "date_to", dateTo, "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to build analytics summary")
		return
	}

	log.InfoContext(r.Context(), "analytics summary built", "date_from", summary.DateFrom, "date_to", summary.DateTo, "days", summary.Days)
	lib.WriteJSON(w, http.StatusOK, summary)
}

// isValidationErr checks if the error originated from a validation rule.
func isValidationErr(err error) bool {
	return strings.HasPrefix(err.Error(), "validation:")
}
