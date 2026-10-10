package service

import (
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/mysql"

	"gorm.io/gorm"
)

// MessageService 消息服务
type MessageService struct {
	messageRepo     registry.MessageRepository
	messageReadRepo registry.MessageReadRepository
}

// NewMessageService 创建消息服务实例
func NewMessageService(
	messageRepo registry.MessageRepository,
	messageReadRepo registry.MessageReadRepository,
) *MessageService {
	return &MessageService{
		messageRepo:     messageRepo,
		messageReadRepo: messageReadRepo,
	}
}

// Send 发送消息
func (s *MessageService) Send(msg *model.Message) error {
	return s.messageRepo.Create(msg)
}

// SendWithUnread persists a message and all affected unread summaries atomically.
func (s *MessageService) SendWithUnread(msg *model.Message, targets []model.UnreadTarget) error {
	return mysql.Client.Transaction(func(tx *gorm.DB) error {
		if err := s.messageRepo.CreateWithTx(tx, msg); err != nil {
			return err
		}
		for _, target := range targets {
			if err := s.messageReadRepo.IncrUnreadWithTx(tx, target.UserID, target.FromID, target.GroupID, msg); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetByClientSeq returns the persisted message for a sender retry.
func (s *MessageService) GetByClientSeq(fromID int64, clientSeq string) (*model.Message, error) {
	return s.messageRepo.FindByClientSeq(fromID, clientSeq)
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

// IncrUnread 增加未读计数
func (s *MessageService) IncrUnread(userID, fromID, groupID int64, latestMessage *model.Message) error {
	return s.messageReadRepo.IncrUnread(userID, fromID, groupID, latestMessage)
}

// GetUnreadCount 获取用户总未读数
func (s *MessageService) GetUnreadCount(userID int64) (int64, error) {
	return s.messageReadRepo.GetUnreadCount(userID)
}

// GetUnreadList 获取用户未读列表
func (s *MessageService) GetUnreadList(userID int64) ([]*model.UnreadCount, error) {
	return s.messageReadRepo.GetUnreadList(userID)
}

// ClearUnread 清除未读计数
func (s *MessageService) ClearUnread(userID, fromID, groupID int64) error {
	return s.messageReadRepo.ClearUnread(userID, fromID, groupID)
}
