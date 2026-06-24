package cache

import (
	"context"
	"fmt"

	"github.com/Bughay/egolifter/internal/shared/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and pings a Redis client from the typed config.
// Returns an error if Redis is unreachable — callers should fall back to NopCache.
func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: cannot connect to %s: %w", cfg.Addr, err)
	}
	return client, nil
}
