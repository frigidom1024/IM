package model

import (
	"time"

	"gochat/pkg/mysql"
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

// MarkMessageRead 标记消息已读
func MarkMessageRead(messageID, userID int64) error {
	read := &MessageRead{
		MessageID: messageID,
		UserID:    userID,
	}
	// 使用 FirstOrCreate 避免重复插入
	return mysql.Client.Where("message_id = ? AND user_id = ?", messageID, userID).
		FirstOrCreate(read).Error
}

// BatchMarkMessagesRead 批量标记消息已读
func BatchMarkMessagesRead(messageIDs []int64, userID int64) error {
	for _, msgID := range messageIDs {
		read := &MessageRead{
			MessageID: msgID,
			UserID:    userID,
		}
		if err := mysql.Client.Where("message_id = ? AND user_id = ?", msgID, userID).
			FirstOrCreate(read).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetMessageReadStatus 获取消息已读状态
func GetMessageReadStatus(messageID int64) ([]*MessageRead, error) {
	var reads []*MessageRead
	err := mysql.Client.Where("message_id = ?", messageID).Find(&reads).Error
	return reads, err
}

// IsMessageRead 检查消息是否已读
func IsMessageRead(messageID, userID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&MessageRead{}).
		Where("message_id = ? AND user_id = ?", messageID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetUnreadMessageCount 获取用户未读消息数量
func GetUnreadMessageCount(userID int64) (int64, error) {
	// 统计发送给该用户但未被标记为已读的消息数量
	var count int64
	err := mysql.Client.Model(&Message{}).
		Where("to_id = ? AND id NOT IN (SELECT message_id FROM message_read WHERE user_id = ?)", userID, userID).
		Count(&count).Error
	return count, err
}

// GetGroupUnreadMessageCount 获取用户在群聊中的未读消息数量
func GetGroupUnreadMessageCount(groupID, userID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&Message{}).
		Where("group_id = ? AND from_id != ? AND id NOT IN (SELECT message_id FROM message_read WHERE user_id = ?)",
			groupID, userID, userID).
		Count(&count).Error
	return count, err
}
