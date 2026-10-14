package model

import "time"

// UnreadCount is a transport projection of a conversation's unread summary.
//
// Private messages use (UserID, FromID, GroupID=0). Group messages use
// (UserID, FromID=0, GroupID). The latest sender is kept separately. This DTO is never persisted;
// Conversation.FromID denotes its owner whereas this compatibility FromID is the peer.
type UnreadCount struct {
	ID                string     `json:"id"`
	UserID            int64      `json:"user_id"`
	FromID            int64      `json:"from_id"`
	GroupID           int64      `json:"group_id"`
	UnreadCount       int64      `json:"unread_count"`
	LatestMessageID   int64      `json:"latest_message_id,string,omitempty"`
	LatestSenderID    int64      `json:"latest_sender_id,omitempty"`
	LatestMessageType string     `json:"latest_message_type"`
	LatestPreview     string     `json:"latest_preview"`
	LatestMessageAt   *time.Time `json:"latest_message_at,omitempty"`
	ReadAt            *time.Time `json:"read_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
