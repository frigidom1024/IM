package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
// automatically reconnects after an unexpected disconnect. Automatic protocol
// pings keep idle connections active until Close is called. Send never retries
// the failed message automatically; callers can safely retry with the same
// seq so the server-side idempotency contract prevents duplicates.
type WebSocketClient struct {
	connMu        sync.RWMutex
	writeMu       sync.Mutex
	messages      chan WSMessage
	receiveErrors chan error
	receiveDone   chan struct{}
	tokenProvider func(context.Context) (string, error)
	lifetime      context.Context
	cancel        context.CancelFunc
	heartbeatDone chan struct{}
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
	client := &WebSocketClient{
		tokenProvider: tokenProvider,
		lifetime:      lifetime,
		cancel:        cancel,
		heartbeatDone: make(chan struct{}),
		messages:      make(chan WSMessage, 64),
		receiveErrors: make(chan error, 16),
		receiveDone:   make(chan struct{}),
		conn:          conn,
		endpoint:      endpoint,
		headers:       cloneHeader(headers),
	}
	go client.receiveLoop()
	if c.websocketHeartbeatInterval > 0 {
		go client.heartbeat(c.websocketHeartbeatInterval)
	} else {
		close(client.heartbeatDone)
	}
	return client, nil
}

func (c *WebSocketClient) heartbeat(interval time.Duration) {
	defer close(c.heartbeatDone)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.lifetime.Done():
			return
		case <-ticker.C:
			// Use the same writer and reconnect path as application messages.
			// Bound heartbeat writes so a stalled peer cannot block them forever.
			_ = c.send(WSMessage{Cmd: "ping"}, 5*time.Second)
		}
	}
}

// Send writes a raw protocol message. If the write fails, the SDK reconnects
// but returns the error without replaying the message.
func (c *WebSocketClient) Send(message WSMessage) error {
	return c.send(message, 0)
}

func (c *WebSocketClient) send(message WSMessage, timeout time.Duration) error {
	if c == nil {
		return errors.New("gochat sdk: websocket is not connected")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn, err := c.currentConnection()
	if err != nil {
		return err
	}
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			_ = c.reconnectFrom(conn)
			return err
		}
	}
	err = conn.WriteJSON(message)
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Time{})
	}
	if err != nil {
		_ = c.reconnectFrom(conn)
		return err
	}
	return nil
}

