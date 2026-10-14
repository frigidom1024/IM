package service

import (
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gorm.io/gorm"
)

// MessagingService coordinates atomic writes across messages and viewer views.
type MessagingService struct {
	messages      *MessageService
	conversations *ConversationService
}

func NewMessagingService(messages registry.MessageRepository, conversations registry.ConversationRepository) *MessagingService {
	return &MessagingService{messages: NewMessageService(messages), conversations: NewConversationService(conversations, messages)}
}

// Send returns only recipients included in the committed conversation projection.
func (s *MessagingService) Send(message *model.Message) ([]int64, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	var recipients []int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := s.messages.Save(tx, message); err != nil {
			return err
		}
		if err := s.conversations.RecordMessage(tx, message); err != nil {
			return err
		}
		if message.GroupID > 0 {
			var members []model.GroupMember
			if err := tx.Where("group_id=? AND user_id<>?", message.GroupID, message.FromID).Find(&members).Error; err != nil {
				return err
			}
			for _, member := range members {
				recipients = append(recipients, member.UserID)
			}
		} else {
			recipients = []int64{message.ToID}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return recipients, nil
}

func (s *MessagingService) GetByClientSeq(sender int64, seq string) (*model.Message, error) {
	return s.messages.GetByClientSeq(sender, seq)
}
