package model

import (
	"time"
)

// Group 群组模型
type Group struct {
	Announcement string    `gorm:"column:announcement;type:text" json:"announcement"`
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name         string    `gorm:"column:name;size:128;not null" json:"name"`
	Avatar       string    `gorm:"column:avatar;size:255" json:"avatar"`
	OwnerID      int64     `gorm:"column:owner_id;not null;index:idx_owner" json:"owner_id"`
	MaxMembers   int       `gorm:"column:max_members;default:500" json:"max_members"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (Group) TableName() string {
	return "group"
}
