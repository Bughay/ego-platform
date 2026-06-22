// Package bot is the HTTP layer for the EgoLifter AI assistant. It follows the
// same handler -> service -> repository pattern as the fitness domains (see
// internal/egolifter/nutrition for a worked example) and is wrapped with the
// shared JWT middleware, so it uses the single platform auth.
//
// A chat turn runs the ReAct agent (pkg/agent/agent) over the egolifter tools
// (log a meal, log a workout, summarize a date range) and persists the
// conversation in egolifter_chats / egolifter_messages.
package bot

import (
	"errors"
	"time"

	"github.com/Bughay/egolifter/internal/shared/db"
)

// ErrChatNotFound is returned when a chat does not exist or does not belong to
// the requesting user. The handler maps it to a 404.
var ErrChatNotFound = errors.New("chat not found")

// titleMaxLen caps the length of the auto-derived chat title.
const titleMaxLen = 60

// ChatRequest is the payload POSTed to /egolifter/chat. ChatID is 0 (or omitted)
// to start a new conversation; otherwise it appends to that chat (which must
// belong to the caller).
type ChatRequest struct {
	ChatID  int64  `json:"chatId"`
	Message string `json:"message"`
}

// ChatResponse is what /egolifter/chat returns. ChatID is echoed back (and
// assigned when a new chat was created) so the caller can keep talking to the
// same chat.
type ChatResponse struct {
	ChatID  int64  `json:"chatId"`
	Content string `json:"content"`
}

// ChatSummary is one chat in the GET /egolifter/chats list. Title is nil when the
// chat has no title (pgtype.Text invalid).
type ChatSummary struct {
	ChatID    int64     `json:"chatId"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// toChatSummary maps a sqlc db.EgolifterChat row to the API DTO: it drops user_id
// and flattens the pgtype.Text title to a plain string pointer (nil when unset).
func toChatSummary(c db.EgolifterChat) ChatSummary {
	var title *string
	if c.Title.Valid {
		title = &c.Title.String
	}
	return ChatSummary{
		ChatID:    c.ID,
		Title:     title,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// MessageView is one message in the GET /egolifter/chats/{id}/messages list (the
// full conversation the frontend renders when a chat is reopened).
type MessageView struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// toMessageView maps a sqlc db.EgolifterMessage row to the API DTO.
func toMessageView(m db.EgolifterMessage) MessageView {
	return MessageView{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
}

// makeTitle derives a short chat title from the first user message.
func makeTitle(s string) string {
	r := []rune(s)
	if len(r) <= titleMaxLen {
		return s
	}
	return string(r[:titleMaxLen]) + "…"
}
