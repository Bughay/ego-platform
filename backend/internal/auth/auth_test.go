package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// stubUserRepository serves users from an in-memory map keyed by email.
type stubUserRepository struct {
	err          error // when set, FindByEmail fails with it
	usersByEmail map[string]*User

	createCalled  bool
	createdEmail  string
	createdHashed string
}

func (s *stubUserRepository) Create(ctx context.Context, email, hashedPassword string) (*User, error) {
	s.createCalled = true
	s.createdEmail = email
	s.createdHashed = hashedPassword
	return &User{ID: "user-1", Email: email, Password: hashedPassword, Role: "user"}, nil
}

func (s *stubUserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.usersByEmail[email], nil
}

func (s *stubUserRepository) FindByID(ctx context.Context, id string) (*User, error) {
	for _, u := range s.usersByEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

// stubRefreshTokenRepository is an in-memory allowlist of token hashes. A hash
// present in the map represents a live refresh token; Delete removes it and
// reports whether it was there (the single-use / revocation gate).
type stubRefreshTokenRepository struct {
	hashes map[string]bool
}

func newStubTokenRepo() *stubRefreshTokenRepository {
	return &stubRefreshTokenRepository{hashes: map[string]bool{}}
}

func (s *stubRefreshTokenRepository) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	s.hashes[tokenHash] = true
	return nil
}

func (s *stubRefreshTokenRepository) Delete(ctx context.Context, tokenHash string) (bool, error) {
	if s.hashes[tokenHash] {
		delete(s.hashes, tokenHash)
		return true, nil
	}
	return false, nil
}

func testManager() *Manager {
	return NewManager("test-secret", 1, 24)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// hashPassword creates a bcrypt hash for test fixtures (min cost: only
// comparison behaviour matters here, not hashing strength).
func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash test password: %v", err)
	}
	return string(hash)
}

// --- JWT Manager tests ---

func TestManagerTokenRoundTrip(t *testing.T) {
	mgr := testManager()

	token, err := mgr.Generate("user-1", "user@test.com", "admin")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	claims, err := mgr.ValidateAccess(token)
	if err != nil {
		t.Fatalf("ValidateAccess failed: %v", err)
	}
	if claims.UserID != "user-1" || claims.Email != "user@test.com" || claims.Role != "admin" {
		t.Errorf("unexpected claims: %+v", claims)
	}
}

func TestManagerRejectsInvalidTokens(t *testing.T) {
	mgr := testManager()

	refreshToken, err := mgr.GenerateRefreshToken("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	accessToken, err := mgr.Generate("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	otherSecretToken, err := NewManager("other-secret", 1, 24).Generate("user-1", "user@test.com", "user")
	if err != nil {
		t.Fatalf("Generate with other secret failed: %v", err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{name: "refresh token used as access token", token: refreshToken},
		{name: "garbage string", token: "not.a.token"},
		{name: "empty string", token: ""},
		{name: "token signed with different secret", token: otherSecretToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := mgr.ValidateAccess(tt.token); err == nil {
				t.Error("expected ValidateAccess to fail")
			}
		})
	}

	t.Run("access token used as refresh token", func(t *testing.T) {
		if _, err := mgr.ValidateRefresh(accessToken); err == nil {
			t.Error("expected ValidateRefresh to fail")
		}
	})
}

// --- Service tests ---

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name     string
		req      *RegisterRequest
		existing map[string]*User // users already in the repo
		wantErr  string           // substring of the expected error; empty means success
	}{
		{
			name: "valid request",
			req:  &RegisterRequest{Email: "user@test.com", Password: "supersecret"},
		},
		{
			name:    "blank email",
			req:     &RegisterRequest{Email: "   ", Password: "supersecret"},
			wantErr: "valid email address is required",
		},
		{
			name:    "email without @",
			req:     &RegisterRequest{Email: "not-an-email", Password: "supersecret"},
			wantErr: "valid email address is required",
		},
		{
			name:    "password too short",
			req:     &RegisterRequest{Email: "user@test.com", Password: "short"},
			wantErr: "at least 8 characters",
		},
		{
			name:     "duplicate email",
			req:      &RegisterRequest{Email: "user@test.com", Password: "supersecret"},
			existing: map[string]*User{"user@test.com": {ID: "user-1", Email: "user@test.com"}},
			wantErr:  "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubUserRepository{usersByEmail: tt.existing}
			svc := NewAuthService(repo, newStubTokenRepo(), testManager())

			resp, err := svc.Register(context.Background(), tt.req)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if resp == nil || !resp.Success {
					t.Fatalf("unexpected response: %+v", resp)
				}
				if !repo.createCalled {
					t.Fatal("expected repository Create to be called")
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "validation:") {
				t.Errorf("expected validation error, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
			if repo.createCalled {
				t.Error("repository Create should not be called on validation failure")
			}
		})
	}
}

