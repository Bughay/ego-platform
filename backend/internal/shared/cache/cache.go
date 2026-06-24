package cache

import (
	"context"
	"errors"
	"time"
)

// ErrCacheMiss is returned by Get when a key does not exist.
// Always compare with errors.Is — never with string comparison.
var ErrCacheMiss = errors.New("cache: key not found")

// Cache is the interface all domain services depend on.
// Nothing outside this package imports go-redis directly.
type Cache interface {
	// Get retrieves a raw JSON string by key.
	// Returns ErrCacheMiss when the key is absent — callers treat this as "ask the DB".
	Get(ctx context.Context, key string) (string, error)

	// Set stores any value as JSON with the given TTL.
	// TTL must always be explicit — never pass 0 (keys without TTL never expire).
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes one or more keys atomically.
	Delete(ctx context.Context, keys ...string) error

	// DeleteByPattern removes all keys matching a glob pattern using SCAN.
	// Never use Redis KEYS in production — it blocks the server.
	DeleteByPattern(ctx context.Context, pattern string) error

	// IncrementWindow atomically increments a fixed-window counter and returns the
	// new count plus the time left in the window. Used by the rate limiter — the
	// INCR and the window expiry are applied atomically so a counter can never be
	// left without a TTL (which would block a client forever).
	IncrementWindow(ctx context.Context, key string, window time.Duration) (count int64, reset time.Duration, err error)
}

// NopCache is a no-op implementation.
// Every Get returns ErrCacheMiss. Every write is silently discarded.
// Use in unit tests and as a startup fallback when Redis is unavailable.
type NopCache struct{}

func (n *NopCache) Get(_ context.Context, _ string) (string, error) {
	return "", ErrCacheMiss
}
func (n *NopCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	return nil
}
func (n *NopCache) Delete(_ context.Context, _ ...string) error       { return nil }
func (n *NopCache) DeleteByPattern(_ context.Context, _ string) error { return nil }

// IncrementWindow on the no-op cache always reports a count of 0, so callers treat
// every request as under the limit. This makes the rate limiter fail open when
// Redis is unavailable (NopCache is the startup fallback).
func (n *NopCache) IncrementWindow(_ context.Context, _ string, window time.Duration) (int64, time.Duration, error) {
	return 0, window, nil
}
