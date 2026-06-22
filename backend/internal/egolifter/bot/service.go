package bot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/Bughay/egolifter/pkg/agent/agent"
	"github.com/Bughay/egolifter/pkg/agent/prompts"
	egotools "github.com/Bughay/egolifter/pkg/agent/tools/egolifter"
)

// The EgoLifter bot runs a ReAct agent over the egolifter tools through the
// provider-neutral pkg/agent/agent layer, so the backend can be swapped without
// touching this module. These fix the run to Grok with the egolifter prompt.
const (
	egolifterModel    = "grok-4.3"
	egolifterTokens   = 100000
	egolifterThinking = false
)

// chatStore is the slice of the repository the service needs: it lets tests
// supply a fake instead of a real Postgres-backed Repository. *Repository
// satisfies it.
type chatStore interface {
	CreateChat(ctx context.Context, userID, title string) (db.EgolifterChat, error)
	GetChat(ctx context.Context, chatID int64, userID string) (db.EgolifterChat, error)
	AddMessage(ctx context.Context, chatID int64, role, content string) (db.EgolifterMessage, error)
	ListChats(ctx context.Context, userID string) ([]db.EgolifterChat, error)
	ListMessages(ctx context.Context, chatID int64) ([]db.EgolifterMessage, error)
	DeleteChat(ctx context.Context, chatID int64, userID string) error
}

// Service holds the EgoLifter bot business logic: it persists the user turn,
// runs the ReAct agent over the egolifter tools, and persists the assistant
// reply.
//
// runAgent is the seam to the agent (pkg/agent/agent). It is a field so tests
// can stub it without hitting the network; NewService wires the real one.
type Service struct {
	repo     chatStore
	runAgent func(ctx context.Context, userID string, memory []agent.Message, prompt string) (string, error)
	logger   *slog.Logger
}

// NewService wires the bot service with its repository, the domain services the
// agent's tools act on, and a logger. The provider API key is read from the
// environment by pkg/agent, so it is not passed here.
func NewService(repo chatStore, agentSvc egotools.Services, logger *slog.Logger) *Service {
	return &Service{
		repo: repo,
		runAgent: func(ctx context.Context, userID string, memory []agent.Message, prompt string) (string, error) {
			a, err := agent.NewAgent(agent.Grok, agent.AgentParameters{
				Model:        egolifterModel,
				SystemPrompt: prompts.EgolifterAgentPrompt,
				UserPrompt:   prompt,
				Memory:       memory,
				Thinking:     egolifterThinking,
				Registry:     egotools.EgolifterFunctions(ctx, agentSvc, userID),
				SchemaData:   egotools.SchemaJSON,
				MaxTokens:    egolifterTokens,
			})
			if err != nil {
				return "", err
			}
			return a.Run(ctx)
		},
		logger: logger,
	}
}

// Chat runs one turn with the bot:
//   - chatID 0 starts a new chat (titled from the message); otherwise the chat
//     must belong to the user (404 if not).
//   - the user message is persisted, the agent is run, and the assistant reply
//     is persisted.
//
// It returns the (possibly newly created) chat id and the assistant reply. The
// agent acts on behalf of userID alone — its tools capture that id, so it can
// only ever touch the caller's own fitness data.
func (s *Service) Chat(ctx context.Context, userID string, req ChatRequest) (ChatResponse, error) {
	chatID := req.ChatID

	// memory carries an existing chat's prior turns so the agent can ask a
	// question on one turn and use the user's reply on the next. A new chat has
	// none. Load it before persisting the current message, since the agent
	// appends that message itself and must not see it twice.
	var memory []agent.Message
	if chatID == 0 {
		chat, err := s.repo.CreateChat(ctx, userID, makeTitle(req.Message))
		if err != nil {
			return ChatResponse{}, err
		}
		chatID = chat.ID
	} else {
		if _, err := s.repo.GetChat(ctx, chatID, userID); err != nil {
			return ChatResponse{}, err // ErrChatNotFound or a real error
		}
		msgs, err := s.repo.ListMessages(ctx, chatID)
		if err != nil {
			return ChatResponse{}, err
		}
		memory = make([]agent.Message, len(msgs))
		for i, m := range msgs {
			memory[i] = agent.Message{Role: m.Role, Content: m.Content}
		}
	}

	if _, err := s.repo.AddMessage(ctx, chatID, "user", req.Message); err != nil {
		return ChatResponse{}, err
	}

	reply, err := s.runAgent(ctx, userID, memory, req.Message)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("botSvc.Chat: run agent: %w", err)
	}

	if _, err := s.repo.AddMessage(ctx, chatID, "assistant", reply); err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{ChatID: chatID, Content: reply}, nil
}

// ListChats returns the user's chats as response DTOs (no user_id), newest
// activity first. It always returns a non-nil slice so the JSON is [] for a user
// with no chats rather than null.
func (s *Service) ListChats(ctx context.Context, userID string) ([]ChatSummary, error) {
	chats, err := s.repo.ListChats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("botSvc.ListChats: %w", err)
	}
	out := make([]ChatSummary, 0, len(chats))
	for _, c := range chats {
		out = append(out, toChatSummary(c))
	}
	return out, nil
}

// ListMessages returns a chat's messages (oldest first) if the chat belongs to
// the user, else ErrChatNotFound. It always returns a non-nil slice so the JSON
// is [] for an empty chat rather than null.
func (s *Service) ListMessages(ctx context.Context, userID string, chatID int64) ([]MessageView, error) {
	if _, err := s.repo.GetChat(ctx, chatID, userID); err != nil {
		return nil, err // ErrChatNotFound or a real error
	}
	msgs, err := s.repo.ListMessages(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("botSvc.ListMessages: %w", err)
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
