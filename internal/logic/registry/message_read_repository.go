package registry

import (
	"strings"
	"time"
	"unicode/utf8"

	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"

	"gorm.io/gorm"
)

// MessageReadRepository 消息已读与会话未读摘要仓储接口。
type MessageReadRepository interface {
	// IncrUnread atomically increments one private/group scope and updates its
	// latest-message summary. For group messages fromID must be zero.
	IncrUnread(userID, fromID, groupID int64, latestMessage *model.Message) error
	IncrUnreadWithTx(tx *gorm.DB, userID, fromID, groupID int64, latestMessage *model.Message) error
	// GetUnreadCount 获取用户总未读数
	GetUnreadCount(userID int64) (int64, error)
	// GetUnreadList 获取用户未读会话摘要
	GetUnreadList(userID int64) ([]*model.UnreadCount, error)
	// ClearUnread 清除某个私聊或群聊范围的未读计数
	ClearUnread(userID, fromID, groupID int64) error
}

// messageReadRepository 未读会话摘要默认实现
type messageReadRepository struct{}

// NewMessageReadRepository 创建消息已读仓储实例
func NewMessageReadRepository() MessageReadRepository {
	return &messageReadRepository{}
}

// IncrUnread 增加会话未读计数并覆盖最新消息摘要。
func (r *messageReadRepository) IncrUnread(userID, fromID, groupID int64, latestMessage *model.Message) error {
	return r.IncrUnreadWithTx(mysql.Client, userID, fromID, groupID, latestMessage)
}

// IncrUnreadWithTx updates one unread aggregate inside the caller's transaction.
func (r *messageReadRepository) IncrUnreadWithTx(tx *gorm.DB, userID, fromID, groupID int64, latestMessage *model.Message) error {
	if groupID > 0 {
		fromID = 0
	}

	messageID, senderID, messageType, preview, messageAt := latestMessageSummary(latestMessage)
	sql := `INSERT INTO unread_count (
			user_id, from_id, group_id, unread_count,
			latest_message_id, latest_sender_id, latest_message_type,
			latest_preview, latest_message_at, created_at, updated_at
		) VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, NOW(3), NOW(3))
		ON DUPLICATE KEY UPDATE
			unread_count = unread_count + 1,
			latest_message_id = VALUES(latest_message_id),
			latest_sender_id = VALUES(latest_sender_id),
			latest_message_type = VALUES(latest_message_type),
			latest_preview = VALUES(latest_preview),
			latest_message_at = VALUES(latest_message_at),
			updated_at = NOW(3)`
	return tx.Exec(sql,
		userID,
		fromID,
		groupID,
		messageID,
		senderID,
		messageType,
		preview,
		messageAt,
	).Error
}

// GetUnreadCount 获取用户总未读数。
func (r *messageReadRepository) GetUnreadCount(userID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&model.UnreadCount{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(unread_count), 0)").
		Scan(&count).Error
	return count, err
}

// GetUnreadList 获取用户未读会话摘要，按最新消息倒序。
func (r *messageReadRepository) GetUnreadList(userID int64) ([]*model.UnreadCount, error) {
	var list []*model.UnreadCount
	err := mysql.Client.
		Where("user_id = ? AND unread_count > 0", userID).
		Order("latest_message_at DESC, updated_at DESC").
		Find(&list).Error
	return list, err
}

// ClearUnread 清除某个私聊或群聊范围的未读计数。
func (r *messageReadRepository) ClearUnread(userID, fromID, groupID int64) error {
	if groupID > 0 {
		fromID = 0
	}
	now := time.Now()
	return mysql.Client.Model(&model.UnreadCount{}).
		Where("user_id = ? AND from_id = ? AND group_id = ?", userID, fromID, groupID).
		Updates(map[string]any{
			"unread_count": 0,
			"read_at":      now,
			"updated_at":   now,
		}).Error
}

func latestMessageSummary(message *model.Message) (int64, int64, string, string, time.Time) {
	if message == nil {
		now := time.Now()
		return 0, 0, model.MsgTypeText, "", now
	}

	messageType := message.MsgType
	if messageType == "" {
		messageType = model.MsgTypeText
	}
	preview := message.Content
	if messageType != model.MsgTypeText && messageType != model.MsgTypeMention {
		preview = "[" + messageType + "]"
	}
	preview = truncatePreview(preview, 512)
	messageAt := message.CreatedAt
	if messageAt.IsZero() {
		messageAt = time.Now()
	}
	return message.ID, message.FromID, messageType, preview, messageAt
}

func truncatePreview(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}
