package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/Bughay/egolifter/internal/shared/db"
	"github.com/Bughay/egolifter/pkg/agent/deepseek"
	"github.com/jackc/pgx/v5/pgtype"
)

type addedMsg struct {
	chatID  int64
	role    string
	content string
}

// fakeStore is an in-memory chatStore for the service tests: no Postgres needed.
type fakeStore struct {
	createdID   int64
	createErr   error
	getErr      error
	createCalls int
	getCalls    int
	added       []addedMsg

	chats       []db.EgolifterChat
	chatsErr    error
	messages    []db.EgolifterMessage
	messagesErr error
	deleteErr   error
	deleteCalls int
}

func (f *fakeStore) CreateChat(_ context.Context, userID, _ string) (db.EgolifterChat, error) {
	f.createCalls++
	if f.createErr != nil {
		return db.EgolifterChat{}, f.createErr
	}
	return db.EgolifterChat{ID: f.createdID, UserID: userID}, nil
}

func (f *fakeStore) GetChat(_ context.Context, chatID int64, userID string) (db.EgolifterChat, error) {
	f.getCalls++
	if f.getErr != nil {
		return db.EgolifterChat{}, f.getErr
	}
	return db.EgolifterChat{ID: chatID, UserID: userID}, nil
}

func (f *fakeStore) AddMessage(_ context.Context, chatID int64, role, content string) (db.EgolifterMessage, error) {
	f.added = append(f.added, addedMsg{chatID, role, content})
	return db.EgolifterMessage{ChatID: chatID, Role: role, Content: content}, nil
}

func (f *fakeStore) ListChats(_ context.Context, _ string) ([]db.EgolifterChat, error) {
	return f.chats, f.chatsErr
}

func (f *fakeStore) ListMessages(_ context.Context, _ int64) ([]db.EgolifterMessage, error) {
	return f.messages, f.messagesErr
}

func (f *fakeStore) DeleteChat(_ context.Context, _ int64, _ string) error {
	f.deleteCalls++
	return f.deleteErr
}

func newTestService(store chatStore, reply string, agentErr error) *Service {
	return &Service{
		repo: store,
		runAgent: func(_ context.Context, _ string, _ []deepseek.Message, _ string) (string, error) {
			return reply, agentErr
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestServiceChat(t *testing.T) {
	errAgent := errors.New("agent boom")

	tests := []struct {
		name           string
		req            ChatRequest
		store          *fakeStore
		reply          string
		agentErr       error
		wantErr        error // sentinel to match with errors.Is; nil means no error
		wantErrNonNil  bool  // for non-sentinel errors
		wantChatID     int64
		wantContent    string
		wantCreate     int
		wantGet        int
		wantAddedRoles []string
	}{
		{
			name:           "new chat is created and both turns persisted",
			req:            ChatRequest{ChatID: 0, Message: "I ate 2 eggs"},
			store:          &fakeStore{createdID: 7},
			reply:          "Logged your eggs!",
			wantChatID:     7,
			wantContent:    "Logged your eggs!",
			wantCreate:     1,
			wantGet:        0,
			wantAddedRoles: []string{"user", "assistant"},
		},
		{
			name:           "existing chat appends without creating",
			req:            ChatRequest{ChatID: 42, Message: "and a banana"},
			store:          &fakeStore{},
			reply:          "Added the banana.",
			wantChatID:     42,
			wantContent:    "Added the banana.",
			wantCreate:     0,
			wantGet:        1,
			wantAddedRoles: []string{"user", "assistant"},
		},
		{
			name:           "chat not owned by user returns ErrChatNotFound",
			req:            ChatRequest{ChatID: 99, Message: "hi"},
			store:          &fakeStore{getErr: ErrChatNotFound},
			wantErr:        ErrChatNotFound,
			wantGet:        1,
			wantAddedRoles: nil,
		},
		{
			name:           "agent failure surfaces and skips the assistant turn",
			req:            ChatRequest{ChatID: 0, Message: "log my run"},
			store:          &fakeStore{createdID: 3},
			agentErr:       errAgent,
			wantErrNonNil:  true,
			wantCreate:     1,
			wantAddedRoles: []string{"user"}, // user persisted, assistant not reached
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.store, tc.reply, tc.agentErr)

			resp, err := svc.Chat(context.Background(), "user-1", tc.req)

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
			case tc.wantErrNonNil:
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if resp.ChatID != tc.wantChatID {
					t.Errorf("ChatID = %d, want %d", resp.ChatID, tc.wantChatID)
				}
				if resp.Content != tc.wantContent {
					t.Errorf("Content = %q, want %q", resp.Content, tc.wantContent)
				}
			}

			if tc.store.createCalls != tc.wantCreate {
				t.Errorf("CreateChat calls = %d, want %d", tc.store.createCalls, tc.wantCreate)
			}
			if tc.store.getCalls != tc.wantGet {
				t.Errorf("GetChat calls = %d, want %d", tc.store.getCalls, tc.wantGet)
			}

			gotRoles := make([]string, len(tc.store.added))
			for i, m := range tc.store.added {
				gotRoles[i] = m.role
			}
			if len(gotRoles) != len(tc.wantAddedRoles) {
				t.Fatalf("persisted roles = %v, want %v", gotRoles, tc.wantAddedRoles)
			}
			for i := range gotRoles {
				if gotRoles[i] != tc.wantAddedRoles[i] {
					t.Errorf("persisted role[%d] = %q, want %q", i, gotRoles[i], tc.wantAddedRoles[i])
				}
			}
		})
	}
}