// SendChat sends a text or media chat message and returns the sequence number.
func (c *WebSocketClient) SendChat(conversationID, content, messageType string) (string, error) {
	if err := validateConversationID(conversationID); err != nil {
		return "", err
	}
	seq := nextSequence()
	return seq, c.Send(WSMessage{Cmd: "msg", Seq: seq, Data: ChatMessage{ConversationID: conversationID, Type: messageType, Content: content}})
}
func validateConversationID(scope string) error {
	parts := strings.Split(scope, ":")
	if len(parts) != 2 || (parts[0] != "private" && parts[0] != "group") {
		return errors.New("gochat sdk: invalid conversation ID")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[1] {
		return errors.New("gochat sdk: invalid conversation ID")
	}
	return nil
}

// Ping sends a protocol ping command.
func (c *WebSocketClient) Ping() error {
	return c.Send(WSMessage{Cmd: "ping"})
}

// Receive returns the single, bounded message stream started by ConnectWebSocket.
// Every call returns the same channel; multiple consumers divide messages rather
// than receiving broadcasts. Close or a terminal receive failure closes it.
func (c *WebSocketClient) Receive() <-chan WSMessage {
	if c == nil || c.messages == nil {
		closed := make(chan WSMessage)
		close(closed)
		return closed
	}
	return c.messages
}

// Errors reports receive/decode/reconnect failures without blocking the reader.
// Its bounded buffer retains recent errors, replacing the oldest on overflow.
// Send and explicit Reconnect errors are still returned by those methods.
func (c *WebSocketClient) Errors() <-chan error {
	if c == nil || c.receiveErrors == nil {
		closed := make(chan error, 1)
		closed <- errors.New("gochat sdk: websocket is not connected")
		close(closed)
		return closed
	}
	return c.receiveErrors
}

func (c *WebSocketClient) reportReceiveError(err error) {
	select {
	case c.receiveErrors <- err:
		return
	default:
	}
	select {
	case <-c.receiveErrors:
	default:
	}
	c.receiveErrors <- err // Only the receive loop writes; a slot is now available.
}

func (c *WebSocketClient) receiveLoop() {
	defer close(c.receiveDone)
	defer close(c.receiveErrors)
	defer close(c.messages)
	defer c.shutdown()
	consecutiveFailures := 0
	for {
		if c.lifetime.Err() != nil {
			return
		}
		conn, err := c.currentConnection()
		if err != nil {
			if !c.isClosed() {
				c.reportReceiveError(err)
			}
			return
		}
		_, body, readErr := conn.ReadMessage()
		if readErr != nil {
			if c.isClosed() {
				return
			}
			// Another caller may have already replaced this connection deliberately.
			c.connMu.RLock()
			replaced := c.conn != conn
			c.connMu.RUnlock()
			if replaced {
				continue
			}
			consecutiveFailures++
			c.reportReceiveError(fmt.Errorf("gochat sdk: websocket read: %w", readErr))
			if consecutiveFailures >= 5 {
				c.reportReceiveError(fmt.Errorf("gochat sdk: websocket repeatedly disconnected: %w", readErr))
				return
			}
			// Back off even when dialing succeeds but the peer immediately disconnects.
			timer := time.NewTimer(time.Duration(100*(1<<min(consecutiveFailures-1, 4))) * time.Millisecond)
			select {
			case <-c.lifetime.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if err = c.reconnectFrom(conn); err != nil {
				if !c.isClosed() {
					c.reportReceiveError(fmt.Errorf("gochat sdk: websocket reconnect: %w", errors.Join(readErr, err)))
				}
				return
			}
			continue
		}
		consecutiveFailures = 0
		var message WSMessage
		if err = json.Unmarshal(body, &message); err != nil {
			c.reportReceiveError(fmt.Errorf("gochat sdk: websocket decode: %w", err))
			continue // A complete malformed JSON frame does not invalidate the socket.
		}
		select {
		case c.messages <- message:
		case <-c.lifetime.Done():
			return
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

// Close cancels I/O and reconnects, stops both loops and closes the streams.
// Buffered messages/errors remain available to drain after closure.
func (c *WebSocketClient) Close() error {
	if c == nil {
		return nil
	}
	err := c.shutdown()
	if c.receiveDone != nil {
		<-c.receiveDone
	}
	if c.heartbeatDone != nil {
		<-c.heartbeatDone
	}
	return err
}

// shutdown is non-waiting so the receive loop can also terminate the lifetime.
func (c *WebSocketClient) shutdown() error {
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
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// currentConnection reads the saved connection; it never dials or reconnects.
func (c *WebSocketClient) currentConnection() (*websocket.Conn, error) {
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
	return c.reconnectConnection(c.lifetime, previous)
}

func (c *WebSocketClient) reconnect(ctx context.Context) error {
	return c.reconnectConnection(ctx, nil)
}

func (c *WebSocketClient) reconnectConnection(ctx context.Context, previous *websocket.Conn) error {
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
	// A reader and heartbeat may detect the same disconnect concurrently.
	// Check after acquiring reconnectMu so only one replaces that connection.
	if previous != nil && c.conn != nil && c.conn != previous {
		c.connMu.RUnlock()
		return nil
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
	ConversationID string     `json:"conversation_id"`
	Type           string     `json:"type"`
	Content        string     `json:"content"`
	ID             int64      `json:"id,string,omitempty"`
	SenderID       int64      `json:"sender_id,string,omitempty"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
}

// MessageReply is the ACK; exact message IDs are decoded from MessageIDString.
type MessageReply struct {
	Status          string `json:"status"`
	ConversationID  string `json:"conversation_id"`
	MessageID       int64  `json:"message_id"`
	MessageIDString string `json:"message_id_string,omitempty"`
	Duplicate       bool   `json:"duplicate,omitempty"`
}

func (r *MessageReply) UnmarshalJSON(data []byte) error {
	type alias MessageReply
	var decoded alias
	wire := struct {
		MessageID json.RawMessage `json:"message_id"`
		*alias
	}{alias: &decoded}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	value := decoded.MessageIDString
	if value == "" && len(wire.MessageID) > 0 {
		value = string(wire.MessageID)
		if wire.MessageID[0] == '"' {
			if err := json.Unmarshal(wire.MessageID, &value); err != nil {
				return err
			}
		}
	}
	if value != "" && value != "null" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		decoded.MessageID = id
	}
	*r = MessageReply(decoded)
	return nil
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
