package registry

import (
	"gochat/internal/logic/model"
	"gochat/internal/pkg/mysql"
)

// MessageReadRepository 消息已读仓储接口
type MessageReadRepository interface {
	// IncrUnread 增加未读计数
	IncrUnread(userID, fromID, groupID int64) error
	// GetUnreadCount 获取用户总未读数
	GetUnreadCount(userID int64) (int64, error)
	// GetUnreadList 获取用户未读列表（按发送者分组）
	GetUnreadList(userID int64) ([]*model.UnreadCount, error)
	// ClearUnread 清除某个发送者的未读计数
	ClearUnread(userID, fromID, groupID int64) error
}

// messageReadRepository 消息已读仓储默认实现
type messageReadRepository struct{}

// NewMessageReadRepository 创建消息已读仓储实例
func NewMessageReadRepository() MessageReadRepository {
	return &messageReadRepository{}
}

// IncrUnread 增加未读计数
func (r *messageReadRepository) IncrUnread(userID, fromID, groupID int64) error {
	// 使用 INSERT ... ON DUPLICATE KEY UPDATE 原子增加计数
	sql := `INSERT INTO unread_count (user_id, from_id, group_id, count, read_at)
			VALUES (?, ?, ?, 1, NOW())
			ON DUPLICATE KEY UPDATE count = count + 1, read_at = NOW()`
	return mysql.Client.Exec(sql, userID, fromID, groupID).Error
}

// GetUnreadCount 获取用户总未读数
func (r *messageReadRepository) GetUnreadCount(userID int64) (int64, error) {
	var count int64
	err := mysql.Client.Model(&model.UnreadCount{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(count), 0)").
		Scan(&count).Error
	return count, err
}

// GetUnreadList 获取用户未读列表（按发送者分组）
func (r *messageReadRepository) GetUnreadList(userID int64) ([]*model.UnreadCount, error) {
	var list []*model.UnreadCount
	err := mysql.Client.Where("user_id = ? AND count > 0", userID).
		Find(&list).Error
	return list, err
}

// ClearUnread 清除某个发送者的未读计数
func (r *messageReadRepository) ClearUnread(userID, fromID, groupID int64) error {
	return mysql.Client.Model(&model.UnreadCount{}).
		Where("user_id = ? AND from_id = ? AND group_id = ?", userID, fromID, groupID).
		Update("count", 0).Error
}
