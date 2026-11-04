// Package redisx connects to Redis.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect opens a Redis client and pings it. addr is either a URL
// (redis://user:pass@host:6379/0) or a bare host:port.
func Connect(ctx context.Context, addr string, timeout time.Duration) (*redis.Client, error) {
	if addr == "" {
		return nil, errors.New("redis address is required")
	}

	opt := &redis.Options{Addr: addr}
	if strings.Contains(addr, "://") {
		parsed, err := redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("parse redis url: %w", err)
		}
		opt = parsed
	}
	opt.DialTimeout = timeout
	opt.ReadTimeout = timeout
	opt.WriteTimeout = timeout

	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
