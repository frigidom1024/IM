package model

import "time"

// 消息类型（字符串标识，可扩展）
const (
	MsgTypeText    = "text"
	MsgTypeImage   = "image"
	MsgTypeFile    = "file"
	MsgTypeMention = "mention"
	MsgTypeVoice   = "voice"
	MsgTypeVideo   = "video"
)

// Message 消息模型
type Message struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id,string"`
	FromID    int64     `gorm:"column:from_id;not null;index:idx_from" json:"from_id"`
	ClientSeq *string   `gorm:"column:client_seq;size:128;uniqueIndex:uk_message_sender_seq,priority:2" json:"client_seq,omitempty"`
	ToID      int64     `gorm:"column:to_id;default:0;index:idx_to" json:"to_id"`
	GroupID   int64     `gorm:"column:group_id;default:0;index:idx_group" json:"group_id"`
	MsgType   string    `gorm:"column:msg_type;type:varchar(20);not null" json:"msg_type"`
	Content   string    `gorm:"column:content;type:text;not null" json:"content"`
	IsRevoked bool      `gorm:"column:is_revoked;default:0" json:"is_revoked"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_created" json:"created_at"`
}

func (Message) TableName() string { return "message" }
