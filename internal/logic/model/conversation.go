package model

import "time"

// Conversation is one viewer's state. FromID is its owner, not the last sender.
type Conversation struct {
	FromID        int64 `gorm:"primaryKey;autoIncrement:false"`
	ToID          int64 `gorm:"primaryKey;autoIncrement:false"`
	GroupID       int64 `gorm:"primaryKey;autoIncrement:false"`
	LastMessageID *int64
	LastMessageAt *time.Time
	UnreadCount   int64 `gorm:"not null;default:0"`
	ReadAt        *time.Time
	Pinned        bool   `gorm:"not null;default:false"`
	Muted         bool   `gorm:"not null;default:false"`
	Draft         string `gorm:"type:text;not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Conversation) TableName() string { return "conversation" }

type ConversationSummary struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Avatar         string     `json:"avatar"`
	UnreadCount    int64      `json:"unread_count"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	LastMessage    *Message   `json:"last_message"`
	LastMessageAt  *time.Time `json:"last_message_at"`
	Pinned         bool       `json:"pinned"`
	Muted          bool       `json:"muted"`
	Draft          string     `json:"draft"`
	SendPolicy     string     `json:"send_policy"`
	IntroRemaining int        `json:"intro_remaining"`
	AddedByMe      bool       `json:"added_by_me"`
	AddedByPeer    bool       `json:"added_by_peer"`
}
