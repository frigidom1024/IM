package service

import (
	"errors"
	"fmt"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrConversationInvalid = errors.New("invalid conversation operation")

func ParseConversationScope(scope string) (string, int64, error) {
	parts := strings.Split(scope, ":")
	if len(parts) != 2 || (parts[0] != "private" && parts[0] != "group") {
		return "", 0, ErrConversationInvalid
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[1] {
		return "", 0, ErrConversationInvalid
	}
	return parts[0], id, nil
}
func ConversationKey(owner int64, scope string) (model.Conversation, error) {
	kind, id, err := ParseConversationScope(scope)
	if err != nil || owner <= 0 {
		return model.Conversation{}, ErrConversationInvalid
	}
	key := model.Conversation{FromID: owner}
	if kind == "group" {
		key.GroupID = id
	} else {
		if owner == id {
			return key, ErrConversationInvalid
		}
		key.ToID = id
	}
	return key, nil
}
func ConversationScope(key model.Conversation) string {
	if key.GroupID > 0 {
		return fmt.Sprintf("group:%d", key.GroupID)
	}
	return fmt.Sprintf("private:%d", key.ToID)
}

type ConversationService struct {
	repo     registry.ConversationRepository
	messages registry.MessageRepository
}

func NewConversationService(repo registry.ConversationRepository, messages registry.MessageRepository) *ConversationService {
	return &ConversationService{repo: repo, messages: messages}
}
func authorizeConversation(db *gorm.DB, key model.Conversation) error {
	var count int64
	var err error
	if key.GroupID > 0 {
		err = db.Model(&model.GroupMember{}).Joins("JOIN `group` g ON g.id=group_member.group_id").Where("group_id=? AND user_id=?", key.GroupID, key.FromID).Count(&count).Error
	} else {
		err = db.Model(&model.User{}).Where("id=?", key.ToID).Count(&count).Error
	}
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrGroupForbidden
	}
	return nil
}
func (s *ConversationService) change(owner int64, scope string, fn func(*gorm.DB, model.Conversation) error) error {
	key, err := ConversationKey(owner, scope)
	if err != nil {
		return err
	}
	if key.GroupID > 0 {
		return (GroupAdminService{}).change(key.GroupID, owner, func(tx *gorm.DB, _ *model.Group, _ *model.GroupMember) error { return fn(tx, key) })
	}
	db, err := groupDB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := authorizeConversation(tx, key); err != nil {
			return err
		}
		return fn(tx, key)
	})
}
func (s *ConversationService) Open(owner int64, scope string) error {
	return s.change(owner, scope, func(tx *gorm.DB, key model.Conversation) error {
		return s.repo.Patch(tx, key, map[string]any{"updated_at": time.Now()})
	})
}
func (s *ConversationService) Patch(owner int64, scope string, pinned, muted *bool, draft *string) error {
	if (pinned == nil && muted == nil && draft == nil) || (draft != nil && len(*draft) > 4096) {
		return ErrConversationInvalid
	}
	return s.change(owner, scope, func(tx *gorm.DB, key model.Conversation) error {
		values := map[string]any{"updated_at": time.Now()}
		if pinned != nil {
			values["pinned"] = *pinned
		}
		if muted != nil {
			values["muted"] = *muted
		}
		if draft != nil {
			values["draft"] = *draft
		}
		return s.repo.Patch(tx, key, values)
	})
}
func (s *ConversationService) MarkRead(owner int64, scope string) (*model.Conversation, error) {
	var result *model.Conversation
	err := s.change(owner, scope, func(tx *gorm.DB, key model.Conversation) error {
		var err error
		result, err = s.repo.MarkRead(tx, key)
		return err
	})
	return result, err
}
func (s *ConversationService) List(owner int64, limit int, offset int64) ([]model.ConversationSummary, error) {
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(db, owner, limit, offset, false)
	if err != nil {
		return nil, err
	}
	result := make([]model.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		item := model.ConversationSummary{ID: ConversationScope(row.Conversation), Name: row.Name, Avatar: row.Avatar, UnreadCount: row.UnreadCount, ReadAt: row.ReadAt, LastMessageAt: row.LastMessageAt, Pinned: row.Pinned, Muted: row.Muted, Draft: row.Draft, SendPolicy: SendUnrestricted}
		if row.GroupID == 0 {
			policy, err := ReadPrivatePolicy(db, owner, row.ToID)
			if err != nil {
				return nil, err
			}
			item.SendPolicy, item.IntroRemaining, item.AddedByMe, item.AddedByPeer = policy.SendPolicy, policy.IntroRemaining, policy.AddedByMe, policy.AddedByPeer
		}
		if row.LastMessageID != nil {
			message, err := s.messages.GetByID(*row.LastMessageID)
			if err != nil {
				return nil, err
			}
			if message.IsRevoked {
				message.Content = ""
			}
			item.LastMessage = message
		}
		result = append(result, item)
	}
	return result, nil
}

