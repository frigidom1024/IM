package service

import (
	"gochat/logic/model"
	"gochat/logic/registry"
)

// MessageService 消息服务
type MessageService struct {
	messageRepo      registry.MessageRepository
	messageReadRepo registry.MessageReadRepository
}

// NewMessageService 创建消息服务实例
func NewMessageService(
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
) *MessageService {
	return &MessageService{
		messageRepo:      messageRepo,
		messageReadRepo: messageReadRepo,
	}
}

// Send 发送消息
func (s *MessageService) Send(msg *model.Message) error {
	return s.messageRepo.Create(msg)
}

// GetByID 获取消息
func (s *MessageService) GetByID(id int64) (*model.Message, error) {
	return s.messageRepo.GetByID(id)
}

// GetPrivateMessages 获取私聊消息
func (s *MessageService) GetPrivateMessages(userID1, userID2 int64, limit int, offset int64) ([]*model.Message, error) {
	return s.messageRepo.GetPrivateMessages(userID1, userID2, limit, offset)
}

// GetGroupMessages 获取群聊消息
func (s *MessageService) GetGroupMessages(groupID int64, limit int, offset int64) ([]*model.Message, error) {
	return s.messageRepo.GetGroupMessages(groupID, limit, offset)
}

// Revoke 撤回消息
func (s *MessageService) Revoke(id int64) error {
	return s.messageRepo.Revoke(id)
}

// MarkRead 标记消息已读
func (s *MessageService) MarkRead(messageID, userID int64) error {
	return s.messageReadRepo.Mark(messageID, userID)
}

// BatchMarkRead 批量标记已读
func (s *MessageService) BatchMarkRead(messageIDs []int64, userID int64) error {
	return s.messageReadRepo.MarkBatch(messageIDs, userID)
}

// GetUnreadCount 获取未读数量
func (s *MessageService) GetUnreadCount(userID int64) (int64, error) {
	return s.messageReadRepo.GetUnreadCount(userID)
}
