package model

import "time"

// UnreadCount stores one unread summary per recipient and private/group scope.
//
// Private messages use (UserID, FromID, GroupID=0). Group messages use
// (UserID, FromID=0, GroupID). The latest sender is kept separately so group
// messages from different senders share one aggregate row.
type UnreadCount struct {
	UserID            int64      `gorm:"column:user_id;primaryKey" json:"user_id"`
	FromID            int64      `gorm:"column:from_id;primaryKey" json:"from_id"`
	GroupID           int64      `gorm:"column:group_id;primaryKey" json:"group_id"`
	UnreadCount       int64      `gorm:"column:unread_count;not null;default:0" json:"unread_count"`
	LatestMessageID   int64      `gorm:"column:latest_message_id" json:"latest_message_id,string,omitempty"`
	LatestSenderID    int64      `gorm:"column:latest_sender_id" json:"latest_sender_id,omitempty"`
	LatestMessageType string     `gorm:"column:latest_message_type;size:20;not null;default:text" json:"latest_message_type"`
	LatestPreview     string     `gorm:"column:latest_preview;size:512;not null;default:''" json:"latest_preview"`
	LatestMessageAt   *time.Time `gorm:"column:latest_message_at" json:"latest_message_at,omitempty"`
	ReadAt            *time.Time `gorm:"column:read_at" json:"read_at,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// UnreadTarget identifies one recipient aggregate to update with a message.
type UnreadTarget struct {
	UserID  int64
	FromID  int64
	GroupID int64
}

// TableName 指定表名
func (UnreadCount) TableName() string {
	return "unread_count"
}
