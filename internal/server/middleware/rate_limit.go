package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter implements a Redis-backed sliding window rate limiter.
// window controls the size of the rolling time bucket (e.g. 1 second).
// burst is the maximum number of requests allowed within that window.
type RateLimiter struct {
	rdb    *redis.Client
	limit  float64
	burst  int
	window time.Duration
}

const rateLimitScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local current_count = redis.call('ZCARD', KEYS[1])

if current_count >= tonumber(ARGV[4]) then
  return 0
end

redis.call('ZADD', KEYS[1], ARGV[2], ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[5])
return 1
`

// NewRateLimiter creates a RateLimiter. window is the sliding window size; burst
// is the max requests per window. limit is reserved for future token-bucket use.
func NewRateLimiter(rdb *redis.Client, limit float64, burst int, window time.Duration) *RateLimiter {
	if window <= 0 {
		window = time.Second
	}
	return &RateLimiter{
		rdb:    rdb,
		limit:  limit,
		burst:  burst,
		window: window,
	}
}

func (limiter *RateLimiter) Limit() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		key := "ratelimit:" + ginContext.ClientIP()
		if orgID, exists := ginContext.Get("org_id"); exists {
			if orgStr, ok := orgID.(string); ok && orgStr != "" {
				key = "ratelimit:" + orgStr
			}
		}

		now := time.Now().UnixNano()
		windowStart := now - limiter.window.Nanoseconds()
		member := fmt.Sprintf("%d", now)
		expireSeconds := int(limiter.window.Seconds() * 2)
		if expireSeconds < 1 {
			expireSeconds = 1
		}

		ctx := ginContext.Request.Context()

		result, err := limiter.rdb.Eval(ctx, rateLimitScript, []string{key}, windowStart, now, member, limiter.burst, expireSeconds).Result()
		if err != nil {
			ginContext.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "rate limiter error"})
			return
		}

		allowed, ok := result.(int64)
		if !ok || allowed == 0 {
			ginContext.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}

		ginContext.Next()
	}
}


