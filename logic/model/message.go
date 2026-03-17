package model

import (
	"time"

	"gochat/pkg/mysql"
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

// CreateMessage 创建消息
func CreateMessage(msg *Message) error {
	return mysql.Client.Create(msg).Error
}

// GetMessageByID 根据ID获取消息
func GetMessageByID(id int64) (*Message, error) {
	var msg Message
	err := mysql.Client.Where("id = ?", id).First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// GetPrivateMessages 获取私聊消息
func GetPrivateMessages(userID1, userID2 int64, limit int, offset int64) ([]*Message, error) {
	var messages []*Message
	err := mysql.Client.Where("(from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)",
		userID1, userID2, userID2, userID1).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}

// GetGroupMessages 获取群聊消息
func GetGroupMessages(groupID int64, limit int, offset int64) ([]*Message, error) {
	var messages []*Message
	err := mysql.Client.Where("group_id = ?", groupID).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}

// RevokeMessage 撤回消息
func RevokeMessage(id int64) error {
	return mysql.Client.Model(&Message{}).Where("id = ?", id).Update("is_revoked", true).Error
}

// GetUserMessages 获取用户发送的消息
func GetUserMessages(userID int64, limit int, offset int64) ([]*Message, error) {
	var messages []*Message
	err := mysql.Client.Where("from_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}
