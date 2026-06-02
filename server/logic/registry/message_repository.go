package registry

import (
	"gochat/logic/model"
	"gochat/pkg/mysql"
)

// MessageRepository 消息仓储接口
type MessageRepository interface {
	Create(msg *model.Message) error
	GetByID(id int64) (*model.Message, error)
	GetPrivateMessages(userID1, userID2 int64, limit int, offset int64) ([]*model.Message, error)
	GetGroupMessages(groupID int64, limit int, offset int64) ([]*model.Message, error)
	Revoke(id int64) error
	GetByUserID(userID int64, limit int, offset int64) ([]*model.Message, error)
	GetUnreadMessages(userID int64, limit int) ([]*model.Message, error)
}

// messageRepository 消息仓储默认实现
type messageRepository struct{}

// NewMessageRepository 创建消息仓储实例
func NewMessageRepository() MessageRepository {
	return &messageRepository{}
}

// Create 创建消息
func (r *messageRepository) Create(msg *model.Message) error {
	return mysql.Client.Create(msg).Error
}

// GetByID 根据ID获取消息
func (r *messageRepository) GetByID(id int64) (*model.Message, error) {
	var msg model.Message
	err := mysql.Client.Where("id = ?", id).First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// GetPrivateMessages 获取私聊消息
func (r *messageRepository) GetPrivateMessages(userID1, userID2 int64, limit int, offset int64) ([]*model.Message, error) {
	var messages []*model.Message
	err := mysql.Client.Where("(from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)",
		userID1, userID2, userID2, userID1).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}

// GetGroupMessages 获取群聊消息
func (r *messageRepository) GetGroupMessages(groupID int64, limit int, offset int64) ([]*model.Message, error) {
	var messages []*model.Message
	err := mysql.Client.Where("group_id = ?", groupID).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}

// Revoke 撤回消息
func (r *messageRepository) Revoke(id int64) error {
	return mysql.Client.Model(&model.Message{}).Where("id = ?", id).Update("is_revoked", true).Error
}

// GetByUserID 获取用户发送的消息
func (r *messageRepository) GetByUserID(userID int64, limit int, offset int64) ([]*model.Message, error) {
	var messages []*model.Message
	err := mysql.Client.Where("from_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(int(offset)).
		Find(&messages).Error
	return messages, err
}

// GetUnreadMessages 获取用户未读消息
func (r *messageRepository) GetUnreadMessages(userID int64, limit int) ([]*model.Message, error) {
	var messages []*model.Message
	err := mysql.Client.Where("to_id = ? AND id NOT IN (SELECT message_id FROM message_read WHERE user_id = ?)", userID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	return messages, err
}
