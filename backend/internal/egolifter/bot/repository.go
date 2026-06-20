package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists EgoLifter bot chats and messages in the shared Postgres
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
func (r *Repository) CreateChat(ctx context.Context, userID, title string) (db.EgolifterChat, error) {
	chat, err := r.queries.CreateEgolifterChat(ctx, db.CreateEgolifterChatParams{
		UserID: userID,
		Title:  pgtype.Text{String: title, Valid: title != ""},
	})
	if err != nil {
		return db.EgolifterChat{}, fmt.Errorf("botRepo.CreateChat: %w", err)
	}
	return chat, nil
}

// GetChat returns the chat if it exists and belongs to userID, else ErrChatNotFound.
func (r *Repository) GetChat(ctx context.Context, chatID int64, userID string) (db.EgolifterChat, error) {
	chat, err := r.queries.GetEgolifterChatForUser(ctx, db.GetEgolifterChatForUserParams{ID: chatID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.EgolifterChat{}, ErrChatNotFound
		}
		return db.EgolifterChat{}, fmt.Errorf("botRepo.GetChat: %w", err)
	}
	return chat, nil
}

// ListChats returns all of the user's chats, newest activity first.
func (r *Repository) ListChats(ctx context.Context, userID string) ([]db.EgolifterChat, error) {
	chats, err := r.queries.GetAllEgolifterChatsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("botRepo.ListChats: %w", err)
	}
	return chats, nil
}

// ListMessages returns all messages in a chat, oldest first.
func (r *Repository) ListMessages(ctx context.Context, chatID int64) ([]db.EgolifterMessage, error) {
	msgs, err := r.queries.ListEgolifterMessagesByChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("botRepo.ListMessages: %w", err)
	}
	return msgs, nil
}

// DeleteChat removes the user's chat (its messages cascade-delete via the FK).
// Returns ErrChatNotFound when no chat with that id belongs to the user.
func (r *Repository) DeleteChat(ctx context.Context, chatID int64, userID string) error {
	rows, err := r.queries.DeleteEgolifterChatForUser(ctx, db.DeleteEgolifterChatForUserParams{ID: chatID, UserID: userID})
	if err != nil {
		return fmt.Errorf("botRepo.DeleteChat: %w", err)
	}
	if rows == 0 {
		return ErrChatNotFound
	}
	return nil
}

// AddMessage appends a message to a chat and bumps the chat's updated_at.
func (r *Repository) AddMessage(ctx context.Context, chatID int64, role, content string) (db.EgolifterMessage, error) {
	msg, err := r.queries.CreateEgolifterMessage(ctx, db.CreateEgolifterMessageParams{
		ChatID:  chatID,
		Role:    role,
		Content: content,
	})
	if err != nil {
		return db.EgolifterMessage{}, fmt.Errorf("botRepo.AddMessage: %w", err)
	}
	if err := r.queries.TouchEgolifterChat(ctx, chatID); err != nil {
		return db.EgolifterMessage{}, fmt.Errorf("botRepo.AddMessage: touch: %w", err)
	}
	return msg, nil
}
