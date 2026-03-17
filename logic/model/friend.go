package model

import (
	"time"

	"gochat/pkg/mysql"
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

// CreateFriend 创建好友关系
func CreateFriend(friend *Friend) error {
	return mysql.Client.Create(friend).Error
}

// GetFriends 获取用户好友列表
func GetFriends(userID int64) ([]*Friend, error) {
	var friends []*Friend
	err := mysql.Client.Where("user_id = ?", userID).Find(&friends).Error
	return friends, err
}

// GetFriend 获取好友关系
func GetFriend(userID, friendID int64) (*Friend, error) {
	var friend Friend
	err := mysql.Client.Where("user_id = ? AND friend_id = ?", userID, friendID).First(&friend).Error
	if err != nil {
		return nil, err
	}
	return &friend, nil
}

// DeleteFriend 删除好友关系
func DeleteFriend(userID, friendID int64) error {
	return mysql.Client.Where("user_id = ? AND friend_id = ?", userID, friendID).Delete(&Friend{}).Error
}

// UpdateFriendRemark 更新好友备注
func UpdateFriendRemark(userID, friendID int64, remark string) error {
	return mysql.Client.Model(&Friend{}).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Update("remark", remark).Error
}

// IsFriend 检查是否是好友
func IsFriend(userID, friendID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&Friend{}).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Count(&count).Error
	return count > 0, err
}
