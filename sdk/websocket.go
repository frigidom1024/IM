package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSOption customizes a WebSocket connection handshake.
type WSOption func(http.Header) error

// WithWebSocketHeader adds a header to the WebSocket handshake.
func WithWebSocketHeader(key, value string) WSOption {
	return func(headers http.Header) error {
		headers.Set(key, value)
		return nil
	}
}

// WebSocketClient is a typed client for GoChat's WebSocket protocol. Receive
// automatically reconnects after an unexpected disconnect. Send never retries
// the failed message automatically; callers can safely retry with the same
// seq so the server-side idempotency contract prevents duplicates.
type WebSocketClient struct {
	connMu        sync.RWMutex
	writeMu       sync.Mutex
	readMu        sync.Mutex
	tokenProvider func(context.Context) (string, error)
	lifetime      context.Context
	cancel        context.CancelFunc
	reconnectMu   sync.Mutex
	conn          *websocket.Conn
	endpoint      string
	headers       http.Header
	closed        bool
}

// ConnectWebSocket connects using the client's configured WebSocket endpoint.
// If no endpoint was configured, the SDK derives one from the HTTP base URL
// and appends /ws. Use WithWebSocketURL when API and WebSocket ports differ.
func (c *Client) ConnectWebSocket(ctx context.Context, options ...WSOption) (*WebSocketClient, error) {
	token, err := c.authToken(ctx)
	if err != nil {
		return nil, err
	}
	endpoint, err := c.websocketEndpoint()
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(headers); err != nil {
			return nil, fmt.Errorf("gochat sdk: configure websocket: %w", err)
		}
	}

	conn, response, err := dialWebSocket(ctx, endpoint, headers)
	if err != nil {
		if response != nil {
			response.Body.Close()
			return nil, fmt.Errorf("gochat sdk: websocket handshake failed: %s: %w", response.Status, err)
		}
		return nil, fmt.Errorf("gochat sdk: websocket connect: %w", err)
	}
	var tokenProvider func(context.Context) (string, error)
	if c.refresh != nil {
		tokenProvider = c.authToken
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &WebSocketClient{
		tokenProvider: tokenProvider,
		lifetime:      lifetime,
		cancel:        cancel,
		conn:          conn,
		endpoint:      endpoint,
		headers:       cloneHeader(headers),
	}, nil
}

