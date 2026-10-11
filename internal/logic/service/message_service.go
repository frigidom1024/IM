package service

import (
	"context"
	"encoding/json"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gochat/internal/pkg/mysql"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	var assetID string
	if msg.MsgType == model.MsgTypeImage || msg.MsgType == model.MsgTypeFile || msg.MsgType == model.MsgTypeVoice || msg.MsgType == model.MsgTypeVideo {
		var reference struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(msg.Content), &reference) == nil && strings.HasPrefix(reference.URL, "/api/file/") {
			parts := strings.Split(strings.TrimPrefix(reference.URL, "/api/file/"), "/")
			if len(parts) != 2 || parts[1] != "download" {
				return ErrMediaForbidden
			}
			if _, err := (&MediaService{}).AuthorizedAsset(context.Background(), parts[0], msg.FromID); err != nil {
				return ErrMediaForbidden
			}
			assetID = parts[0]
		}
	}
	return mysql.Client.Transaction(func(tx *gorm.DB) error {
		if msg.GroupID > 0 {
			var group model.Group
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, msg.GroupID).Error; err != nil {
				return err
			}
			var sender model.GroupMember
			if err := tx.Where("group_id = ? AND user_id = ?", msg.GroupID, msg.FromID).First(&sender).Error; err != nil {
				return ErrGroupForbidden
			}
			if sender.MutedUntil != nil && sender.MutedUntil.After(time.Now()) {
				return ErrGroupForbidden
			}
			var members []model.GroupMember
			if err := tx.Where("group_id = ? AND user_id <> ?", msg.GroupID, msg.FromID).Find(&members).Error; err != nil {
				return err
			}
			targets = make([]model.UnreadTarget, 0, len(members))
			for _, member := range members {
				targets = append(targets, model.UnreadTarget{UserID: member.UserID, GroupID: msg.GroupID})
			}
		}
		if err := s.messageRepo.CreateWithTx(tx, msg); err != nil {
			return err
		}
		if assetID != "" {
			if err := tx.Create(&model.MessageAsset{MessageID: msg.ID, AssetID: assetID}).Error; err != nil {
				return err
			}
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
