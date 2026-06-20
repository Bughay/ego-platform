// Package aistudio is the HTTP layer for the Ego AI Studio app. It follows the
// same handler -> service -> repository pattern as the fitness domains (see
// internal/egolifter/nutrition for a worked example) and every route is wrapped
// with the shared JWT middleware, so it uses the single platform auth.
package aistudio

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Bughay/egolifter/internal/auth"
	"github.com/Bughay/egolifter/internal/shared/lib"
)

// Handler is the HTTP entry point for the AI Studio app.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler creates the AI Studio HTTP handler.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes attaches the AI Studio endpoints to the mux, each wrapped with
// the shared JWT middleware (mw).
func (h *Handler) RegisterRoutes(mux *http.ServeMux, mw func(http.Handler) http.Handler) {
	mux.Handle("POST /ai/chat", mw(http.HandlerFunc(h.Chat)))
	mux.Handle("GET /ai/chats", mw(http.HandlerFunc(h.ListChats)))
	mux.Handle("GET /ai/chats/{id}/messages", mw(http.HandlerFunc(h.ListMessages)))
	mux.Handle("DELETE /ai/chats/{id}", mw(http.HandlerFunc(h.DeleteChat)))
}

// userID extracts the authenticated user's id from the JWT claims in context.
func userID(r *http.Request) (string, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
}

// Chat handles POST /ai/chat: one conversational turn with DB-backed memory.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WarnContext(r.Context(), "invalid request body", "error", err)
		lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.UserPrompt == "" {
		lib.WriteError(w, http.StatusBadRequest, "userPrompt is required")
		return
	}

	resp, err := h.svc.Chat(r.Context(), uid, req)
	if err != nil {
		if errors.Is(err, ErrChatNotFound) {
			lib.WriteError(w, http.StatusNotFound, "chat not found")
			return
		}
		log.ErrorContext(r.Context(), "chat failed", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to generate reply")
		return
	}

	lib.WriteJSON(w, http.StatusOK, resp)
}

// ListChats handles GET /ai/chats: all of the authenticated user's chats, newest
// activity first.
func (h *Handler) ListChats(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	chats, err := h.svc.ListChats(r.Context(), uid)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to list chats", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to list chats")
		return
	}

	lib.WriteJSON(w, http.StatusOK, chats)
}

// ListMessages handles GET /ai/chats/{id}/messages: the full conversation of one of
// the authenticated user's chats, oldest message first.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	chatID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		lib.WriteError(w, http.StatusBadRequest, "invalid chat id")
		return
	}

	msgs, err := h.svc.ListMessages(r.Context(), uid, chatID)
	if err != nil {
		if errors.Is(err, ErrChatNotFound) {
			lib.WriteError(w, http.StatusNotFound, "chat not found")
			return
		}
		log.ErrorContext(r.Context(), "failed to list messages", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}

	lib.WriteJSON(w, http.StatusOK, msgs)
}

// DeleteChat handles DELETE /ai/chats/{id}: removes one of the user's chats (and its
// messages, via the ON DELETE CASCADE foreign key).
func (h *Handler) DeleteChat(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("method", r.Method, "path", r.URL.Path)

	uid, ok := userID(r)
	if !ok {
		lib.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	log = log.With("user_id", uid)

	chatID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		lib.WriteError(w, http.StatusBadRequest, "invalid chat id")
		return
	}

	if err := h.svc.DeleteChat(r.Context(), uid, chatID); err != nil {
		if errors.Is(err, ErrChatNotFound) {
			lib.WriteError(w, http.StatusNotFound, "chat not found")
			return
		}
		log.ErrorContext(r.Context(), "failed to delete chat", "error", err)
		lib.WriteError(w, http.StatusInternalServerError, "failed to delete chat")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
