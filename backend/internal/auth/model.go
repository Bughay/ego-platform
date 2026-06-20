package auth

import "time"

// User represents an authenticated application user.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // The "-" tag ensures the hash is NEVER serialized to JSON
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Request Payloads (Incoming) ---

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// --- Response Payloads (Outgoing) ---

type LoginResponse struct {
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type"`
	User        *User  `json:"user"`
}

type RegistrationResponse struct {
	Message string
	Success bool
	Person  *User
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// Claims represents the data embedded inside our JWT.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}
