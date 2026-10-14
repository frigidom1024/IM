package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Conversation is a viewer's private or group conversation and preferences.
type Conversation struct {
	SendPolicy     string     `json:"send_policy"`
	IntroRemaining int        `json:"intro_remaining"`
	AddedByMe      bool       `json:"added_by_me"`
	AddedByPeer    bool       `json:"added_by_peer"`
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Avatar         string     `json:"avatar"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	UnreadCount    int64      `json:"unread_count"`
	LastMessage    *Message   `json:"last_message"`
	LastMessageAt  *time.Time `json:"last_message_at"`
	Pinned         bool       `json:"pinned"`
	Muted          bool       `json:"muted"`
	Draft          string     `json:"draft"`
}

// MarkConversationRead clears the authenticated viewer's whole conversation.
func (c *Client) MarkConversationRead(ctx context.Context, scope string) error {
	return c.doAuthenticated(ctx, http.MethodPost, "/api/conversation/"+url.PathEscape(scope)+"/read", nil, nil)
}

// MarkRead marks a private conversation read using its peer user ID.
func (c *Client) MarkRead(ctx context.Context, fromID int64) error {
	return c.MarkReadConversation(ctx, fromID, 0)
}

// MarkReadConversation preserves the ID-based SDK interface while using the
// conversation endpoint.
func (c *Client) MarkReadConversation(ctx context.Context, fromID, groupID int64) error {
	if groupID > 0 {
		return c.MarkConversationRead(ctx, fmt.Sprintf("group:%d", groupID))
	}
	if groupID < 0 || fromID <= 0 {
		return errors.New("gochat sdk: invalid conversation")
	}
	return c.MarkConversationRead(ctx, fmt.Sprintf("private:%d", fromID))
}

func (c *Client) GetUnreadCount(ctx context.Context) (int64, error) {
	response, err := c.GetUnreadSummary(ctx)
	return response.Count, err
}

// GetUnreadSummary gets the viewer's unread conversation summaries.
func (c *Client) GetUnreadSummary(ctx context.Context) (*UnreadCountResponse, error) {
	var response UnreadCountResponse
	err := c.doAuthenticated(ctx, http.MethodGet, "/api/conversation/unread", nil, &response)
	return &response, err
}

func (c *Client) OpenConversation(ctx context.Context, scope string) error {
	return c.doAuthenticated(ctx, http.MethodPost, "/api/conversation/"+url.PathEscape(scope)+"/open", nil, nil)
}

// ConversationPatch uses pointers to distinguish omitted values from false/empty.
type ConversationPatch struct {
	Pinned *bool   `json:"pinned,omitempty"`
	Muted  *bool   `json:"muted,omitempty"`
	Draft  *string `json:"draft,omitempty"`
}

func (c *Client) ListConversations(ctx context.Context, limit int, offset int64) ([]Conversation, error) {
	var result []Conversation
	err := c.doAuthenticated(ctx, http.MethodGet, fmt.Sprintf("/api/conversation?limit=%d&offset=%d", limit, offset), nil, &result)
	return result, err
}
func (c *Client) UpdateConversation(ctx context.Context, scope string, patch ConversationPatch) error {
	return c.doAuthenticated(ctx, http.MethodPatch, "/api/conversation/"+url.PathEscape(scope), patch, nil)
}

// UnmarshalJSON accepts both legacy numeric IDs and the lossless string IDs
// used by the server. The public ID field remains int64 for compatibility.
func (m *Message) UnmarshalJSON(data []byte) error {
	type alias Message
	var wire struct {
		ID json.RawMessage `json:"id"`
		*alias
	}
	wire.alias = (*alias)(m)
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.ID) == 0 || string(wire.ID) == "null" {
		m.ID = 0
		return nil
	}
	var value string
	if wire.ID[0] == '"' {
		if err := json.Unmarshal(wire.ID, &value); err != nil {
			return err
		}
	} else {
		value = string(wire.ID)
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("gochat sdk: invalid message ID: %w", err)
	}
	m.ID = id
	return nil
}
