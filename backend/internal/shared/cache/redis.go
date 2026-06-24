package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisCache struct {
	client *redis.Client
}

// incrWindowScript implements a fixed-window counter atomically: increment the key,
// and on the first hit of the window set its expiry. Doing both in one script means
// the counter can never be left without a TTL (which would block a client forever).
// Returns {count, pttl_ms}.
var incrWindowScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return {c, redis.call('PTTL', KEYS[1])}
`)

// NewRedisCache wraps a *redis.Client in the Cache interface.
func NewRedisCache(client *redis.Client) Cache {
	return &redisCache{client: client}
}

func (c *redisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("cache.Get %q: %w", key, err)
	}
	return val, nil
}

func (c *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache.Set marshal %q: %w", key, err)
	}
	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache.Set %q: %w", key, err)
	}
	return nil
}

func (c *redisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache.Delete: %w", err)
	}
	return nil
}

// IncrementWindow runs the atomic fixed-window counter script and returns the new
// count and the time left in the window (from Redis's PTTL).
func (c *redisCache) IncrementWindow(ctx context.Context, key string, window time.Duration) (int64, time.Duration, error) {
	res, err := incrWindowScript.Run(ctx, c.client, []string{key}, window.Milliseconds()).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("cache.IncrementWindow %q: %w", key, err)
	}
	vals, ok := res.([]interface{})
	if !ok || len(vals) != 2 {
		return 0, 0, fmt.Errorf("cache.IncrementWindow %q: unexpected reply %T", key, res)
	}
	count, _ := vals[0].(int64)
	pttl, _ := vals[1].(int64)
	return count, time.Duration(pttl) * time.Millisecond, nil
}

// DeleteByPattern uses SCAN + DEL in batches of 100.
// SCAN is safe in production; KEYS blocks Redis and must never be used.
func (c *redisCache) DeleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("cache.DeleteByPattern scan %q: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache.DeleteByPattern del %q: %w", pattern, err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}
