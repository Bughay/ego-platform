package lib

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Bughay/egolifter/internal/shared/config"
)

// fakeCache implements cache.Cache; only IncrementWindow is exercised by the rate
// limiter, so the rest are no-op stubs.
type fakeCache struct {
	count int64         // the count IncrementWindow reports
	reset time.Duration // the window-remaining IncrementWindow reports
	err   error         // when set, IncrementWindow fails (limiter must fail open)
}

func (f *fakeCache) Get(context.Context, string) (string, error)           { return "", nil }
func (f *fakeCache) Set(context.Context, string, any, time.Duration) error { return nil }
func (f *fakeCache) Delete(context.Context, ...string) error               { return nil }
func (f *fakeCache) DeleteByPattern(context.Context, string) error         { return nil }
func (f *fakeCache) IncrementWindow(context.Context, string, time.Duration) (int64, time.Duration, error) {
	return f.count, f.reset, f.err
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRateLimit(t *testing.T) {
	cfg := config.RateLimitConfig{Enabled: true, Requests: 100, WindowSec: 60}

	tests := []struct {
		name       string
		cfg        config.RateLimitConfig
		cache      *fakeCache
		wantStatus int
		wantRetry  bool // expect a Retry-After header
	}{
		{
			name:       "under limit passes",
			cfg:        cfg,
			cache:      &fakeCache{count: 1, reset: 60 * time.Second},
			wantStatus: http.StatusOK,
		},
		{
			name:       "at limit passes",
			cfg:        cfg,
			cache:      &fakeCache{count: 100, reset: 30 * time.Second},
			wantStatus: http.StatusOK,
		},
		{
			name:       "over limit is rejected",
			cfg:        cfg,
			cache:      &fakeCache{count: 101, reset: 30 * time.Second},
			wantStatus: http.StatusTooManyRequests,
			wantRetry:  true,
		},
		{
			name:       "disabled always passes",
			cfg:        config.RateLimitConfig{Enabled: false, Requests: 1, WindowSec: 60},
			cache:      &fakeCache{count: 9999, reset: 60 * time.Second},
			wantStatus: http.StatusOK,
		},
		{
			name:       "cache error fails open",
			cfg:        cfg,
			cache:      &fakeCache{err: errors.New("redis down")},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached bool
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			})
			h := RateLimit(tt.cache, quietLogger(), tt.cfg)(next)

			req := httptest.NewRequest(http.MethodGet, "/anything", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			wantReached := tt.wantStatus == http.StatusOK
			if reached != wantReached {
				t.Fatalf("next handler reached = %v, want %v", reached, wantReached)
			}
			if got := rec.Header().Get("Retry-After"); (got != "") != tt.wantRetry {
				t.Fatalf("Retry-After present = %v (%q), want %v", got != "", got, tt.wantRetry)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{name: "remote addr host:port", remoteAddr: "192.0.2.5:1234", want: "192.0.2.5"},
		{name: "xff single", remoteAddr: "10.0.0.1:9", xff: "198.51.100.2", want: "198.51.100.2"},
		{name: "xff first of chain", remoteAddr: "10.0.0.1:9", xff: "198.51.100.2, 10.0.0.1", want: "198.51.100.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := clientIP(req); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
