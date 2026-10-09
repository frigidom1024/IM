package model

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username  string    `gorm:"column:username;uniqueIndex;size:32;not null" json:"username"`
	Password  string    `gorm:"column:password;size:64;not null" json:"-"`
	Nickname  string    `gorm:"column:nickname;size:64" json:"nickname"`
	Avatar    string    `gorm:"column:avatar;size:255" json:"avatar"`
	Status    int8      `gorm:"column:status;default:0" json:"status"` // 0离线 1在线 2忙碌
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (User) TableName() string {
	return "user"
}

// BeforeCreate GORM创建前回调
func (u *User) BeforeCreate(tx *gorm.DB) error {
	return nil
}
