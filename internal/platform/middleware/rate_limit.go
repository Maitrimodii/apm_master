package middleware

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

const rateLimitKeyPrefix = "ratelimit:"

// rateLimitScript is a sliding-window counter: drop entries older than the
// window, reject when the window is full, otherwise record this request.
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

// RateLimiter is a Redis-backed sliding-window limiter allowing burst
// requests per window for each key.
type RateLimiter struct {
	rdb    *redis.Client
	burst  int
	window time.Duration
	log    *slog.Logger
}

// NewRateLimiter creates a limiter. A zero window defaults to one second.
func NewRateLimiter(rdb *redis.Client, burst int, window time.Duration, log *slog.Logger) *RateLimiter {
	if window <= 0 {
		window = time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &RateLimiter{rdb: rdb, burst: burst, window: window, log: log}
}

// ByClientIP limits every request per client IP. Mount it globally so
// unauthenticated routes such as login are covered.
func (l *RateLimiter) ByClientIP() gin.HandlerFunc {
	return l.limit(func(c *gin.Context) string { return "ip:" + c.ClientIP() })
}

// ByOrg limits per authenticated organization. Mount it after JWTAuth or
// APIKeyAuth; it falls back to the client IP when no org is set.
func (l *RateLimiter) ByOrg() gin.HandlerFunc {
	return l.limit(func(c *gin.Context) string {
		if orgID := c.GetString(httpx.CtxKeyOrgID); orgID != "" {
			return "org:" + orgID
		}
		return "ip:" + c.ClientIP()
	})
}

func (l *RateLimiter) limit(keyFn func(*gin.Context) string) gin.HandlerFunc {
	expireSeconds := max(int(l.window.Seconds()*2), 1)
	return func(c *gin.Context) {
		now := time.Now().UnixNano()
		key := rateLimitKeyPrefix + keyFn(c)
		member := strconv.FormatInt(now, 10)
		result, err := l.rdb.Eval(c.Request.Context(), rateLimitScript, []string{key},
			now-l.window.Nanoseconds(), now, member, l.burst, expireSeconds).Int64()
		if err != nil {
			// Fail open: a Redis outage must not take the API down.
			l.log.WarnContext(c.Request.Context(), "rate limiter unavailable, allowing request", "error", err)
			c.Next()
			return
		}
		if result == 0 {
			httpx.Abort(c, apperr.RateLimited())
			return
		}
		c.Next()
	}
}
