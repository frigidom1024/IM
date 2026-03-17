package registry

import (
	"gochat/logic/model"
	"gochat/pkg/mysql"
)

// MessageReadRepository 消息已读仓储接口
type MessageReadRepository interface {
	Mark(messageID, userID int64) error
	MarkBatch(messageIDs []int64, userID int64) error
	GetByMessageID(messageID int64) ([]*model.MessageRead, error)
	IsRead(messageID, userID int64) (bool, error)
	GetUnreadCount(userID int64) (int64, error)
	GetGroupUnreadCount(groupID, userID int64) (int64, error)
}

// messageReadRepository 消息已读仓储默认实现
type messageReadRepository struct{}

// NewMessageReadRepository 创建消息已读仓储实例
func NewMessageReadRepository() MessageReadRepository {
	return &messageReadRepository{}
}

// Mark 标记消息已读
func (r *messageReadRepository) Mark(messageID, userID int64) error {
	read := &model.MessageRead{
		MessageID: messageID,
		UserID:    userID,
	}
	// 使用 FirstOrCreate 避免重复插入
	return mysql.Client.Where("message_id = ? AND user_id = ?", messageID, userID).
		FirstOrCreate(read).Error
}

// MarkBatch 批量标记消息已读
func (r *messageReadRepository) MarkBatch(messageIDs []int64, userID int64) error {
	for _, msgID := range messageIDs {
		read := &model.MessageRead{
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

// GetByMessageID 获取消息已读状态
func (r *messageReadRepository) GetByMessageID(messageID int64) ([]*model.MessageRead, error) {
	var reads []*model.MessageRead
	err := mysql.Client.Where("message_id = ?", messageID).Find(&reads).Error
	return reads, err
}

// IsRead 检查消息是否已读
func (r *messageReadRepository) IsRead(messageID, userID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&model.MessageRead{}).
		Where("message_id = ? AND user_id = ?", messageID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetUnreadCount 获取用户未读消息数量
func (r *messageReadRepository) GetUnreadCount(userID int64) (int64, error) {
	// 统计发送给该用户但未被标记为已读的消息数量
	var count int64
	err := mysql.Client.Model(&model.Message{}).
		Where("to_id = ? AND id NOT IN (SELECT message_id FROM message_read WHERE user_id = ?)", userID, userID).
		Count(&count).Error
	return count, err
}

// GetGroupUnreadCount 获取用户在群聊中的未读消息数量
func (r *messageReadRepository) GetGroupUnreadCount(groupID, userID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&model.Message{}).
		Where("group_id = ? AND from_id != ? AND id NOT IN (SELECT message_id FROM message_read WHERE user_id = ?)",
		groupID, userID, userID).
		Count(&count).Error
	return count, err
}
