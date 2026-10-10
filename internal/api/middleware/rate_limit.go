package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitState struct {
	mu      sync.Mutex
	clients map[string]rateLimitEntry
	limit   int
	window  time.Duration
}

type rateLimitEntry struct {
	started time.Time
	count   int
}

// RateLimit applies a single-process, per-client-IP fixed-window limiter.
// It is intended for auth endpoints in the P0 single-instance deployment.
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 {
		limit = 30
	}
	if window <= 0 {
		window = time.Minute
	}
	state := &rateLimitState{
		clients: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
	}

	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		now := time.Now()
		key := clientIP(c.Request)
		state.mu.Lock()
		entry, exists := state.clients[key]
		if !exists || now.Sub(entry.started) >= state.window {
			entry = rateLimitEntry{started: now}
		}
		entry.count++
		state.clients[key] = entry
		allowed := entry.count <= state.limit
		remaining := state.window - now.Sub(entry.started)
		if len(state.clients) > 10000 {
			for client, candidate := range state.clients {
				if now.Sub(candidate.started) >= state.window {
					delete(state.clients, client)
				}
			}
		}
		state.mu.Unlock()

		if !allowed {
			retryAfter := int(math.Ceil(remaining.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", formatSeconds(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "请求过于频繁，请稍后重试",
			})
			return
		}

		c.Next()
	}
}

func clientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if request.RemoteAddr != "" {
		return request.RemoteAddr
	}
	return "unknown"
}

func formatSeconds(seconds int) string {
	return strconv.Itoa(seconds)
}
