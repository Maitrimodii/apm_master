package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	rdb   *redis.Client
	limit float64
	burst int
}

func NewRateLimiter(rdb *redis.Client, limit float64, burst int) *RateLimiter {
	return &RateLimiter{
		rdb:   rdb,
		limit: limit,
		burst: burst,
	}
}

func (limiter *RateLimiter) Limit() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		key := "ratelimit:" + ginContext.ClientIP()
		if orgID, exists := ginContext.Get("org_id"); exists {
			if orgStr, exists := orgID.(string); exists {
				key = "ratelimit:" + orgStr
			}
		}

		now := time.Now().UnixNano()
		// 1-second window size
		windowStart := now - int64(time.Second)

		ctx := ginContext.Request.Context()

		pipe := limiter.rdb.TxPipeline()
		pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
		countCmd := pipe.ZCard(ctx, key)
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d", now)})
		pipe.Expire(ctx, key, time.Minute)

		_, err := pipe.Exec(ctx)
		if err != nil {
			ginContext.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "rate limiter error"})
			return
		}

		count := countCmd.Val()
		if count >= int64(limiter.burst) {
			ginContext.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}
		ginContext.Next()
	}
}
