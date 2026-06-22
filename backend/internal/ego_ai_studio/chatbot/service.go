package aistudio

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Bughay/egolifter/pkg/agent/agent"
	"github.com/Bughay/egolifter/pkg/agent/prompts"
)

// defaultMaxTokens is used when the request omits a positive value. defaultModel is
// the model used when the request omits one (or sends an unknown one).
const (
	defaultMaxTokens = 2048
	titleMaxLen      = 60
	defaultModel     = "deepseek-pro"
)

// modelInfo binds a selectable UI model to the agent provider, the LLM call mode
// (plain chat vs. web search), and the real API id sent on the wire.
type modelInfo struct {
	provider agent.Provider
	mode     agent.LLMMode
	apiID    string
}

// modelRegistry is the allow-list of selectable models: each UI value maps to a
// provider + mode + real API id. Only these are accepted; resolveModel falls back
// to the default (DeepSeek pro) for anything else.
var modelRegistry = map[string]modelInfo{
	"deepseek-pro":   {agent.DeepSeek, agent.ModeChat, "deepseek-v4-pro"},
	"deepseek-flash": {agent.DeepSeek, agent.ModeChat, "deepseek-v4-flash"},
	"grok":           {agent.Grok, agent.ModeChat, "grok-4.3"},
	"grok-websearch": {agent.Grok, agent.ModeWebSearch, "grok-4.3"},
}

// resolveModel maps a UI model value to its provider + mode + API id, defaulting
// to DeepSeek pro when the value is empty or not one of the allowed models.
func resolveModel(uiModel string) modelInfo {
	if info, ok := modelRegistry[uiModel]; ok {
		return info
	}
	return modelRegistry[defaultModel]
}

// Service holds the AI Studio chat business logic: it loads a chat's history from
// the DB (the model's memory), runs the selected model through the unified LLM
// layer (pkg/agent/agent), and persists the new user + assistant turns through
// the Repository.
type Service struct {
	repo   *Repository
	logger *slog.Logger
}

// NewService wires the AI Studio service with its repository and a logger. The
// provider API keys are read from the environment by pkg/agent, so apiKey is
// accepted here for wiring symmetry with the other modules but is not stored.
func NewService(repo *Repository, apiKey string, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// Chat runs one conversational turn with memory:
//   - chatID 0 starts a new chat (titled from the first user message); otherwise
//     the chat's prior messages are loaded as memory (404 if not the user's).
//   - the user message is persisted, the selected model is called with system +
//     history + user, and the assistant reply is persisted.
//
// It returns the (possibly newly created) chat id and the assistant reply.
func (s *Service) Chat(ctx context.Context, userID string, req ChatRequest) (ChatResponse, error) {
	chatID := req.ChatID

	// Build the conversation memory: system prompt first, then prior turns.
	systemPrompt := req.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = prompts.BackendAssistant
	}
	msgs := []agent.Message{{Role: "system", Content: systemPrompt}}

	if chatID == 0 {
		chat, err := s.repo.CreateChat(ctx, userID, makeTitle(req.UserPrompt))
		if err != nil {
			return ChatResponse{}, err
		}
		chatID = chat.ID
	} else {
		if _, err := s.repo.GetChat(ctx, chatID, userID); err != nil {
			return ChatResponse{}, err // ErrChatNotFound or a real error
		}
		history, err := s.repo.ListMessages(ctx, chatID)
		if err != nil {
			return ChatResponse{}, err
		}
		for _, m := range history {
			msgs = append(msgs, agent.Message{Role: m.Role, Content: m.Content})
		}
	}

	// Append + persist the new user message.
	msgs = append(msgs, agent.Message{Role: "user", Content: req.UserPrompt})
	if _, err := s.repo.AddMessage(ctx, chatID, "user", req.UserPrompt); err != nil {
		return ChatResponse{}, err
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	info := resolveModel(req.Model)

	llm, err := agent.NewLLM(info.provider, agent.LLMParameters{
		Model:       info.apiID,
		Memory:      msgs, // full system + history + user slice, used verbatim
		Temperature: req.Temperature,
		MaxTokens:   maxTokens,
		Thinking:    req.Thinking, // honored by deepseek; grok ignores it (see llm.go)
		Mode:        info.mode,
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("chatSvc.Chat: build llm: %w", err)
	}
	reply, err := llm.Complete(ctx)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("chatSvc.Chat: %s: %w", info.provider, err)
	}

	if _, err := s.repo.AddMessage(ctx, chatID, "assistant", reply); err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{ChatID: chatID, Content: reply}, nil
}

// ListChats returns the user's chats as response DTOs (no user_id, plain title),
// newest activity first. It always returns a non-nil slice so the JSON is [] for
// a user with no chats rather than null.
func (s *Service) ListChats(ctx context.Context, userID string) ([]ChatSummary, error) {
	chats, err := s.repo.ListChats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("chatSvc.ListChats: %w", err)
	}
	out := make([]ChatSummary, 0, len(chats))
	for _, c := range chats {
		out = append(out, toChatSummary(c))
	}
	return out, nil
}

// ListMessages returns a chat's messages (oldest first) if the chat belongs to the
// user, else ErrChatNotFound. It always returns a non-nil slice so the JSON is []
// for an empty chat rather than null.
func (s *Service) ListMessages(ctx context.Context, userID string, chatID int64) ([]MessageView, error) {
	if _, err := s.repo.GetChat(ctx, chatID, userID); err != nil {
		return nil, err // ErrChatNotFound or a real error
	}
	msgs, err := s.repo.ListMessages(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("chatSvc.ListMessages: %w", err)
	}
	out := make([]MessageView, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageView(m))
	}
	return out, nil
}

// DeleteChat removes one of the user's chats and all of its messages. Returns
// ErrChatNotFound if the chat does not exist or is not the user's.
func (s *Service) DeleteChat(ctx context.Context, userID string, chatID int64) error {
	return s.repo.DeleteChat(ctx, chatID, userID)
}

// makeTitle derives a short chat title from the first user message.
func makeTitle(s string) string {
	r := []rune(s)
	if len(r) <= titleMaxLen {
		return s
	}
	return string(r[:titleMaxLen]) + "…"
}
