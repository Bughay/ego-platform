package aistudio

import (
	"time"

	"github.com/Bughay/egolifter/internal/shared/db"
)

// ChatRequest is the payload the frontend chat UI (frontend/ego_ai_studio) POSTs
// to /ai/chat. ChatID is 0 (or omitted) to start a new conversation; otherwise it
// continues that chat, whose prior messages are the model's memory.
type ChatRequest struct {
	ChatID       int64   `json:"chatId"`
	SystemPrompt string  `json:"systemPrompt"`
	UserPrompt   string  `json:"userPrompt"`
	Model        string  `json:"model"` // "deepseek-pro" | "deepseek-flash"
	Thinking     bool    `json:"thinking"` // DeepSeek reasoning mode; omitted/false = off
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"maxTokens"`
}

// ChatResponse is what /ai/chat returns. ChatID is echoed back (and assigned when
// a new chat was created) so the frontend can keep sending to the same chat.
type ChatResponse struct {
	ChatID  int64  `json:"chatId"`
	Content string `json:"content"`
}

// ChatSummary is one chat in the GET /ai/chats list. Title is nil when the chat
// has no title (pgtype.Text invalid).
type ChatSummary struct {
	ChatID    int64     `json:"chatId"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// toChatSummary maps a sqlc db.AiStudioChat row to the API DTO: it drops user_id
// and flattens the pgtype.Text title to a plain string pointer (nil when unset).
func toChatSummary(c db.AiStudioChat) ChatSummary {
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

// MessageView is one message in the GET /ai/chats/{id}/messages list (the full
// conversation the frontend renders when a chat is reopened).
type MessageView struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// toMessageView maps a sqlc db.AiStudioMessage row to the API DTO.
func toMessageView(m db.AiStudioMessage) MessageView {
	return MessageView{Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt}
}
