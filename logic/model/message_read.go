package model

import (
	"time"
)

// MessageRead 消息已读模型
type MessageRead struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MessageID int64     `gorm:"column:message_id;not null;uniqueIndex:uk_msg_user,priority:1" json:"message_id"`
	UserID    int64     `gorm:"column:user_id;not null;index:idx_user;uniqueIndex:uk_msg_user,priority:2" json:"user_id"`
	ReadAt    time.Time `gorm:"column:read_at;autoCreateTime" json:"read_at"`
}

// TableName 指定表名
func (MessageRead) TableName() string {
	return "message_read"
}
