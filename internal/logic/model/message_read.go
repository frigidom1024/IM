package model

import "time"

// UnreadCount 用户未读消息计数模型
// 存储每个用户对每个发送者/群组的未读消息数量
type UnreadCount struct {
	UserID   int64     `gorm:"column:user_id;primaryKey"`                    // 接收者ID
	FromID   int64     `gorm:"column:from_id;primaryKey"`                    // 发送者ID（群聊时为0）
	GroupID  int64     `gorm:"column:group_id;default:0"`                    // 群组ID（0表示私聊）
	Count    int64     `gorm:"column:count;default:0"`                       // 未读数量
	ReadAt   time.Time `gorm:"column:read_at;autoUpdateTime"`               // 最后阅读时间
}

// TableName 指定表名
func (UnreadCount) TableName() string {
	return "unread_count"
}
