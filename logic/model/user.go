package model

import (
	"time"

	"gochat/pkg/mysql"
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

// BeforeCreate 创建前回调
func (u *User) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// CreateUser 创建用户
func CreateUser(user *User) error {
	return mysql.Client.Create(user).Error
}

// GetUserByID 根据ID获取用户
func GetUserByID(id int64) (*User, error) {
	var user User
	err := mysql.Client.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(username string) (*User, error) {
	var user User
	err := mysql.Client.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserStatus 更新用户状态
func UpdateUserStatus(id int64, status int8) error {
	return mysql.Client.Model(&User{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateUserProfile 更新用户资料
func UpdateUserProfile(id int64, nickname, avatar string) error {
	updates := map[string]interface{}{}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	return mysql.Client.Model(&User{}).Where("id = ?", id).Updates(updates).Error
}
