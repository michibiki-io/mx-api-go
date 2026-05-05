package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const rateLimitWindow = time.Minute

type fixedWindowLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	now     func() time.Time
	buckets map[string]rateLimitBucket
}

type rateLimitBucket struct {
	count     int
	expiresAt time.Time
}

func newFixedWindowLimiter(window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		window:  window,
		now:     time.Now,
		buckets: map[string]rateLimitBucket{},
	}
}

func (l *fixedWindowLimiter) Allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket := l.bucket(key, now)
	if bucket.count >= limit {
		l.buckets[key] = bucket
		return false
	}
	bucket.count++
	l.buckets[key] = bucket
	return true
}

func (l *fixedWindowLimiter) Limited(key string, limit int) bool {
	if limit <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.bucket(key, l.now())
	l.buckets[key] = bucket
	return bucket.count >= limit
}

func (l *fixedWindowLimiter) Add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket := l.bucket(key, now)
	bucket.count++
	l.buckets[key] = bucket
}

func (l *fixedWindowLimiter) bucket(key string, now time.Time) rateLimitBucket {
	bucket := l.buckets[key]
	if bucket.expiresAt.IsZero() || !now.Before(bucket.expiresAt) {
		return rateLimitBucket{expiresAt: now.Add(l.window)}
	}
	return bucket
}

func (h *Handler) rateLimitPublicAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.cfg.Security.RateLimit.Enabled || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		key := publicRateLimitKey(c)
		if !h.rateLimiter.Allow(key, h.cfg.Security.RateLimit.RequestsPerMinute) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"status": "TooManyRequests"})
			return
		}
		if h.failureRateLimiter.Limited(key, h.cfg.Security.RateLimit.FailureRequestsPerMinute) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"status": "TooManyRequests"})
			return
		}

		c.Next()
		if c.Writer.Status() >= http.StatusBadRequest {
			h.failureRateLimiter.Add(key)
		}
	}
}

func publicRateLimitKey(c *gin.Context) string {
	endpoint := c.FullPath()
	if strings.TrimSpace(endpoint) == "" {
		endpoint = c.Request.URL.Path
	}
	return clientAddress(c) + "|" + c.Request.Method + "|" + endpoint
}