func TestRegisterHashesPasswordAndNormalizesEmail(t *testing.T) {
	repo := &stubUserRepository{}
	svc := NewAuthService(repo, testManager())

	password := "supersecret"
	_, err := svc.Register(context.Background(), &RegisterRequest{Email: "  USER@Test.COM  ", Password: password})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if repo.createdEmail != "user@test.com" {
		t.Errorf("expected email to be trimmed and lowercased, got %q", repo.createdEmail)
	}
	if repo.createdHashed == password {
		t.Fatal("password must not be stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.createdHashed), []byte(password)); err != nil {
		t.Errorf("stored password is not a valid bcrypt hash of the input: %v", err)
	}
}

func TestLogin(t *testing.T) {
	const password = "supersecret"
	mgr := testManager()

	newRepo := func(t *testing.T) *stubUserRepository {
		return &stubUserRepository{usersByEmail: map[string]*User{
			"user@test.com": {ID: "user-1", Email: "user@test.com", Password: hashPassword(t, password), Role: "user"},
		}}
	}

	t.Run("unknown email and wrong password return identical generic error", func(t *testing.T) {
		svc := NewAuthService(newRepo(t), mgr)

		_, _, errUnknown := svc.Login(context.Background(), &LoginRequest{Email: "nobody@test.com", Password: password})
		_, _, errWrongPw := svc.Login(context.Background(), &LoginRequest{Email: "user@test.com", Password: "wrong-password"})

		if errUnknown == nil || errWrongPw == nil {
			t.Fatalf("expected both logins to fail, got: %v / %v", errUnknown, errWrongPw)
		}
		if errUnknown.Error() != errWrongPw.Error() {
			t.Errorf("errors must be identical to prevent user enumeration: %q vs %q", errUnknown, errWrongPw)
		}
		if !strings.Contains(errUnknown.Error(), "invalid email or password") {
			t.Errorf("expected generic credentials error, got: %v", errUnknown)
		}
	})

	t.Run("success returns valid tokens", func(t *testing.T) {
		svc := NewAuthService(newRepo(t), mgr)

		resp, refreshToken, err := svc.Login(context.Background(), &LoginRequest{Email: "User@Test.com", Password: password})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if resp.TokenType != "Bearer" {
			t.Errorf("expected token type Bearer, got %q", resp.TokenType)
		}
		claims, err := mgr.ValidateAccess(resp.AccessToken)
		if err != nil {
			t.Fatalf("returned access token is invalid: %v", err)
		}
		if claims.UserID != "user-1" {
			t.Errorf("unexpected claims: %+v", claims)
		}
		if _, err := mgr.ValidateRefresh(refreshToken); err != nil {
			t.Fatalf("returned refresh token is invalid: %v", err)
		}
	})
}

func TestRefreshAndLogout(t *testing.T) {
	const email = "user@test.com"
	mgr := testManager()
	user := &User{ID: "user-1", Email: email, Role: "user"}

	refreshToken, err := mgr.GenerateRefreshToken(user.ID, user.Email, user.Role)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	accessToken, err := mgr.Generate(user.ID, user.Email, user.Role)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	t.Run("refresh rotates tokens", func(t *testing.T) {
		repo := &stubUserRepository{usersByEmail: map[string]*User{email: user}}
		svc := NewAuthService(repo, mgr)

		resp, newRefresh, err := svc.Refresh(context.Background(), refreshToken)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if _, err := mgr.ValidateAccess(resp.AccessToken); err != nil {
			t.Errorf("new access token is invalid: %v", err)
		}
		if _, err := mgr.ValidateRefresh(newRefresh); err != nil {
			t.Errorf("new refresh token is invalid: %v", err)
		}
	})

	t.Run("refresh rejects access token", func(t *testing.T) {
		repo := &stubUserRepository{usersByEmail: map[string]*User{email: user}}
		svc := NewAuthService(repo, mgr)

		if _, _, err := svc.Refresh(context.Background(), accessToken); err == nil {
			t.Error("expected refresh with an access token to fail")
		}
	})

	t.Run("refresh fails when user no longer exists", func(t *testing.T) {
		repo := &stubUserRepository{} // empty: user deleted
		svc := NewAuthService(repo, mgr)

		_, _, err := svc.Refresh(context.Background(), refreshToken)
		if err == nil || !strings.Contains(err.Error(), "user no longer exists") {
			t.Fatalf("expected user-gone error, got: %v", err)
		}
	})

	t.Run("logout accepts valid refresh token and rejects garbage", func(t *testing.T) {
		svc := NewAuthService(&stubUserRepository{}, mgr)

		if err := svc.Logout(context.Background(), refreshToken); err != nil {
			t.Errorf("expected logout success, got: %v", err)
		}
		if err := svc.Logout(context.Background(), "garbage"); err == nil {
			t.Error("expected logout with garbage token to fail")
		}
	})
}

// --- Handler tests ---

func newAuthHandler(t *testing.T, repo UserRepository, mgr *Manager) *AuthHandler {
	t.Helper()
	return NewAuthHandler(NewAuthService(repo, mgr), testLogger())
}

func postJSON(t *testing.T, handlerFunc http.HandlerFunc, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handlerFunc(rec, req)
	return rec
}

func TestRegisterEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		h := newAuthHandler(t, &stubUserRepository{}, testManager())
		password := "supersecret"
		rec := postJSON(t, h.Register, "/auth/register", RegisterRequest{Email: "user@test.com", Password: password})

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if strings.Contains(body, password) {
			t.Error("response body must not contain the plaintext password")
		}
		if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
			t.Error("response body must not contain the bcrypt hash")
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		h := newAuthHandler(t, &stubUserRepository{}, testManager())
		rec := postJSON(t, h.Register, "/auth/register", RegisterRequest{Email: "user@test.com", Password: "short"})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		h := newAuthHandler(t, &stubUserRepository{}, testManager())
		req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader("{not json"))
		rec := httptest.NewRecorder()
		h.Register(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestLoginEndpoint(t *testing.T) {
	const password = "supersecret"

	newRepo := func(t *testing.T) *stubUserRepository {
		return &stubUserRepository{usersByEmail: map[string]*User{
			"user@test.com": {ID: "user-1", Email: "user@test.com", Password: hashPassword(t, password), Role: "user"},
		}}
	}

	t.Run("wrong credentials", func(t *testing.T) {
		h := newAuthHandler(t, newRepo(t), testManager())
		rec := postJSON(t, h.Login, "/auth/login", LoginRequest{Email: "user@test.com", Password: "wrong"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success sets refresh cookie and returns access token", func(t *testing.T) {
		h := newAuthHandler(t, newRepo(t), testManager())
		rec := postJSON(t, h.Login, "/auth/login", LoginRequest{Email: "user@test.com", Password: password})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp LoginResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.AccessToken == "" || resp.TokenType != "Bearer" {
			t.Errorf("unexpected login response: %+v", resp)
		}

		cookie := findCookie(rec.Result().Cookies(), "refresh_token")
		if cookie == nil || cookie.Value == "" {
			t.Fatal("expected refresh_token cookie to be set")
		}
		if !cookie.HttpOnly {
			t.Error("refresh_token cookie must be HttpOnly")
		}
	})
}

func TestRefreshEndpoint(t *testing.T) {
	mgr := testManager()
	user := &User{ID: "user-1", Email: "user@test.com", Role: "user"}
	repo := &stubUserRepository{usersByEmail: map[string]*User{user.Email: user}}

	t.Run("missing cookie", func(t *testing.T) {
		h := newAuthHandler(t, repo, mgr)
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		rec := httptest.NewRecorder()
		h.Refresh(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("valid cookie rotates tokens", func(t *testing.T) {
		refreshToken, err := mgr.GenerateRefreshToken(user.ID, user.Email, user.Role)
		if err != nil {
			t.Fatalf("GenerateRefreshToken failed: %v", err)
		}

		h := newAuthHandler(t, repo, mgr)
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
		rec := httptest.NewRecorder()
		h.Refresh(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp RefreshResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if _, err := mgr.ValidateAccess(resp.AccessToken); err != nil {
			t.Errorf("new access token is invalid: %v", err)
		}
		if cookie := findCookie(rec.Result().Cookies(), "refresh_token"); cookie == nil || cookie.Value == "" {
			t.Error("expected rotated refresh_token cookie to be set")
		}
	})
}

func TestLogoutEndpoint(t *testing.T) {
	h := newAuthHandler(t, &stubUserRepository{}, testManager())
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cookie := findCookie(rec.Result().Cookies(), "refresh_token")
	if cookie == nil || cookie.MaxAge >= 0 {
		t.Error("expected refresh_token cookie to be cleared (MaxAge < 0)")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
