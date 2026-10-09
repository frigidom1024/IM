package service

import (
	"gochat/internal/logic/model"
	"gochat/internal/logic/registry"
)

// GroupService 群组服务
type GroupService struct {
	groupRepo       registry.GroupRepository
	groupMemberRepo registry.GroupMemberRepository
}

// NewGroupService 创建群组服务实例
func NewGroupService(
	groupRepo registry.GroupRepository,
	groupMemberRepo registry.GroupMemberRepository,
) *GroupService {
	return &GroupService{
		groupRepo:       groupRepo,
		groupMemberRepo: groupMemberRepo,
	}
}

// Create 创建群组
func (s *GroupService) Create(name, avatar string, ownerID int64, maxMembers int) (*model.Group, error) {
	group := &model.Group{
		Name:       name,
		Avatar:     avatar,
		OwnerID:    ownerID,
		MaxMembers: maxMembers,
	}
	return group, s.groupRepo.Create(group)
}

// GetByID 获取群组信息
func (s *GroupService) GetByID(id int64) (*model.Group, error) {
	return s.groupRepo.GetByID(id)
}

// Update 更新群组信息
func (s *GroupService) Update(id int64, name, avatar string) error {
	return s.groupRepo.Update(id, name, avatar)
}

// Delete 删除群组
func (s *GroupService) Delete(id int64) error {
	return s.groupRepo.Delete(id)
}

// GetMembers 获取群成员列表
func (s *GroupService) GetMembers(groupID int64) ([]*model.GroupMember, error) {
	return s.groupMemberRepo.GetByGroupID(groupID)
}

// GetMemberCount 获取群成员数量
func (s *GroupService) GetMemberCount(groupID int64) (int64, error) {
	return s.groupMemberRepo.GetCount(groupID)
}

// AddMember 添加群成员
func (s *GroupService) AddMember(groupID, userID int64, role int8) error {
	member := &model.GroupMember{
		GroupID: groupID,
		UserID:  userID,
		Role:    role,
	}
	return s.groupMemberRepo.Add(member)
}

// RemoveMember 移除群成员
func (s *GroupService) RemoveMember(groupID, userID int64) error {
	return s.groupMemberRepo.Remove(groupID, userID)
}

// IsMember 检查是否是群成员
func (s *GroupService) IsMember(groupID, userID int64) (bool, error) {
	return s.groupMemberRepo.IsMember(groupID, userID)
}

// GetByOwner 获取用户拥有的群组
func (s *GroupService) GetByOwner(ownerID int64) ([]*model.Group, error) {
	return s.groupRepo.GetByOwnerID(ownerID)
}

// GetByUser 获取用户加入的所有群组（通过群成员表）
func (s *GroupService) GetByUser(userID int64) ([]*model.Group, error) {
	return s.groupRepo.GetByUserID(userID)
}

// Transfer 转让群主
func (s *GroupService) Transfer(groupID, newOwnerID int64) error {
	return s.groupRepo.TransferOwnership(groupID, newOwnerID)
}
