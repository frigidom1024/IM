package service

import (
	"gochat/logic/model"
	"gochat/logic/registry"
)

// FriendService 好友服务
type FriendService struct {
	friendRepo registry.FriendRepository
}

// NewFriendService 创建好友服务实例
func NewFriendService(friendRepo registry.FriendRepository) *FriendService {
	return &FriendService{
		friendRepo: friendRepo,
	}
}

// Add 添加好友
func (s *FriendService) Add(userID, friendID int64, remark string) error {
	friend := &model.Friend{
		UserID:   userID,
		FriendID: friendID,
		Remark:   remark,
	}
	return s.friendRepo.Create(friend)
}

// GetList 获取好友列表
func (s *FriendService) GetList(userID int64) ([]*model.Friend, error) {
	return s.friendRepo.GetByUserID(userID)
}

// Remove 删除好友
func (s *FriendService) Remove(userID, friendID int64) error {
	return s.friendRepo.Delete(userID, friendID)
}

// UpdateRemark 更新好友备注
func (s *FriendService) UpdateRemark(userID, friendID int64, remark string) error {
	return s.friendRepo.UpdateRemark(userID, friendID, remark)
}

// IsFriend 检查是否是好友
func (s *FriendService) IsFriend(userID, friendID int64) (bool, error) {
	return s.friendRepo.IsFriend(userID, friendID)
}
