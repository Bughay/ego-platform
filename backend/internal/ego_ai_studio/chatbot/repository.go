package aistudio

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrChatNotFound is returned when a chat does not exist or does not belong to
// the requesting user. The service maps it to a 404.
var ErrChatNotFound = errors.New("chat not found")

// Repository persists AI Studio chats and messages in the shared Postgres
// database, wrapping the sqlc-generated queries (internal/shared/db). Every read
// is scoped by userID so users only ever touch their own conversations — the
// same rule the fitness modules follow.
type Repository struct {
	queries *db.Queries
}

// NewRepository creates a Repository backed by the single shared pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: db.New(pool)}
}

// CreateChat starts a new conversation for the user with an optional title.
func (r *Repository) CreateChat(ctx context.Context, userID, title string) (db.AiStudioChat, error) {
	chat, err := r.queries.CreateChat(ctx, db.CreateChatParams{
		UserID: userID,
		Title:  pgtype.Text{String: title, Valid: title != ""},
	})
	if err != nil {
		return db.AiStudioChat{}, fmt.Errorf("chatRepo.CreateChat: %w", err)
	}
	return chat, nil
}

// GetChat returns the chat if it exists and belongs to userID, else ErrChatNotFound.
func (r *Repository) GetChat(ctx context.Context, chatID int64, userID string) (db.AiStudioChat, error) {
	chat, err := r.queries.GetChatForUser(ctx, db.GetChatForUserParams{ID: chatID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AiStudioChat{}, ErrChatNotFound
		}
		return db.AiStudioChat{}, fmt.Errorf("chatRepo.GetChat: %w", err)
	}
	return chat, nil
}

// ListChats returns all of the user's chats, newest activity first.
func (r *Repository) ListChats(ctx context.Context, userID string) ([]db.AiStudioChat, error) {
	chats, err := r.queries.GetAllChatsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("chatRepo.ListChats: %w", err)
	}
	return chats, nil
}

// DeleteChat removes the user's chat (its messages cascade-delete via the FK).
// Returns ErrChatNotFound when no chat with that id belongs to the user.
func (r *Repository) DeleteChat(ctx context.Context, chatID int64, userID string) error {
	rows, err := r.queries.DeleteChatForUser(ctx, db.DeleteChatForUserParams{ID: chatID, UserID: userID})
	if err != nil {
		return fmt.Errorf("chatRepo.DeleteChat: %w", err)
	}
	if rows == 0 {
		return ErrChatNotFound
	}
	return nil
}

// ListMessages returns all messages in a chat, oldest first (the conversation
// memory handed to DeepSeek).
func (r *Repository) ListMessages(ctx context.Context, chatID int64) ([]db.AiStudioMessage, error) {
	msgs, err := r.queries.ListMessagesByChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("chatRepo.ListMessages: %w", err)
	}
	return msgs, nil
}

// AddMessage appends a message to a chat and bumps the chat's updated_at.
func (r *Repository) AddMessage(ctx context.Context, chatID int64, role, content string) (db.AiStudioMessage, error) {
	msg, err := r.queries.CreateMessage(ctx, db.CreateMessageParams{
		ChatID:  chatID,
		Role:    role,
		Content: content,
	})
	if err != nil {
		return db.AiStudioMessage{}, fmt.Errorf("chatRepo.AddMessage: %w", err)
	}
	if err := r.queries.TouchChat(ctx, chatID); err != nil {
		return db.AiStudioMessage{}, fmt.Errorf("chatRepo.AddMessage: touch: %w", err)
	}
	return msg, nil
}