// Send writes a raw protocol message. If the write fails, the SDK reconnects
// but returns the error without replaying the message.
func (c *WebSocketClient) Send(message WSMessage) error {
	if c == nil {
		return errors.New("gochat sdk: websocket is not connected")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn, err := c.connection()
	if err != nil {
		return err
	}
	if err := conn.WriteJSON(message); err != nil {
		_ = c.reconnectFrom(conn)
		return err
	}
	return nil
}

// SendChat sends a text or media chat message and returns the sequence number.
func (c *WebSocketClient) SendChat(fromID, toID, groupID int64, content, messageType string) (string, error) {
	seq := nextSequence()
	return seq, c.Send(WSMessage{
		Cmd: "msg",
		Seq: seq,
		Data: ChatMessage{
			FromID:  fromID,
			ToID:    toID,
			GroupID: groupID,
			Type:    messageType,
			Content: content,
		},
	})
}

// Ping sends a protocol ping command.
func (c *WebSocketClient) Ping() error {
	return c.Send(WSMessage{Cmd: "ping"})
}

// Receive reads and decodes one protocol message. Transient connection
// failures are reconnected transparently and are not returned to the caller.
func (c *WebSocketClient) Receive() (WSMessage, error) {
	if c == nil {
		return WSMessage{}, errors.New("gochat sdk: websocket is not connected")
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	for {
		conn, err := c.connection()
		if err != nil {
			return WSMessage{}, err
		}
		var message WSMessage
		if err := conn.ReadJSON(&message); err == nil {
			return message, nil
		}
		if c.isClosed() {
			return WSMessage{}, errors.New("gochat sdk: websocket is closed")
		}
		if err := c.reconnectFrom(conn); err != nil {
			return WSMessage{}, fmt.Errorf("gochat sdk: websocket reconnect: %w", err)
		}
	}
}

// Reconnect forces a reconnect using the original endpoint and handshake.
func (c *WebSocketClient) Reconnect(ctx context.Context) error {
	if c == nil {
		return errors.New("gochat sdk: websocket is not connected")
	}
	return c.reconnect(ctx)
}

// Close closes the WebSocket connection and disables automatic reconnect.
func (c *WebSocketClient) Close() error {
	if c == nil {
		return nil
	}
	c.connMu.Lock()
	if c.closed {
		c.connMu.Unlock()
		return nil
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	conn := c.conn
	c.conn = nil
	c.connMu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (c *WebSocketClient) connection() (*websocket.Conn, error) {
	if c == nil {
		return nil, errors.New("gochat sdk: websocket is not connected")
	}
	c.connMu.RLock()
	conn, closed := c.conn, c.closed
	c.connMu.RUnlock()
	if closed {
		return nil, errors.New("gochat sdk: websocket is closed")
	}
	if conn == nil {
		if err := c.reconnect(c.lifetime); err != nil {
			return nil, err
		}
		c.connMu.RLock()
		conn = c.conn
		c.connMu.RUnlock()
	}
	if conn == nil {
		return nil, errors.New("gochat sdk: websocket is not connected")
	}
	return conn, nil
}

func (c *WebSocketClient) isClosed() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.closed
}

func (c *WebSocketClient) reconnectFrom(previous *websocket.Conn) error {
	c.connMu.RLock()
	if c.closed {
		c.connMu.RUnlock()
		return errors.New("gochat sdk: websocket is closed")
	}
	current := c.conn
	c.connMu.RUnlock()
	if current != previous && current != nil {
		return nil
	}
	return c.reconnect(c.lifetime)
}

func (c *WebSocketClient) reconnect(ctx context.Context) error {
	ctx, cancelLifetime := context.WithCancel(ctx)
	stop := context.AfterFunc(c.lifetime, cancelLifetime)
	defer func() { stop(); cancelLifetime() }()

	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.connMu.RLock()
	if c.closed {
		c.connMu.RUnlock()
		return errors.New("gochat sdk: websocket is closed")
	}
	c.connMu.RUnlock()

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(100*(1<<min(attempt-1, 4))) * time.Millisecond):
			}
		}
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		headers := cloneHeader(c.headers)
		if c.tokenProvider != nil {
			token, tokenErr := c.tokenProvider(dialCtx)
			if tokenErr != nil {
				cancel()
				return tokenErr
			}
			headers.Set("Authorization", "Bearer "+token)
		}
		conn, response, err := dialWebSocket(dialCtx, c.endpoint, headers)
		cancel()
		if err == nil {
			c.connMu.Lock()
			if c.closed {
				c.connMu.Unlock()
				_ = conn.Close()
				return errors.New("gochat sdk: websocket is closed")
			}
			old := c.conn
			c.conn = conn
			c.connMu.Unlock()
			if old != nil {
				_ = old.Close()
			}
			return nil
		}
		if response != nil {
			response.Body.Close()
		}
		lastErr = err
	}
	return lastErr
}

func dialWebSocket(ctx context.Context, endpoint string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	return (&websocket.Dialer{}).DialContext(ctx, endpoint, headers)
}

func cloneHeader(header http.Header) http.Header {
	clone := make(http.Header, len(header))
	for key, values := range header {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

// WSMessage is the common WebSocket envelope.
type WSMessage struct {
	Cmd  string `json:"cmd"`
	Seq  string `json:"seq,omitempty"`
	Data any    `json:"data,omitempty"`
}

// ChatMessage is the data payload for a msg command.
type ChatMessage struct {
	FromID  int64  `json:"from_id"`
	ToID    int64  `json:"to_id"`
	GroupID int64  `json:"group_id"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// MessageReply is the ACK payload returned for a successfully persisted chat
// message.
type MessageReply struct {
	Status    string `json:"status"`
	MessageID int64  `json:"message_id"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// DecodeData decodes a WebSocket data payload into a caller-provided value.
func (m WSMessage) DecodeData(target any) error {
	data, err := json.Marshal(m.Data)
	if err != nil {
		return fmt.Errorf("gochat sdk: encode websocket data: %w", err)
	}
	return json.Unmarshal(data, target)
}

func (c *Client) websocketEndpoint() (string, error) {
	if c.websocketURL != "" {
		return c.websocketURL, nil
	}
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("gochat sdk: invalid websocket base URL: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("gochat sdk: unsupported websocket URL scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func nextSequence() string {
	return fmt.Sprintf("sdk-%d", time.Now().UnixNano())
}
