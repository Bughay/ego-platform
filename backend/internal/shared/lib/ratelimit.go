package lib

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Bughay/egolifter/internal/shared/cache"
	"github.com/Bughay/egolifter/internal/shared/config"
)

// RateLimit returns middleware that caps how many requests a single client IP may
// make per window (a fixed-window counter held in Redis). On exceeding the limit it
// responds 429 with a Retry-After header. It fails open: if the counter backend
// errors, the request is allowed — a limiter outage must not lock everyone out.
func RateLimit(c cache.Cache, logger *slog.Logger, cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	window := time.Duration(cfg.WindowSec) * time.Second
	limit := int64(cfg.Requests)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			count, reset, err := c.IncrementWindow(r.Context(), "ratelimit:ip:"+ip, window)
			if err != nil {
				// Fail open: allow the request, but record that the limiter is blind.
				logger.WarnContext(r.Context(), "rate limiter unavailable — allowing request",
					"ip", ip, "error", err)
				next.ServeHTTP(w, r)
				return
			}

			remaining := limit - count
			if remaining < 0 {
				remaining = 0
			}
			resetSec := int(reset.Seconds()) + 1 // round up so clients never poll early

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

			if count > limit {
				w.Header().Set("Retry-After", strconv.Itoa(resetSec))
				logger.WarnContext(r.Context(), "rate limit exceeded",
					"ip", ip, "count", count, "limit", limit)
				WriteError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the originating client IP. It prefers the first entry of
// X-Forwarded-For (set by a reverse proxy) and falls back to the connection's
// RemoteAddr. Note: X-Forwarded-For is only trustworthy behind a proxy you control;
// locally there is no such header and RemoteAddr (127.0.0.1) is used.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