func TestServiceListChats(t *testing.T) {
	withTitle := "Leg day"

	tests := []struct {
		name           string
		store          *fakeStore
		wantErr        bool
		wantLen        int
		wantFirstTitle *string
	}{
		{
			name:    "no chats returns a non-nil empty slice",
			store:   &fakeStore{chats: nil},
			wantLen: 0,
		},
		{
			name: "maps rows and flattens the title",
			store: &fakeStore{chats: []db.EgolifterChat{
				{ID: 1, Title: pgtype.Text{String: withTitle, Valid: true}},
				{ID: 2}, // untitled -> nil title
			}},
			wantLen:        2,
			wantFirstTitle: &withTitle,
		},
		{
			name:    "repo error surfaces",
			store:   &fakeStore{chatsErr: errors.New("boom")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.store, "", nil)

			out, err := svc.ListChats(context.Background(), "user-1")

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out == nil {
				t.Fatalf("ListChats returned nil slice, want non-nil for [] JSON")
			}
			if len(out) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(out), tc.wantLen)
			}
			if tc.wantFirstTitle != nil {
				if out[0].Title == nil || *out[0].Title != *tc.wantFirstTitle {
					t.Errorf("first title = %v, want %q", out[0].Title, *tc.wantFirstTitle)
				}
				if out[1].Title != nil {
					t.Errorf("second title = %v, want nil", *out[1].Title)
				}
			}
		})
	}
}

func TestServiceListMessages(t *testing.T) {
	tests := []struct {
		name    string
		store   *fakeStore
		wantErr error // sentinel; nil means no error
		wantLen int
	}{
		{
			name:    "chat not owned by user returns ErrChatNotFound",
			store:   &fakeStore{getErr: ErrChatNotFound},
			wantErr: ErrChatNotFound,
		},
		{
			name: "maps the conversation oldest first",
			store: &fakeStore{messages: []db.EgolifterMessage{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			}},
			wantLen: 2,
		},
		{
			name:    "empty chat returns a non-nil empty slice",
			store:   &fakeStore{messages: nil},
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.store, "", nil)

			out, err := svc.ListMessages(context.Background(), "user-1", 42)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out == nil {
				t.Fatalf("ListMessages returned nil slice, want non-nil for [] JSON")
			}
			if len(out) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(out), tc.wantLen)
			}
		})
	}
}

func TestServiceDeleteChat(t *testing.T) {
	tests := []struct {
		name    string
		store   *fakeStore
		wantErr error // sentinel; nil means no error
	}{
		{
			name:  "deletes the user's chat",
			store: &fakeStore{},
		},
		{
			name:    "missing chat returns ErrChatNotFound",
			store:   &fakeStore{deleteErr: ErrChatNotFound},
			wantErr: ErrChatNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(tc.store, "", nil)

			err := svc.DeleteChat(context.Background(), "user-1", 42)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.store.deleteCalls != 1 {
				t.Errorf("DeleteChat calls = %d, want 1", tc.store.deleteCalls)
			}
		})
	}
}
