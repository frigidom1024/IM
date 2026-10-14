package registry

import (
	"gochat/internal/logic/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type ConversationRow struct {
	model.Conversation
	Name   string
	Avatar string
}

type ConversationRepository interface {
	List(db *gorm.DB, owner int64, limit int, offset int64, unreadOnly bool) ([]ConversationRow, error)
	Ensure(tx *gorm.DB, key model.Conversation) error
	Record(tx *gorm.DB, key model.Conversation, message *model.Message, increment int64) error
	Patch(tx *gorm.DB, key model.Conversation, values map[string]any) error
	MarkRead(tx *gorm.DB, key model.Conversation) (*model.Conversation, error)
}

type conversationRepository struct{}

func NewConversationRepository() ConversationRepository { return &conversationRepository{} }

func (r *conversationRepository) List(db *gorm.DB, owner int64, limit int, offset int64, unreadOnly bool) ([]ConversationRow, error) {
	query := db.Table("conversation c").Select("c.*, COALESCE(NULLIF(u.nickname,''),u.username,g.name,'') AS name, COALESCE(u.avatar,g.avatar,'') AS avatar").
		Joins("LEFT JOIN user u ON c.group_id=0 AND u.id=c.to_id").
		Joins("LEFT JOIN `group` g ON c.group_id>0 AND g.id=c.group_id").
		Joins("LEFT JOIN group_member gm ON gm.group_id=g.id AND gm.user_id=c.from_id").
		Where("c.from_id=? AND ((c.group_id=0 AND u.id IS NOT NULL) OR (c.group_id>0 AND g.id IS NOT NULL AND gm.user_id IS NOT NULL))", owner)
	if unreadOnly {
		query = query.Where("c.unread_count>0")
	}
	if limit > 0 {
		query = query.Limit(limit).Offset(int(offset))
	}
	var rows []ConversationRow
	err := query.Order("c.pinned DESC, COALESCE(c.last_message_at,c.updated_at) DESC, c.last_message_id DESC, c.group_id, c.to_id").Scan(&rows).Error
	return rows, err
}

func (r *conversationRepository) Ensure(tx *gorm.DB, key model.Conversation) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&key).Error
}

func conversationKey(db *gorm.DB, key model.Conversation) *gorm.DB {
	return db.Where("from_id=? AND to_id=? AND group_id=?", key.FromID, key.ToID, key.GroupID)
}

func (r *conversationRepository) Record(tx *gorm.DB, key model.Conversation, message *model.Message, increment int64) error {
	if err := r.Ensure(tx, key); err != nil {
		return err
	}
	var current model.Conversation
	if err := conversationKey(tx, key).Clauses(clause.Locking{Strength: "UPDATE"}).First(&current).Error; err != nil {
		return err
	}
	values := map[string]any{"unread_count": gorm.Expr("unread_count + ?", increment), "updated_at": time.Now()}
	if current.LastMessageAt == nil || message.CreatedAt.After(*current.LastMessageAt) || (message.CreatedAt.Equal(*current.LastMessageAt) && (current.LastMessageID == nil || message.ID > *current.LastMessageID)) {
		values["last_message_id"], values["last_message_at"] = message.ID, message.CreatedAt
	}
	return conversationKey(tx.Model(&model.Conversation{}), key).Updates(values).Error
}

func (r *conversationRepository) Patch(tx *gorm.DB, key model.Conversation, values map[string]any) error {
	if err := r.Ensure(tx, key); err != nil {
		return err
	}
	return conversationKey(tx.Model(&model.Conversation{}), key).Updates(values).Error
}

func (r *conversationRepository) MarkRead(tx *gorm.DB, key model.Conversation) (*model.Conversation, error) {
	var row model.Conversation
	if err := conversationKey(tx, key).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	if err := conversationKey(tx.Model(&model.Conversation{}), key).Updates(map[string]any{"unread_count": 0, "read_at": now, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	row.UnreadCount, row.ReadAt = 0, &now
	return &row, nil
}
