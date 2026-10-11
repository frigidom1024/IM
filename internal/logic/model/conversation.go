package model

import "time"

// ConversationPreference belongs to one viewer, never to the shared scope.
type ConversationPreference struct {
	UserID    int64     `gorm:"primaryKey" json:"-"`
	Scope     string    `gorm:"primaryKey;size:64" json:"id"`
	Pinned    bool      `json:"pinned"`
	Muted     bool      `json:"muted"`
	Draft     string    `gorm:"size:4096" json:"draft"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ConversationPreference) TableName() string { return "conversation_preference" }

type Conversation struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Avatar        string     `json:"avatar"`
	UnreadCount   int64      `json:"unread_count"`
	LastMessage   *Message   `json:"last_message"`
	LastMessageAt *time.Time `json:"last_message_at"`
	Pinned        bool       `json:"pinned"`
	Muted         bool       `json:"muted"`
	Draft         string     `json:"draft"`
}
