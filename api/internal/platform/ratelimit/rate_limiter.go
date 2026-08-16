package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "rate_limit:"

type RateLimiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

func New(rdb *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		rdb:    rdb,
		limit:  limit,
		window: window,
	}
}

func LoginKey(ip string) string {
	return "login:ip:" + ip
}

func (rl *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	count, err := rl.rdb.Get(ctx, keyPrefix+key).Int64()
	if errors.Is(err, redis.Nil) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return count < int64(rl.limit), nil
}

func (rl *RateLimiter) RecordFailure(ctx context.Context, key string) (int64, error) {
	fullKey := keyPrefix + key

	if err := rl.rdb.SetArgs(ctx, fullKey, 0, redis.SetArgs{
		Mode: "NX",
		TTL:  rl.window,
	}).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}

	return rl.rdb.Incr(ctx, fullKey).Result()
}

func (rl *RateLimiter) Reset(ctx context.Context, key string) error {
	return rl.rdb.Del(ctx, keyPrefix+key).Err()
}
