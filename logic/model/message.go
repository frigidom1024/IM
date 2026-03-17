package model

import (
	"time"
)

// MessageType 消息类型
const (
	MsgTypeText    int8 = 1 // 文字消息
	MsgTypeImage   int8 = 2 // 图片消息
	MsgTypeFile    int8 = 3 // 文件消息
	MsgTypeMention int8 = 4 // @提醒消息
)

// Message 消息模型
type Message struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id"`
	FromID    int64     `gorm:"column:from_id;not null;index:idx_from" json:"from_id"`
	ToID      int64     `gorm:"column:to_id;default:0;index:idx_to" json:"to_id"`          // 0表示群消息
	GroupID   int64     `gorm:"column:group_id;default:0;index:idx_group" json:"group_id"` // 0表示单聊
	MsgType   int8      `gorm:"column:msg_type;not null" json:"msg_type"`
	Content   string    `gorm:"column:content;type:text;not null" json:"content"`
	IsRevoked bool      `gorm:"column:is_revoked;default:0" json:"is_revoked"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_created" json:"created_at"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "message"
}