// History is read-only; reading an empty scope does not open it.
func (s *ConversationService) History(owner int64, scope string, limit int, offset int64) ([]*model.Message, error) {
	key, err := ConversationKey(owner, scope)
	if err != nil {
		return nil, err
	}
	db, err := groupDB()
	if err != nil {
		return nil, err
	}
	if err := authorizeConversation(db, key); err != nil {
		return nil, err
	}
	if key.GroupID > 0 {
		return s.messages.GetGroupMessages(key.GroupID, limit, offset)
	}
	return s.messages.GetPrivateMessages(owner, key.ToID, limit, offset)
}
func (s *ConversationService) UnreadSummary(owner int64) (int64, []*model.UnreadCount, error) {
	db, err := groupDB()
	if err != nil {
		return 0, nil, err
	}
	rows, err := s.repo.List(db, owner, 0, 0, true)
	if err != nil {
		return 0, nil, err
	}
	list := make([]*model.UnreadCount, 0, len(rows))
	var total int64
	for _, row := range rows {
		item := &model.UnreadCount{ID: ConversationScope(row.Conversation), UserID: owner, FromID: row.ToID, GroupID: row.GroupID, UnreadCount: row.UnreadCount, ReadAt: row.ReadAt, LatestMessageAt: row.LastMessageAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
		if row.LastMessageID != nil {
			message, err := s.messages.GetByID(*row.LastMessageID)
			if err != nil {
				return 0, nil, err
			}
			item.LatestMessageID, item.LatestSenderID, item.LatestMessageType = message.ID, message.FromID, message.MsgType
			item.LatestPreview = messagePreview(message)
		}
		total += row.UnreadCount
		list = append(list, item)
	}
	return total, list, nil
}
func messagePreview(message *model.Message) string {
	if message.IsRevoked {
		return ""
	}
	preview := message.Content
	if message.MsgType != model.MsgTypeText && message.MsgType != model.MsgTypeMention {
		preview = "[" + message.MsgType + "]"
	}
	if len(preview) > 512 {
		preview = preview[:512]
		for !utf8.ValidString(preview) {
			preview = preview[:len(preview)-1]
		}
	}
	return preview
}

// RecordMessage executes only after the idempotent message insert succeeds.
func (s *ConversationService) RecordMessage(tx *gorm.DB, message *model.Message) error {
	keys := []model.Conversation{{FromID: message.FromID, ToID: message.ToID}, {FromID: message.ToID, ToID: message.FromID}}
	if message.GroupID > 0 {
		var members []model.GroupMember
		if err := tx.Where("group_id=?", message.GroupID).Find(&members).Error; err != nil {
			return err
		}
		keys = nil
		for _, member := range members {
			keys = append(keys, model.Conversation{FromID: member.UserID, GroupID: message.GroupID})
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].FromID < keys[j].FromID })
	for _, key := range keys {
		increment := int64(1)
		if key.FromID == message.FromID {
			increment = 0
		}
		if err := s.repo.Record(tx, key, message, increment); err != nil {
			return err
		}
	}
	return nil
}

// Membership transitions hold the group lock while invoking these hooks.
func InitializeGroupConversation(tx *gorm.DB, groupID, owner int64) error {
	var latest model.Message
	err := tx.Where("group_id=?", groupID).Order("created_at DESC,id DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return registry.NewConversationRepository().Record(tx, model.Conversation{FromID: owner, GroupID: groupID}, &latest, 0)
}
func HideGroupConversations(tx *gorm.DB, groupID, owner int64) error {
	query := tx.Model(&model.Conversation{}).Where("group_id=?", groupID)
	if owner > 0 {
		query = query.Where("from_id=?", owner)
	}
	return query.Updates(map[string]any{"unread_count": 0, "updated_at": time.Now()}).Error
}
