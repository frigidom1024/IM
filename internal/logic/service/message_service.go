package service

import (
	"context"
	"encoding/json"
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
	apierrors "gochat/internal/pkg/errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MessageService 消息服务
type MessageService struct{ messageRepo registry.MessageRepository }

func NewMessageService(repo registry.MessageRepository) *MessageService {
	return &MessageService{messageRepo: repo}
}

// Save persists one authorized message in a caller-owned transaction.
func (s *MessageService) Save(tx *gorm.DB, msg *model.Message) error {
	if msg == nil || msg.ID <= 0 || msg.FromID <= 0 || msg.ToID < 0 || msg.GroupID < 0 || (msg.ToID > 0) == (msg.GroupID > 0) || msg.FromID == msg.ToID {
		return apierrors.New(apierrors.CodeMsgTarget, "无效的消息目标")
	}
	switch msg.MsgType {
	case model.MsgTypeText, model.MsgTypeMention, model.MsgTypeImage, model.MsgTypeFile, model.MsgTypeVoice, model.MsgTypeVideo:
	default:
		return apierrors.New(apierrors.CodeMsgInvalid, "无效的消息类型")
	}
	if strings.TrimSpace(msg.Content) == "" {
		return apierrors.New(apierrors.CodeMsgEmpty, "消息内容不能为空")
	}
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

	if msg.GroupID == 0 {
		if err := enforcePrivateSend(tx, msg); err != nil {
			return err
		}
	}
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
	}
	if err := s.messageRepo.CreateWithTx(tx, msg); err != nil {
		return err
	}
	// MySQL's historical TIMESTAMP column has second precision. Project the
	// stored timestamp so summary ordering and history use the same tie-break.
	var stored model.Message
	if err := tx.Select("created_at").Where("id = ?", msg.ID).First(&stored).Error; err != nil {
		return err
	}
	msg.CreatedAt = stored.CreatedAt
	if assetID != "" {
		if err := tx.Create(&model.MessageAsset{MessageID: msg.ID, AssetID: assetID}).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetByClientSeq returns the persisted message for a sender retry.
func (s *MessageService) GetByClientSeq(fromID int64, clientSeq string) (*model.Message, error) {
	return s.messageRepo.FindByClientSeq(fromID, clientSeq)
}

// GetByID 获取消息
func (s *MessageService) GetByID(id int64) (*model.Message, error) {
	return s.messageRepo.GetByID(id)
}

// Revoke 撤回消息
func (s *MessageService) Revoke(id int64) error {
	return s.messageRepo.Revoke(id)
}
