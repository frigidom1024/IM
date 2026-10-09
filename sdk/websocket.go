package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WSOption customizes a WebSocket connection.
type WSOption func(http.Header) error

// WithWebSocketHeader adds a header to the WebSocket handshake.
func WithWebSocketHeader(key, value string) WSOption {
	return func(headers http.Header) error {
		headers.Set(key, value)
		return nil
	}
}

// WebSocketClient is a small typed client for GoChat's WebSocket protocol.
type WebSocketClient struct {
	conn *websocket.Conn
}

// ConnectWebSocket connects using the client's configured WebSocket endpoint.
// If no endpoint was configured, the SDK derives one from the HTTP base URL
// and appends /ws. Use WithWebSocketURL when API and WebSocket ports differ.
func (c *Client) ConnectWebSocket(ctx context.Context, options ...WSOption) (*WebSocketClient, error) {
	if c.token == "" {
		return nil, ErrMissingToken
	}
	endpoint, err := c.websocketEndpoint()
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+NormalizeToken(c.token))
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(headers); err != nil {
			return nil, fmt.Errorf("gochat sdk: configure websocket: %w", err)
		}
	}

	conn, response, err := (&websocket.Dialer{}).DialContext(ctx, endpoint, headers)
	if err != nil {
		if response != nil {
			response.Body.Close()
			return nil, fmt.Errorf("gochat sdk: websocket handshake failed: %s: %w", response.Status, err)
		}
		return nil, fmt.Errorf("gochat sdk: websocket connect: %w", err)
	}
	return &WebSocketClient{conn: conn}, nil
}

// Send writes a raw protocol message.
func (c *WebSocketClient) Send(message WSMessage) error {
	if c == nil || c.conn == nil {
		return errors.New("gochat sdk: websocket is not connected")
	}
	return c.conn.WriteJSON(message)
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

// Receive reads and decodes one protocol message.
func (c *WebSocketClient) Receive() (WSMessage, error) {
	if c == nil || c.conn == nil {
		return WSMessage{}, errors.New("gochat sdk: websocket is not connected")
	}
	var message WSMessage
	if err := c.conn.ReadJSON(&message); err != nil {
		return WSMessage{}, err
	}
	return message, nil
}

// Close closes the WebSocket connection.
func (c *WebSocketClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
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

func nextSequence() string {
	return fmt.Sprintf("sdk-%d", time.Now().UnixNano())
}
