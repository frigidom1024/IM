package registry

import (
	"gochat/logic/model"
	"gochat/pkg/mysql"
)

// FriendRepository 好友仓储接口
type FriendRepository interface {
	Create(friend *model.Friend) error
	GetByUserID(userID int64) ([]*model.Friend, error)
	Get(userID, friendID int64) (*model.Friend, error)
	Delete(userID, friendID int64) error
	UpdateRemark(userID, friendID int64, remark string) error
	IsFriend(userID, friendID int64) (bool, error)
}

// friendRepository 好友仓储默认实现
type friendRepository struct{}

// NewFriendRepository 创建好友仓储实例
func NewFriendRepository() FriendRepository {
	return &friendRepository{}
}

// Create 创建好友关系
func (r *friendRepository) Create(friend *model.Friend) error {
	return mysql.Client.Create(friend).Error
}

// GetByUserID 获取用户好友列表
func (r *friendRepository) GetByUserID(userID int64) ([]*model.Friend, error) {
	var friends []*model.Friend
	err := mysql.Client.Where("user_id = ?", userID).Find(&friends).Error
	return friends, err
}

// Get 获取好友关系
func (r *friendRepository) Get(userID, friendID int64) (*model.Friend, error) {
	var friend model.Friend
	err := mysql.Client.Where("user_id = ? AND friend_id = ?", userID, friendID).First(&friend).Error
	if err != nil {
		return nil, err
	}
	return &friend, nil
}

// Delete 删除好友关系
func (r *friendRepository) Delete(userID, friendID int64) error {
	return mysql.Client.Where("user_id = ? AND friend_id = ?", userID, friendID).Delete(&model.Friend{}).Error
}

// UpdateRemark 更新好友备注
func (r *friendRepository) UpdateRemark(userID, friendID int64, remark string) error {
	return mysql.Client.Model(&model.Friend{}).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Update("remark", remark).Error
}

// IsFriend 检查是否是好友
func (r *friendRepository) IsFriend(userID, friendID int64) (bool, error) {
	var count int64
	err := mysql.Client.Model(&model.Friend{}).
		Where("user_id = ? AND friend_id = ?", userID, friendID).
		Count(&count).Error
	return count > 0, err
}
