package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

func ConnectRedis(ctx context.Context, redisURL string, timeout time.Duration) (*RedisClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("redis context is required")
	}
	if redisURL == "" {
		return nil, fmt.Errorf("redis URL is required")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		// Fallback for simple address format (e.g. localhost:6379)
		opt = &redis.Options{
			Addr: redisURL,
		}
	}

	opt.DialTimeout = timeout
	opt.ReadTimeout = timeout
	opt.WriteTimeout = timeout

	client := redis.NewClient(opt)


	// verify connection at startup
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisClient{Client: client}, nil
}

func (redisClient *RedisClient) Close() error {
	return redisClient.Client.Close()
}
