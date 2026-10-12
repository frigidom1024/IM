package model

import (
	"time"
)

// Friend 好友关系模型
type Friend struct {
	ID              int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID          int64      `gorm:"column:user_id;not null;index:idx_user;uniqueIndex:uk_user_friend,priority:1" json:"user_id"`
	FriendID        int64      `gorm:"column:friend_id;not null;index:idx_friend_recipient;uniqueIndex:uk_user_friend,priority:2" json:"friend_id"`
	Remark          string     `gorm:"column:remark;size:64" json:"remark"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	RecipientSeenAt *time.Time `json:"recipient_seen_at"`
	AddedByPeer     bool       `gorm:"-" json:"added_by_peer"`
}

// TableName 指定表名
func (Friend) TableName() string {
	return "friend"
}
