package sdk

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// WithReadRetry enables at most five retries for bodyless GET requests after
// transport failures or HTTP 429/502/503/504. Writes are never replayed.
func WithReadRetry(retries int, delay time.Duration) Option {
	return func(c *Client) {
		if retries < 0 {
			retries = 0
		}
		if retries > 5 {
			retries = 5
		}
		if delay < 0 {
			delay = 0
		}
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
		c.readRetries, c.retryDelay = retries, delay
	}
}

// Version is the semantic SDK release version.
const Version = "0.2.0"

// TokenRefreshFunc runs before authenticated HTTP requests and WS handshakes.
// Return current unchanged while valid, or refresh it when near expiration.
// It is serialized per user client and must honor context cancellation.
type TokenRefreshFunc func(context.Context, string) (string, error)

// WithTokenRefresh installs a token provider. A token must first be supplied by
// WithToken. No request is replayed after authentication failure.
func WithTokenRefresh(refresh TokenRefreshFunc) Option {
	return func(c *Client) { c.refresh = refresh }
}

// WithTimeout sets the total HTTP request timeout. Zero disables the timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout >= 0 {
			clone := *c.httpClient
			clone.Timeout = timeout
			c.httpClient = &clone
		}
	}
}

// WithTransport supplies a custom HTTP RoundTripper without mutating the caller's client.
func WithTransport(transport http.RoundTripper) Option {
	return func(c *Client) {
		if transport != nil {
			clone := *c.httpClient
			clone.Transport = transport
			c.httpClient = &clone
		}
	}
}

type tokenState struct {
	mu    sync.Mutex
	token string
}

func (c *Client) authToken(ctx context.Context) (string, error) {
	c.auth.mu.Lock()
	defer c.auth.mu.Unlock()
	if c.auth.token == "" {
		return "", ErrMissingToken
	}
	if c.refresh != nil {
		token, err := c.refresh(ctx, c.auth.token)
		if err != nil {
			return "", err
		}
		token = NormalizeToken(token)
		if token == "" {
			return "", ErrMissingToken
		}
		c.auth.token = token
	}
	return c.auth.token, nil
}

// Logger receives fixed event names and sanitized metadata only. Fields never
// include URLs, query strings, tokens, request/response bodies or raw errors.
type Logger interface {
	Log(context.Context, string, map[string]any)
}

func WithLogger(logger Logger) Option { return func(c *Client) { c.logger = logger } }
