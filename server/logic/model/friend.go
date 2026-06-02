package model

import (
	"time"
)

// Friend 好友关系模型
type Friend struct {
	ID       int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID   int64     `gorm:"column:user_id;not null;index:idx_user" json:"user_id"`
	FriendID int64     `gorm:"column:friend_id;not null" json:"friend_id"`
	Remark   string    `gorm:"column:remark;size:64" json:"remark"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (Friend) TableName() string {
	return "friend"
}
